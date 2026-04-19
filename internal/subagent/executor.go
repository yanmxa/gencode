package subagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/worktree"
	"go.uber.org/zap"
)

// Executor runs agent LLM loops
type Executor struct {
	provider            llm.Provider
	cwd                 string
	parentModelID       string // Parent conversation's model ID (used when inheriting)
	hooks               *hook.Engine
	sessionStore        SubagentSessionStore        // Optional: when set, subagent sessions are persisted
	parentSessionID     string                      // Parent session ID for linking subagent sessions
	userInstructions    string                      // ~/.gen/GEN.md + rules
	projectInstructions string                      // .gen/GEN.md + rules + local
	isGit               bool                        // whether cwd is a git repository
	skillsPrompt        string                      // available skills section for capable subagents
	agentsPrompt        string                      // available agents section for capable subagents
	mcpGetter           func() []core.ToolSchema // MCP tool schemas from parent
	mcpRegistry         *mcp.Registry               // MCP registry for tool execution
}

type SubagentSessionStore interface {
	SaveSubagentConversation(parentSessionID, title, modelID, cwd string, messages []core.Message) (string, string, error)
	LoadSubagentMessages(agentID string) ([]core.Message, error)
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
func NewExecutor(llmProvider llm.Provider, cwd string, parentModelID string, hookEngine *hook.Engine) *Executor {
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

// SetCapabilities provides skills and agents prompt sections so subagents
// that have Agent/Skill tools can see available capabilities.
func (e *Executor) SetCapabilities(skillsPrompt, agentsPrompt string) {
	e.skillsPrompt = skillsPrompt
	e.agentsPrompt = agentsPrompt
}

// SetMCP provides the parent's MCP tool getter and registry so subagents
// can access MCP tools (schemas via getter, execution via registry).
func (e *Executor) SetMCP(getter func() []core.ToolSchema, registry *mcp.Registry) {
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
	config, ok := defaultRegistry.Get(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", req.Agent)
	}

	// Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())

	// Use custom name if provided, otherwise use config name
	displayName := displayNameFor(config, req)

	// Create agent task
	agentTask := task.NewAgentTask(
		generateShortID(),
		displayName,
		req.Description,
		ctx,
		cancel,
	)
	agentTask.SetIdentity(req.Agent, req.ResumeID)
	req.LiveTaskID = agentTask.GetID()

	// Register with task manager
	task.Default().RegisterTask(agentTask)

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
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("agent prompt cannot be empty")
	}
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
	config, ok := defaultRegistry.Get(req.Agent)
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
		maxTurns = defaultMaxTurns
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
	e.hooks.ExecuteAsync(hook.SubagentStart, hook.HookInput{
		AgentType:   req.Agent,
		AgentID:     agentHookID,
		Description: req.Description,
	})
}

func (e *Executor) buildAgent(ctx context.Context, rc *runConfig, agentCwd string, onToolExec ...func(string, map[string]any)) (core.Agent, func(), error) {
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

	// Capabilities — only inject for subagents with corresponding tools
	var skillsPrompt, agentsPrompt string
	if hasToolAccess(rc.config.Tools, "Skill") {
		skillsPrompt = e.skillsPrompt
	}
	if hasToolAccess(rc.config.Tools, "Agent") {
		agentsPrompt = e.agentsPrompt
	}

	// System prompt
	sys := system.Build(system.Config{
		ProviderName:        e.provider.Name(),
		ModelID:             rc.modelID,
		Cwd:                 agentCwd,
		IsGit:               e.isGit,
		IsSubagent:          true,
		PlanMode:            rc.permMode == PermissionPlan,
		UserInstructions:    e.userInstructions,
		ProjectInstructions: e.projectInstructions,
		Skills:              skillsPrompt,
		Agents:              agentsPrompt,
		Extra:               []system.ExtraLayer{{Name: "agent-identity", Content: rc.agentPrompt}},
	})

	// Tools — adapt legacy tool registry + MCP tools
	toolSet := newAgentToolSet([]string(rc.config.Tools), []string(rc.config.DisallowedTools), e.mcpGetter)
	schemas := toolSet.Tools()
	tools := tool.AdaptToolRegistry(schemas, func() string { return agentCwd })

	// Add MCP tool executors
	if e.mcpRegistry != nil {
		mcpCaller := mcp.NewCaller(e.mcpRegistry)
		for _, t := range mcp.AsCoreTools(schemas, mcpCaller) {
			tools.Add(t)
		}
	}

	var coreTools core.Tools = tools
	if len(onToolExec) > 0 && onToolExec[0] != nil {
		coreTools = &progressTools{inner: tools, onExec: onToolExec[0]}
	}

	// Wrap tools with permission decorator
	permFn := perm.AsPermissionFunc(agentPermission(rc.permMode))
	coreTools = tool.WithPermission(coreTools, permFn)

	ag := core.NewAgent(core.Config{
		LLM:       llm.NewClient(e.provider, rc.modelID, 0),
		System:    sys,
		Tools:     coreTools,
		AgentType: rc.config.Name,
		CWD:       agentCwd,
		MaxTurns:  rc.maxTurns,
		OutboxBuf: -1, // no outbox: subagents use direct ThinkAct path
	})

	return ag, cleanup, nil
}


func (e *Executor) loadConversation(ag core.Agent, ctx context.Context, req AgentRequest) error {
	// Fork: inherit parent conversation context
	if len(req.ParentMessages) > 0 {
		if depth := countForkDepth(req.ParentMessages); depth >= maxForkDepth {
			return fmt.Errorf("maximum fork depth (%d) exceeded — forked agents cannot fork more than %d levels deep", maxForkDepth, maxForkDepth)
		}
		forked := prepareForkedMessages(req.ParentMessages)
		ag.SetMessages(forked)
		ag.Append(ctx, core.UserMessage(req.Prompt, nil))
		return nil
	}

	// Resume from saved session
	if req.ResumeID != "" {
		if err := e.resumeFromSession(ag, ctx, req.ResumeID, req.Prompt); err != nil {
			return fmt.Errorf("failed to resume agent: %w", err)
		}
		return nil
	}

	// Fresh start
	ag.Append(ctx, core.UserMessage(req.Prompt, nil))
	return nil
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
	e.hooks.ExecuteAsync(hook.SubagentStop, hook.HookInput{
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
		return resolveModelAlias(requestModel)
	}
	return e.parentModelID
}

// agentPermission maps PermissionMode to a perm.Checker.
func agentPermission(mode PermissionMode) perm.Checker {
	switch mode {
	case PermissionPlan:
		return perm.ReadOnly()
	case PermissionAcceptEdits:
		return perm.AcceptEdits()
	case PermissionDefault:
		return perm.PermitAll()
	default:
		return perm.PermitAll()
	}
}

// hasToolAccess returns true if the tool list includes the given tool.
// A nil list means all tools are accessible.
func hasToolAccess(tools ToolList, name string) bool {
	if tools == nil {
		return true
	}
	for _, t := range tools {
		if t == name {
			return true
		}
	}
	return false
}

// newAgentToolSet creates a tool.Set for subagents with the disallow set eagerly initialized.
func newAgentToolSet(allow, disallow []string, mcpGetter func() []core.ToolSchema) *tool.Set {
	s := &tool.Set{Allow: allow, Disallow: disallow, MCP: mcpGetter, IsAgent: true}
	s.InitDisallowSet()
	return s
}

// generateShortID creates a short random hex ID for background tasks.
func generateShortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
