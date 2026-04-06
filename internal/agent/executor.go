package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/worktree"
	"go.uber.org/zap"
)

// Executor runs agent LLM loops
type Executor struct {
	provider            provider.LLMProvider
	cwd                 string
	parentModelID       string // Parent conversation's model ID (used when inheriting)
	hooks               *hooks.Engine
	sessionStore        *session.Store         // Optional: when set, subagent sessions are persisted
	parentSessionID     string                 // Parent session ID for linking subagent sessions
	userInstructions    string                 // ~/.gen/GEN.md + rules
	projectInstructions string                 // .gen/GEN.md + rules + local
	isGit               bool                   // whether cwd is a git repository
	mcpGetter           func() []provider.Tool // MCP tool schemas from parent
	mcpRegistry         *mcp.Registry          // MCP registry for tool execution
}

type runConfig struct {
	config      *AgentConfig
	modelID     string
	maxTurns    int
	displayName string
	agentPrompt string
	permMode    PermissionMode
}

// NewExecutor creates a new agent executor
// parentModelID is the model used by the parent conversation (for inheritance)
// hookEngine is optional — when non-nil, PreToolUse hooks will fire during agent tool calls
func NewExecutor(llmProvider provider.LLMProvider, cwd string, parentModelID string, hookEngine *hooks.Engine) *Executor {
	return &Executor{
		provider:      llmProvider,
		cwd:           cwd,
		parentModelID: parentModelID,
		hooks:         hookEngine,
	}
}

// SetContext provides project context (instructions, git status) so subagents
// get the same system prompt foundation as the parent conversation.
func (e *Executor) SetContext(userInstructions, projectInstructions string, isGit bool) {
	e.userInstructions = userInstructions
	e.projectInstructions = projectInstructions
	e.isGit = isGit
}

// SetMCP provides the parent's MCP tool getter and registry so subagents
// can access MCP tools (schemas via getter, execution via registry).
func (e *Executor) SetMCP(getter func() []provider.Tool, registry *mcp.Registry) {
	e.mcpGetter = getter
	e.mcpRegistry = registry
}

// SetSessionStore configures session persistence for subagent conversations.
// When set, completed subagent conversations are saved under the parent session.
func (e *Executor) SetSessionStore(store *session.Store, parentSessionID string) {
	e.sessionStore = store
	e.parentSessionID = parentSessionID
}

// GetParentModelID returns the parent model ID
func (e *Executor) GetParentModelID() string {
	return e.parentModelID
}

// Run executes an agent request and returns the result.
// For background agents, this should be called in a goroutine.
func (e *Executor) Run(ctx context.Context, req AgentRequest) (*AgentResult, error) {
	run, err := e.prepareRun(req)
	if err != nil {
		return nil, err
	}
	defer run.close()

	ctx = e.attachRunContext(ctx, run.cfg.displayName)
	e.logRunStart(run)
	e.fireSubagentStart(run.req, run.hookID)

	result, err := e.executePreparedRun(ctx, run)
	if err != nil {
		cancelled := e.buildCancelledAgentResult(run, result)
		if cancelled != nil {
			return cancelled, err
		}
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	return e.buildAgentResult(run, result), nil
}

// RunBackground executes an agent in the background and returns the task
func (e *Executor) RunBackground(req AgentRequest) (*task.AgentTask, error) {
	// Get agent configuration for validation
	config, ok := DefaultRegistry.Get(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", req.Agent)
	}

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Use custom name if provided, otherwise use config name
	displayName := displayNameFor(config, req)

	// Create agent task
	agentTask := task.NewAgentTask(
		task.GenerateID(),
		displayName,
		req.Description,
		ctx,
		cancel,
	)

	// Register with task manager
	task.DefaultManager.RegisterTask(agentTask)

	// Set up progress callback to forward to task
	req.OnProgress = func(msg string) {
		agentTask.AppendProgress(msg)
	}

	// Start goroutine to run agent
	go func() {
		defer cancel()

		result, err := e.Run(ctx, req)
		if err != nil {
			agentTask.AppendOutput([]byte(fmt.Sprintf("Error: %v\n", err)))
			agentTask.Complete(err)
			return
		}

		// Append result content to output
		if result.Content != "" {
			agentTask.AppendOutput([]byte(result.Content))
		}

		// Update progress
		agentTask.UpdateProgress(result.TurnCount, result.TokenUsage.TotalTokens)

		if result.Success {
			agentTask.Complete(nil)
		} else {
			agentTask.Complete(fmt.Errorf("%s", result.Error))
		}
	}()

	return agentTask, nil
}

func (e *Executor) validateRequest(req AgentRequest) error {
	if req.TeamName != "" {
		return fmt.Errorf("team spawning is not yet supported (team: %s)", req.TeamName)
	}
	return nil
}

func (e *Executor) prepareWorkspace(req AgentRequest) (string, func(), error) {
	if req.Isolation != "worktree" {
		return e.cwd, func() {}, nil
	}

	result, cleanup, err := worktree.Create(e.cwd, "")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create worktree: %w", err)
	}
	return result.Path, cleanup, nil
}

