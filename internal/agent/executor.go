package agent

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/runtime"
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
	sessionStore        SubagentSessionStore         // Optional: when set, subagent sessions are persisted
	parentSessionID     string                       // Parent session ID for linking subagent sessions
	userInstructions    string                       // ~/.gen/GEN.md + rules
	projectInstructions string                       // .gen/GEN.md + rules + local
	isGit               bool                         // whether cwd is a git repository
	mcpGetter           func() []provider.ToolSchema // MCP tool schemas from parent
	mcpRegistry         *mcp.Registry                // MCP registry for tool execution
}

type SubagentSessionStore interface {
	SaveSubagentConversation(parentSessionID, title, modelID, cwd string, messages []message.Message) (string, string, error)
	LoadSubagentMessages(agentID string) ([]message.Message, error)
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
func (e *Executor) SetMCP(getter func() []provider.ToolSchema, registry *mcp.Registry) {
	e.mcpGetter = getter
	e.mcpRegistry = registry
}

// SetSessionStore configures session persistence for subagent conversations.
// When set, completed subagent conversations are saved under the parent session.
func (e *Executor) SetSessionStore(store SubagentSessionStore, parentSessionID string) {
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
	agentTask.SetIdentity(req.Agent, req.ResumeID)
	req.LiveTaskID = agentTask.GetID()

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

		agentTask.SetIdentity(req.Agent, result.AgentID)
		agentTask.SetOutputFile(result.TranscriptPath)

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

func (e *Executor) buildLoop(ctx context.Context, rc *runConfig, agentCwd string) (*runtime.Loop, func(), error) {
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
	loop := &runtime.Loop{
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
		Tool:       &tool.Set{Allow: []string(rc.config.Tools), Disallow: []string(rc.config.DisallowedTools), MCP: e.mcpGetter, IsAgent: true},
		Permission: agentPermission(rc.permMode),
		Hooks:      e.hooks,
	}

	if e.mcpRegistry != nil {
		loop.MCP = mcp.NewCaller(e.mcpRegistry)
	}

	return loop, cleanup, nil
}

func (e *Executor) loadConversation(loop *runtime.Loop, req AgentRequest) error {
	// Fork: inherit parent conversation context
	if len(req.ParentMessages) > 0 {
		// Guard against recursive forking
		if depth := CountForkDepth(req.ParentMessages); depth >= MaxForkDepth {
			return fmt.Errorf("maximum fork depth (%d) exceeded — forked agents cannot fork more than %d levels deep", MaxForkDepth, MaxForkDepth)
		}
		loop.SetMessages(PrepareForkedMessages(req.ParentMessages))
		loop.AddUser(req.Prompt, nil)
		return nil
	}

	// Resume from saved session
	if req.ResumeID != "" {
		if err := e.resumeFromSession(loop, req.ResumeID, req.Prompt); err != nil {
			return fmt.Errorf("failed to resume agent: %w", err)
		}
		return nil
	}

	// Fresh start
	loop.AddUser(req.Prompt, nil)
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

func interpretStopReason(result *runtime.Result, maxTurns int) (success bool, errMsg string) {
	success = result.StopReason == runtime.StopEndTurn
	switch result.StopReason {
	case runtime.StopMaxTurns:
		errMsg = fmt.Sprintf("reached maximum turns (%d)", maxTurns)
	case runtime.StopMaxOutputRecoveryExhausted:
		errMsg = "output was repeatedly truncated and recovery was exhausted"
	case runtime.StopHook:
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
// 1. Explicit request override (highest priority, aliases resolved)
// 2. Parent conversation model (inherited)
func (e *Executor) resolveModelID(requestModel string) string {
	if requestModel != "" {
		return ResolveModelAlias(requestModel)
	}
	if e.parentModelID != "" {
		return e.parentModelID
	}
	return e.parentModelID
}

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
