package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconAgent = "a"

	// MaxAgentNestingDepth is the maximum allowed nesting depth for subagents.
	// This prevents infinite recursion when agents spawn other agents.
	MaxAgentNestingDepth = 5
)

// agentDepthKey is the context key used to track agent nesting depth.
type agentDepthKey struct{}

// AgentExecutor is the interface for executing agents
// This allows the Agent tool to be decoupled from the agent package
type AgentExecutor interface {
	// Run executes an agent in foreground and returns the result
	Run(ctx context.Context, req AgentExecRequest) (*AgentExecResult, error)

	// RunBackground executes an agent in background and returns task info
	RunBackground(req AgentExecRequest) (AgentTaskInfo, error)

	// GetAgentConfig returns configuration for an agent type
	GetAgentConfig(agentType string) (AgentConfigInfo, bool)

	// GetParentModelID returns the parent conversation's model ID
	GetParentModelID() string
}

// ProgressFunc is called when the agent makes progress
type ProgressFunc func(msg string)

// AgentExecRequest contains parameters for agent execution
type AgentExecRequest struct {
	Agent       string
	Name        string // Display name for the agent instance
	Prompt      string
	Description string
	Background  bool
	Model       string // Explicit model override (highest priority)
	MaxTurns    int
	Mode        string       // Per-invocation permission mode override
	ResumeID    string       // Agent ID to resume from
	Isolation   string       // Isolation mode (e.g., "worktree")
	TeamName    string       // Team name for spawning
	OnProgress  ProgressFunc // Called when agent makes progress
	OnQuestion  AskQuestionFunc
}

// AgentExecResult contains the result of agent execution
type AgentExecResult struct {
	AgentID     string // Session ID for resume
	AgentName   string
	Model       string // Model ID used for execution
	Success     bool
	Content     string
	TurnCount   int
	ToolUses    int
	TotalTokens int
	Duration    time.Duration
	Progress    []string // Intermediate progress messages (tool calls)
	Error       string
}

// AgentTaskInfo contains info about a background agent task
type AgentTaskInfo struct {
	TaskID    string
	AgentName string
}

// AgentConfigInfo contains agent configuration for display
type AgentConfigInfo struct {
	Name           string
	Description    string
	PermissionMode string
	Tools          []string
}

// AgentTool spawns subagents to handle complex tasks.
// It implements PermissionAwareTool to require user confirmation.
type AgentTool struct {
	Executor AgentExecutor
}

// NewAgentTool creates a new AgentTool
func NewAgentTool() *AgentTool {
	return &AgentTool{}
}

func (t *AgentTool) Name() string        { return "Agent" }
func (t *AgentTool) Description() string { return "Launch a subagent to handle complex tasks" }
func (t *AgentTool) Icon() string        { return IconAgent }

// RequiresPermission returns true - Agent always requires permission
func (t *AgentTool) RequiresPermission() bool {
	return true
}

// SetExecutor sets the agent executor
func (t *AgentTool) SetExecutor(executor AgentExecutor) {
	t.Executor = executor
}

// PreparePermission prepares a permission request with agent metadata
func (t *AgentTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error) {
	agentType := getString(params, "subagent_type")
	if agentType == "" {
		agentType = "general-purpose"
	}

	prompt, err := requireString(params, "prompt")
	if err != nil {
		return nil, err
	}

	description := getString(params, "description")
	if description == "" {
		description = "Run agent task"
	}

	runBackground := getBool(params, "run_in_background")
	requestModel := getString(params, "model")

	// Check if executor is configured
	if t.Executor == nil {
		return nil, fmt.Errorf("agent executor not configured")
	}

	// Get agent config
	config, ok := t.Executor.GetAgentConfig(agentType)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	// Determine effective model (priority: request > parent > fallback)
	effectiveModel := requestModel
	if effectiveModel == "" {
		effectiveModel = t.Executor.GetParentModelID()
	}
	if effectiveModel == "" {
		effectiveModel = "claude-sonnet-4-20250514" // fallback
	}

	// Build description
	desc := fmt.Sprintf("Spawn %s agent: %s", config.Name, description)
	if runBackground {
		desc += " (background)"
	}

	return &permission.PermissionRequest{
		ID:          generateRequestID(),
		ToolName:    t.Name(),
		Description: desc,
		AgentMeta: &permission.AgentMetadata{
			AgentName:      config.Name,
			Description:    config.Description,
			Model:          effectiveModel,
			PermissionMode: config.PermissionMode,
			Tools:          config.Tools,
			Prompt:         prompt,
			Background:     runBackground,
		},
	}, nil
}

// ExecuteApproved executes the agent after user approval
func (t *AgentTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return t.execute(ctx, params, cwd)
}

// Execute implements the Tool interface
func (t *AgentTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return t.execute(ctx, params, cwd)
}