func (e *Executor) prepareRunConfig(req AgentRequest) (*runConfig, error) {
	config, ok := DefaultRegistry.Get(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", req.Agent)
	}

	displayName := displayNameFor(config, req)

	permMode := config.PermissionMode
	if req.Mode != "" {
		permMode = PermissionMode(req.Mode)
	}

	maxTurns := config.MaxTurns
	if req.MaxTurns > 0 {
		maxTurns = req.MaxTurns
	}
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}

	return &runConfig{
		config:      config,
		modelID:     e.resolveModelID(req.Model),
		maxTurns:    maxTurns,
		displayName: displayName,
		agentPrompt: e.buildSystemPrompt(config, permMode),
		permMode:    permMode,
	}, nil
}

func (e *Executor) fireSubagentStart(req AgentRequest, agentHookID string) {
	if e.hooks == nil {
		return
	}
	e.hooks.ExecuteAsync(hooks.SubagentStart, hooks.HookInput{
		AgentType:   req.Agent,
		AgentID:     agentHookID,
		Description: req.Description,
	})
}

func (e *Executor) buildLoop(ctx context.Context, rc *runConfig, agentCwd string) (*core.Loop, func(), error) {
	cleanup := func() {}

	if len(rc.config.McpServers) > 0 && e.mcpRegistry != nil {
		mcpCleanup, errs := mcp.ConnectServers(ctx, e.mcpRegistry, rc.config.McpServers)
		if mcpCleanup != nil {
			cleanup = mcpCleanup
		}
		for _, err := range errs {
			log.Logger().Warn("Agent MCP server connection failed", zap.Error(err))
		}
	}

	c := &client.Client{Provider: e.provider, Model: rc.modelID}
	loop := &core.Loop{
		System: &system.System{
			Client:              c,
			Cwd:                 agentCwd,
			IsGit:               e.isGit,
			PlanMode:            rc.permMode == PermissionPlan,
			UserInstructions:    e.userInstructions,
			ProjectInstructions: e.projectInstructions,
			Extra:               []string{rc.agentPrompt},
		},
		Client:     c,
		Tool:       &tool.Set{Allow: []string(rc.config.Tools), MCP: e.mcpGetter, IsAgent: true},
		Permission: agentPermission(rc.permMode),
		Hooks:      e.hooks,
	}

	if e.mcpRegistry != nil {
		loop.MCP = mcp.NewCaller(e.mcpRegistry)
	}

	return loop, cleanup, nil
}

func (e *Executor) loadConversation(loop *core.Loop, req AgentRequest) error {
	if req.ResumeID == "" {
		loop.AddUser(req.Prompt, nil)
		return nil
	}

	if err := e.resumeFromSession(loop, req.ResumeID, req.Prompt); err != nil {
		return fmt.Errorf("failed to resume agent: %w", err)
	}
	return nil
}

