package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/extension/mcp"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/worktree"
	"go.uber.org/zap"
)

// Executor runs agent LLM loops
type Executor struct {
	provider            provider.Provider
	cwd                 string
	parentModelID       string // Parent conversation's model ID (used when inheriting)
	hooks               *hooks.Engine
	sessionStore        SubagentSessionStore        // Optional: when set, subagent sessions are persisted
	parentSessionID     string                      // Parent session ID for linking subagent sessions
	userInstructions    string                      // ~/.gen/GEN.md + rules
	projectInstructions string                      // .gen/GEN.md + rules + local
	isGit               bool                        // whether cwd is a git repository
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
func NewExecutor(llmProvider provider.Provider, cwd string, parentModelID string, hookEngine *hooks.Engine) *Executor {
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
		generateShortID(),
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

	var mcpCaller runtime.MCPCaller
	if e.mcpRegistry != nil {
		mcpCaller = mcp.NewCaller(e.mcpRegistry)
	}

	l, err := runtime.NewLoop(runtime.LoopConfig{
		System: system.Build(system.Config{
			ProviderName:        e.provider.Name(),
			ModelID:             rc.modelID,
			Cwd:                 agentCwd,
			IsGit:               e.isGit,
			PlanMode:            rc.permMode == PermissionPlan,
			UserInstructions:    e.userInstructions,
			ProjectInstructions: e.projectInstructions,
			Extra:               []string{rc.agentPrompt},
		}),
		Client:     provider.NewClient(e.provider, rc.modelID, 0),
		Tool:       newAgentToolSet([]string(rc.config.Tools), []string(rc.config.DisallowedTools), e.mcpGetter),
		Permission: agentPermission(rc.permMode),
		Hooks:      e.hooks,
		MCP:        mcpCaller,
		Cwd:        agentCwd,
	})
	if err != nil {
		cleanup()
		return nil, cleanup, err
	}

	return l, cleanup, nil
}

func (e *Executor) loadConversation(lp *runtime.Loop, req AgentRequest) error {
	// Fork: inherit parent conversation context
	if len(req.ParentMessages) > 0 {
		// Guard against recursive forking
		if depth := countForkDepth(req.ParentMessages); depth >= maxForkDepth {
			return fmt.Errorf("maximum fork depth (%d) exceeded — forked agents cannot fork more than %d levels deep", maxForkDepth, maxForkDepth)
		}
		lp.SetMessages(prepareForkedMessages(req.ParentMessages))
		lp.AddUser(req.Prompt, nil)
		return nil
	}

	// Resume from saved session
	if req.ResumeID != "" {
		if err := e.resumeFromSession(lp, req.ResumeID, req.Prompt); err != nil {
			return fmt.Errorf("failed to resume agent: %w", err)
		}
		return nil
	}

	// Fresh start
	lp.AddUser(req.Prompt, nil)
	return nil
}

func (e *Executor) buildOnToolStart(req AgentRequest, allProgress *[]string) func(tc core.ToolCall) bool {
	return func(tc core.ToolCall) bool {
		params, _ := core.ParseToolInput(tc.Input)
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
		return resolveModelAlias(requestModel)
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
