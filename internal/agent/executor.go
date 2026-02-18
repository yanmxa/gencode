package agent

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"go.uber.org/zap"
)

// Executor runs agent LLM loops
type Executor struct {
	provider      provider.LLMProvider
	cwd           string
	parentModelID string // Parent conversation's model ID (used when inheriting)
	hooks         *hooks.Engine
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

// GetParentModelID returns the parent model ID
func (e *Executor) GetParentModelID() string {
	return e.parentModelID
}

// Run executes an agent request and returns the result.
// For background agents, this should be called in a goroutine.
func (e *Executor) Run(ctx context.Context, req AgentRequest) (*AgentResult, error) {
	start := time.Now()

	// Get agent configuration
	config, ok := DefaultRegistry.Get(req.Agent)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", req.Agent)
	}

	// Determine model to use (priority: request > parent > fallback)
	modelID := e.resolveModelID(req.Model)

	// Determine max turns
	maxTurns := config.MaxTurns
	if req.MaxTurns > 0 {
		maxTurns = req.MaxTurns
	}
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}

	// Build the agent-specific system prompt as Extra
	agentPrompt := e.buildSystemPrompt(config, req)

	// Create turn tracker for hierarchical DEV_DIR logging
	tracker := log.NewAgentTurnTracker(config.Name, nil)
	ctx = log.WithAgentTracker(ctx, tracker)

	log.Logger().Info("Starting agent execution",
		zap.String("agent", config.Name),
		zap.String("description", req.Description),
		zap.Int("maxTurns", maxTurns),
	)

	// Build core.Loop
	c := &client.Client{Provider: e.provider, Model: modelID, MaxTokens: 8192}
	loop := &core.Loop{
		System:     &system.System{Client: c, Cwd: e.cwd, Extra: []string{agentPrompt}},
		Client:     c,
		Tool:       &tool.Set{Access: convertToolAccess(config.Tools)},
		Permission: agentPermission(config.PermissionMode),
		Hooks:      e.hooks,
	}
	loop.AddUser(req.Prompt, nil)

	// Create progress callback
	onToolStart := func(tc message.ToolCall) bool {
		if req.OnProgress != nil {
			params, _ := message.ParseToolInput(tc.Input)
			progressMsg := formatToolProgress(tc.Name, params)
			req.OnProgress(progressMsg)
		}
		return true
	}

	result, err := loop.Run(ctx, core.RunOptions{MaxTurns: maxTurns, OnToolStart: onToolStart})
	if err != nil {
		if result != nil && result.StopReason == "cancelled" {
			return &AgentResult{
				AgentName:  config.Name,
				Success:    false,
				Content:    result.Content,
				Messages:   result.Messages,
				TurnCount:  result.Turns,
				TokenUsage: result.Tokens,
				Duration:   time.Since(start),
				Error:      "agent cancelled",
			}, err
		}
		return nil, fmt.Errorf("LLM completion failed: %w", err)
	}

	success := result.StopReason == "end_turn"
	errMsg := ""
	if result.StopReason == "max_turns" {
		errMsg = fmt.Sprintf("reached maximum turns (%d)", maxTurns)
	}

	logFields := []zap.Field{
		zap.String("agent", config.Name),
		zap.String("stopReason", result.StopReason),
		zap.Int("turns", result.Turns),
		zap.Int("inputTokens", result.Tokens.InputTokens),
		zap.Int("outputTokens", result.Tokens.OutputTokens),
	}
	if success {
		log.Logger().Info("Agent completed", logFields...)
	} else {
		log.Logger().Warn("Agent completed", logFields...)
	}

	return &AgentResult{
		AgentName:  config.Name,
		Success:    success,
		Content:    result.Content,
		Messages:   result.Messages,
		TurnCount:  result.Turns,
		TokenUsage: result.Tokens,
		Duration:   time.Since(start),
		Error:      errMsg,
	}, nil
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

	// Create agent task
	agentTask := task.NewAgentTask(
		task.GenerateID(),
		config.Name,
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

// buildSystemPrompt builds the agent-specific system prompt.
func (e *Executor) buildSystemPrompt(config *AgentConfig, req AgentRequest) string {
	var sb strings.Builder

	// Base agent identity
	sb.WriteString("You are a specialized AI agent within GenCode, an AI coding assistant.\n\n")

	// Agent-specific prompt
	sb.WriteString(fmt.Sprintf("## Agent Type: %s\n", config.Name))
	sb.WriteString(config.Description)
	sb.WriteString("\n\n")

	// Task context
	sb.WriteString("## Your Task\n")
	sb.WriteString(req.Prompt)
	sb.WriteString("\n\n")

	// Mode-specific instructions
	switch config.PermissionMode {
	case PermissionPlan:
		sb.WriteString("## Mode: Read-Only\n")
		sb.WriteString("You are in read-only mode. You can only use tools that read information (Read, Glob, Grep, WebFetch, WebSearch). Do not attempt to modify any files.\n\n")
	case PermissionDontAsk:
		sb.WriteString("## Mode: Autonomous\n")
		sb.WriteString("You have full autonomy to complete your task. You can read and modify files, execute commands, and make changes as needed.\n\n")
	}

	// Custom system prompt from config (lazily loaded)
	if sysPrompt := config.GetSystemPrompt(); sysPrompt != "" {
		sb.WriteString("## Additional Instructions\n")
		sb.WriteString(sysPrompt)
		sb.WriteString("\n\n")
	}

	// Environment info
	sb.WriteString("## Environment\n")
	sb.WriteString(fmt.Sprintf("- Working directory: %s\n", e.cwd))
	sb.WriteString(fmt.Sprintf("- Platform: %s\n", runtime.GOOS))
	sb.WriteString(fmt.Sprintf("- Date: %s\n", time.Now().Format("2006-01-02")))
	sb.WriteString("\n")

	// Guidelines
	sb.WriteString("## Guidelines\n")
	sb.WriteString("- Focus on completing your assigned task efficiently\n")
	sb.WriteString("- Return a clear summary when your task is complete\n")
	sb.WriteString("- If you encounter errors, report them clearly\n")

	return sb.String()
}

// toolProgressFormats maps tool names to their progress format templates and param keys.
var toolProgressFormats = map[string]struct {
	verb  string
	param string
}{
	"Read":      {"Reading", "file_path"},
	"Glob":      {"Finding", "pattern"},
	"Grep":      {"Searching", "pattern"},
	"WebFetch":  {"Fetching", "url"},
	"WebSearch": {"Searching web", "query"},
	"Bash":      {"Running", "command"},
}

// formatToolProgress creates a progress message for a tool call.
func formatToolProgress(toolName string, params map[string]any) string {
	format, ok := toolProgressFormats[toolName]
	if !ok {
		return fmt.Sprintf("Executing: %s", toolName)
	}

	value, ok := params[format.param].(string)
	if !ok {
		return fmt.Sprintf("Executing: %s", toolName)
	}

	// Truncate long command strings
	if toolName == "Bash" && len(value) > 50 {
		value = value[:47] + "..."
	}

	return fmt.Sprintf("%s: %s", format.verb, value)
}

// --- Internal helpers ---

// agentPermission maps PermissionMode to a permission.Checker.
func agentPermission(mode PermissionMode) permission.Checker {
	if mode == PermissionPlan {
		return permission.ReadOnly()
	}
	return permission.PermitAll()
}


// convertToolAccess converts agent.ToolAccess to tool.AccessConfig.
func convertToolAccess(ta ToolAccess) *tool.AccessConfig {
	return &tool.AccessConfig{
		Mode:  tool.AccessMode(ta.Mode),
		Allow: ta.Allow,
		Deny:  ta.Deny,
	}
}