func (e *Executor) buildOnToolStart(req AgentRequest, allProgress *[]string) func(tc message.ToolCall) bool {
	return func(tc message.ToolCall) bool {
		params, _ := message.ParseToolInput(tc.Input)
		progressMsg := formatToolProgress(tc.Name, params)
		*allProgress = append(*allProgress, progressMsg)
		if req.OnProgress != nil {
			req.OnProgress(progressMsg)
		}
		return true
	}
}

func interpretStopReason(result *core.Result, maxTurns int) (success bool, errMsg string) {
	success = result.StopReason == core.StopEndTurn
	switch result.StopReason {
	case core.StopMaxTurns:
		errMsg = fmt.Sprintf("reached maximum turns (%d)", maxTurns)
	case core.StopMaxOutputRecoveryExhausted:
		errMsg = "output was repeatedly truncated and recovery was exhausted"
	case core.StopHook:
		errMsg = result.StopDetail
	}
	return success, errMsg
}

func (e *Executor) fireSubagentStop(req AgentRequest, agentHookID, agentSessionID, agentTranscriptPath, resultContent string) {
	if e.hooks == nil {
		return
	}

	stopAgentID := agentHookID
	if agentSessionID != "" {
		stopAgentID = agentSessionID
	}
	e.hooks.ExecuteAsync(hooks.SubagentStop, hooks.HookInput{
		AgentType:            req.Agent,
		AgentID:              stopAgentID,
		AgentTranscriptPath:  agentTranscriptPath,
		LastAssistantMessage: resultContent,
		StopHookActive:       e.hooks.StopHookActive(),
	})
}

// resolveModelID determines the model to use based on priority:
// 1. Explicit request override (highest priority)
// 2. Parent conversation model (inherited)
// 3. Fallback default (lowest priority)
func (e *Executor) resolveModelID(requestModel string) string {
	if requestModel != "" {
		return requestModel
	}
	if e.parentModelID != "" {
		return e.parentModelID
	}
	return FallbackModel
}

// buildSystemPrompt builds agent-specific Extra content for the system prompt.
// Identity, environment, instructions, and tool guidelines are already provided
// by system.System — this method only adds agent-specific content.
func (e *Executor) buildSystemPrompt(config *AgentConfig, permMode PermissionMode) string {
	var sb strings.Builder

	// Agent type header
	sb.WriteString(fmt.Sprintf("## Agent Type: %s\n", config.Name))
	sb.WriteString(config.Description)
	sb.WriteString("\n\n")

	// Mode-specific instructions
	switch permMode {
	case PermissionPlan:
		sb.WriteString("## Mode: Read-Only\n")
		sb.WriteString("You are in read-only mode. You can only use tools that read information (Read, Glob, Grep, WebFetch, WebSearch). Do not attempt to modify any files.\n\n")
	case PermissionDontAsk, PermissionBypassPermissions:
		sb.WriteString("## Mode: Autonomous\n")
		sb.WriteString("You have full autonomy to complete your task. You can read and modify files, execute commands, and make changes as needed.\n\n")
	}

	// Custom system prompt from config (lazily loaded from AGENT.md body)
	if sysPrompt := config.GetSystemPrompt(); sysPrompt != "" {
		sb.WriteString("## Additional Instructions\n")
		sb.WriteString(sysPrompt)
		sb.WriteString("\n\n")
	}

	// Preload skills into agent system prompt
	if len(config.Skills) > 0 && skill.DefaultRegistry != nil {
		for _, skillName := range config.Skills {
			prompt := skill.DefaultRegistry.GetSkillInvocationPrompt(skillName)
			if prompt != "" {
				sb.WriteString("\n")
				sb.WriteString(prompt)
				sb.WriteString("\n")
			}
		}
	}

	// Guidelines
	sb.WriteString("## Guidelines\n")
	sb.WriteString("- Focus on completing your assigned task efficiently\n")
	sb.WriteString("- Return a clear summary when your task is complete\n")
	sb.WriteString("- If you encounter errors, report them clearly\n")

	return sb.String()
}

