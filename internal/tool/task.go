package tool

import (
	"context"
	"fmt"
	"time"

	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tool/ui"
)

const (
	IconTask = "t"
)

// AgentExecutor is the interface for executing agents
// This allows the Task tool to be decoupled from the agent package
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
	Prompt      string
	Description string
	Background  bool
	ResumeID    string
	Model       string // Explicit model override (highest priority)
	MaxTurns    int
	Cwd         string
	OnProgress  ProgressFunc // Called when agent makes progress
}

// AgentExecResult contains the result of agent execution
type AgentExecResult struct {
	AgentName  string
	Success    bool
	Content    string
	TurnCount  int
	TotalTokens int
	Error      string
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

// TaskTool spawns subagents to handle complex tasks
// It implements PermissionAwareTool to require user confirmation
type TaskTool struct {
	// Executor runs agent tasks
	Executor AgentExecutor
}

// NewTaskTool creates a new TaskTool
func NewTaskTool() *TaskTool {
	return &TaskTool{}
}

func (t *TaskTool) Name() string        { return "Task" }
func (t *TaskTool) Description() string { return "Launch a subagent to handle complex tasks" }
func (t *TaskTool) Icon() string        { return IconTask }

// RequiresPermission returns true - Task always requires permission
func (t *TaskTool) RequiresPermission() bool {
	return true
}

// SetExecutor sets the agent executor
func (t *TaskTool) SetExecutor(executor AgentExecutor) {
	t.Executor = executor
}

// PreparePermission prepares a permission request with agent metadata
func (t *TaskTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error) {
	// Get agent type
	agentType, ok := params["subagent_type"].(string)
	if !ok || agentType == "" {
		return nil, fmt.Errorf("subagent_type is required")
	}

	// Get prompt
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Get optional parameters
	description, _ := params["description"].(string)
	if description == "" {
		description = "Run agent task"
	}

	runBackground, _ := params["run_in_background"].(bool)
	requestModel, _ := params["model"].(string)

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
func (t *TaskTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return t.execute(ctx, params, cwd)
}

// Execute implements the Tool interface
func (t *TaskTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return t.execute(ctx, params, cwd)
}

// execute is the internal implementation
func (t *TaskTool) execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	start := time.Now()

	// Get agent type
	agentType, ok := params["subagent_type"].(string)
	if !ok || agentType == "" {
		return ui.NewErrorResult(t.Name(), "subagent_type is required")
	}

	// Get prompt
	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return ui.NewErrorResult(t.Name(), "prompt is required")
	}

	// Get optional parameters
	description, _ := params["description"].(string)
	runBackground, _ := params["run_in_background"].(bool)
	resumeID, _ := params["resume"].(string)
	model, _ := params["model"].(string)

	// Get progress callback (set by TUI)
	var onProgress ProgressFunc
	if cb, ok := params["_onProgress"].(ProgressFunc); ok {
		onProgress = cb
	}

	// Get max turns
	maxTurns := 0
	if mt, ok := params["max_turns"].(float64); ok {
		maxTurns = int(mt)
	}

	// Check executor
	if t.Executor == nil {
		return ui.NewErrorResult(t.Name(), "agent executor not configured")
	}

	// Build request
	req := AgentExecRequest{
		Agent:       agentType,
		Prompt:      prompt,
		Description: description,
		Background:  runBackground,
		ResumeID:    resumeID,
		Model:       model,
		MaxTurns:    maxTurns,
		Cwd:         cwd,
		OnProgress:  onProgress,
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
		return ui.ToolResult{
			Success: false,
			Output:  result.Content,
			Error:   result.Error,
			Metadata: ui.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: fmt.Sprintf("%s: failed", agentType),
				Duration: duration,
			},
		}
	}

	// Format output
	output := result.Content
	if output == "" {
		output = fmt.Sprintf("Agent completed successfully.\nTurns: %d\nTokens: %d",
			result.TurnCount, result.TotalTokens)
	}

	return ui.ToolResult{
		Success: true,
		Output:  output,
		Metadata: ui.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("%s: done (%d turns)", agentType, result.TurnCount),
			Duration: duration,
		},
	}
}

func init() {
	Register(NewTaskTool())
}