// execute is the internal implementation
func (t *AgentTool) execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Check and enforce nesting depth limit to prevent infinite recursion.
	currentDepth := 0
	if d, ok := ctx.Value(agentDepthKey{}).(int); ok {
		currentDepth = d
	}
	if currentDepth >= MaxAgentNestingDepth {
		return ui.NewErrorResult(t.Name(), fmt.Sprintf(
			"maximum agent nesting depth (%d) exceeded — agents cannot spawn agents more than %d levels deep",
			MaxAgentNestingDepth, MaxAgentNestingDepth,
		))
	}
	// Pass incremented depth to child context so nested agents can detect it.
	ctx = context.WithValue(ctx, agentDepthKey{}, currentDepth+1)

	agentType := getString(params, "subagent_type")
	if agentType == "" {
		agentType = "general-purpose"
	}

	prompt := getString(params, "prompt")
	if prompt == "" {
		return ui.NewErrorResult(t.Name(), "prompt is required")
	}

	description := getString(params, "description")
	agentName := getString(params, "name")
	runBackground := getBool(params, "run_in_background")
	model := getString(params, "model")
	mode := getString(params, "mode")
	resumeID := getString(params, "resume")
	isolation := getString(params, "isolation")
	teamName := getString(params, "team_name")

	var onProgress ProgressFunc
	if cb, ok := params["_onProgress"].(ProgressFunc); ok {
		onProgress = cb
	}
	var onQuestion AskQuestionFunc
	if cb, ok := params["_onQuestion"].(AskQuestionFunc); ok {
		onQuestion = cb
	}

	maxTurns := getInt(params, "max_turns", 0)

	// Check executor
	if t.Executor == nil {
		return ui.NewErrorResult(t.Name(), "agent executor not configured")
	}

	// Build request
	req := AgentExecRequest{
		Agent:       agentType,
		Name:        agentName,
		Prompt:      prompt,
		Description: description,
		Background:  runBackground,
		Model:       model,
		MaxTurns:    maxTurns,
		Mode:        mode,
		ResumeID:    resumeID,
		Isolation:   isolation,
		TeamName:    teamName,
		OnProgress:  onProgress,
		OnQuestion:  onQuestion,
	}

	// Handle background execution
	if runBackground {
		taskInfo, err := t.Executor.RunBackground(req)
		if err != nil {
			return ui.NewErrorResult(t.Name(), fmt.Sprintf("failed to start background agent: %v", err))
		}

		duration := time.Since(start)
		return ui.ToolResult{
			Success: true,
			Output: fmt.Sprintf("Agent started in background.\nTask ID: %s\nAgent: %s\nDescription: %s\n\nUse TaskOutput with task_id=\"%s\" to check the result.",
				taskInfo.TaskID, taskInfo.AgentName, description, taskInfo.TaskID),
			Metadata: ui.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: fmt.Sprintf("[background] %s: %s", agentType, taskInfo.TaskID),
				Duration: duration,
			},
		}
	}

	// Foreground execution
	result, err := t.Executor.Run(ctx, req)
	if err != nil {
		return ui.NewErrorResult(t.Name(), fmt.Sprintf("agent execution failed: %v", err))
	}

	duration := time.Since(start)

	if !result.Success {
		hookResponse := buildAgentHookResponse(result, agentType, prompt)
		return ui.ToolResult{
			Success:      false,
			Output:       result.Content,
			Error:        result.Error,
			HookResponse: hookResponse,
			Metadata: ui.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: fmt.Sprintf("%s: failed", agentType),
				Duration: duration,
			},
		}
	}

	// Format output with structured metadata for TUI rendering
	displayName := result.AgentName
	if displayName == "" {
		displayName = agentType
	}
	agentDuration := result.Duration
	if agentDuration == 0 {
		agentDuration = duration
	}
	var outputBuilder strings.Builder
	fmt.Fprintf(&outputBuilder, "Agent: %s\nModel: %s\nTurns: %d\nToolUses: %d\nTokens: %d\nDuration: %s\n",
		displayName, result.Model, result.TurnCount, result.ToolUses, result.TotalTokens, FormatDuration(agentDuration))
	if result.AgentID != "" {
		fmt.Fprintf(&outputBuilder, "AgentID: %s\n", result.AgentID)
	}

	// Include process count as metadata, then process lines + response after blank line
	if len(result.Progress) > 0 {
		fmt.Fprintf(&outputBuilder, "Process: %d\n", len(result.Progress))
	}
	outputBuilder.WriteString("\n")
	if len(result.Progress) > 0 {
		for _, p := range result.Progress {
			outputBuilder.WriteString(p)
			outputBuilder.WriteString("\n")
		}
	}
	if result.Content != "" {
		outputBuilder.WriteString(result.Content)
	}

	hookResponse := buildAgentHookResponse(result, agentType, prompt)
	return ui.ToolResult{
		Success:      true,
		Output:       outputBuilder.String(),
		HookResponse: hookResponse,
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("%s: done (%d turns)", agentType, result.TurnCount),
			Duration: duration,
		},
	}
}

// buildAgentHookResponse creates a CC-compatible structured response for PostToolUse hooks.
func buildAgentHookResponse(result *AgentExecResult, agentType, prompt string) map[string]any {
	status := "completed"
	if !result.Success {
		status = "error"
	}

	return map[string]any{
		"agentId":           result.AgentID,
		"agentType":         agentType,
		"content":           result.Content,
		"status":            status,
		"prompt":            prompt,
		"totalDurationMs":   result.Duration.Milliseconds(),
		"totalTokens":       result.TotalTokens,
		"totalToolUseCount": result.ToolUses,
		"usage": map[string]any{
			"total_tokens": result.TotalTokens,
		},
	}
}

// FormatDuration formats a duration as human-readable string (e.g., "2m 30s", "45s")
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

func init() {
	Register(NewAgentTool())
}