func displayNameFor(config *AgentConfig, req AgentRequest) string {
	if req.Name != "" {
		return req.Name
	}
	return config.Name
}

// toolProgressParams maps tool names to the parameter key used for display.
var toolProgressParams = map[string]string{
	"Read":      "file_path",
	"Glob":      "pattern",
	"Grep":      "pattern",
	"WebFetch":  "url",
	"WebSearch": "query",
	"Bash":      "command",
}

// formatToolProgress creates a progress message for a tool call in ToolName(args) format.
func formatToolProgress(toolName string, params map[string]any) string {
	paramKey, ok := toolProgressParams[toolName]
	if !ok {
		return fmt.Sprintf("%s()", toolName)
	}

	value, ok := params[paramKey].(string)
	if !ok {
		return fmt.Sprintf("%s()", toolName)
	}

	// Truncate long strings
	if len(value) > 60 {
		value = value[:57] + "..."
	}

	return fmt.Sprintf("%s(%s)", toolName, value)
}

// persistSubagentSession saves the subagent conversation to disk if a session store is configured.
// Returns the session ID and transcript path (both empty if not persisted).
func (e *Executor) persistSubagentSession(agentName, modelID, description string, messages []message.Message) (string, string) {
	if e.sessionStore == nil || e.parentSessionID == "" {
		return "", ""
	}

	entries := session.MessagesToEntries(messages)
	if len(entries) == 0 {
		return "", ""
	}

	title := description
	if title == "" {
		title = agentName
	}

	sess := &session.Session{
		Metadata: session.SessionMetadata{
			Title:           title,
			Model:           modelID,
			Cwd:             e.cwd,
			ParentSessionID: e.parentSessionID,
		},
		Entries: entries,
	}

	if err := e.sessionStore.SaveSubagent(e.parentSessionID, sess); err != nil {
		log.Logger().Warn("Failed to persist subagent session",
			zap.String("agent", agentName),
			zap.Error(err),
		)
		return "", ""
	}

	transcriptPath := e.sessionStore.SubagentPath(e.parentSessionID, sess.Metadata.ID)
	return sess.Metadata.ID, transcriptPath
}

// --- Resume & Isolation ---

// resumeFromSession loads a previous subagent session and restores its conversation,
// then appends the new prompt as a continuation.
func (e *Executor) resumeFromSession(loop *core.Loop, agentID, newPrompt string) error {
	if e.sessionStore == nil {
		return fmt.Errorf("session store not configured, cannot resume")
	}

	sess, err := e.sessionStore.LoadSubagent(agentID)
	if err != nil {
		return err
	}

	// Convert persisted entries back to messages
	prevMessages := session.EntriesToMessages(sess.Entries)
	if len(prevMessages) == 0 {
		return fmt.Errorf("no messages found in session %s", agentID)
	}

	// Restore previous conversation and append continuation prompt
	loop.SetMessages(prevMessages)
	loop.AddUser(newPrompt, nil)

	log.Logger().Info("Resumed agent from previous session",
		zap.String("agentID", agentID),
		zap.Int("previousMessages", len(prevMessages)),
	)
	return nil
}

// --- Internal helpers ---

// agentPermission maps PermissionMode to a permission.Checker.
func agentPermission(mode PermissionMode) permission.Checker {
	switch mode {
	case PermissionPlan:
		return permission.ReadOnly()
	case PermissionAcceptEdits:
		return permission.AcceptEdits()
	case PermissionDefault:
		return permission.PermitAll() // Non-interactive agents auto-approve
	default:
		return permission.PermitAll()
	}
}
