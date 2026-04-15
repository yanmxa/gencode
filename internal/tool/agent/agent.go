package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

const backgroundLaunchSuffix = "\n\nThe agent is working in the background. You will be notified automatically when it completes.\nBriefly tell the user what you launched and end your response. Do not generate any other text — agent results will arrive in a subsequent message."

// AgentTool spawns subagents to handle complex tasks.
// It implements PermissionAwareTool to require user confirmation.
type AgentTool struct {
	Executor tool.AgentExecutor
}

// NewAgentTool creates a new AgentTool
func NewAgentTool() *AgentTool {
	return &AgentTool{}
}

func (t *AgentTool) Name() string        { return "Agent" }
func (t *AgentTool) Description() string { return "Launch a subagent to handle complex tasks" }
func (t *AgentTool) Icon() string        { return tool.IconAgent }

// RequiresPermission returns true - Agent always requires permission
func (t *AgentTool) RequiresPermission() bool {
	return true
}

// SetExecutor sets the agent executor
func (t *AgentTool) SetExecutor(executor tool.AgentExecutor) {
	t.Executor = executor
}

// PreparePermission prepares a permission request with agent metadata
func (t *AgentTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*perm.PermissionRequest, error) {
	agentType := tool.GetString(params, "subagent_type")
	if agentType == "" {
		agentType = "general-purpose"
	}

	prompt, err := tool.RequireString(params, "prompt")
	if err != nil {
		return nil, err
	}

	description := tool.GetString(params, "description")
	if description == "" {
		description = "Run agent task"
	}

	runBackground := tool.GetBool(params, "run_in_background")
	requestModel := tool.GetString(params, "model")

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

	return &perm.PermissionRequest{
		ID:          tool.GenerateRequestID(),
		ToolName:    t.Name(),
		Description: desc,
		AgentMeta: &perm.AgentMetadata{
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
func (t *AgentTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return t.execute(ctx, params, cwd)
}

// Execute implements the Tool interface
func (t *AgentTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return t.execute(ctx, params, cwd)
}

// execute is the internal implementation
func (t *AgentTool) execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	start := time.Now()

	// Check and enforce nesting depth limit to prevent infinite recursion.
	currentDepth := tool.GetAgentDepth(ctx)
	if currentDepth >= tool.MaxAgentNestingDepth {
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf(
			"maximum agent nesting depth (%d) exceeded — agents cannot spawn agents more than %d levels deep",
			tool.MaxAgentNestingDepth, tool.MaxAgentNestingDepth,
		))
	}
	// Pass incremented depth to child context so nested agents can detect it.
	ctx = tool.WithAgentDepth(ctx, currentDepth+1)

	agentType := tool.GetString(params, "subagent_type")
	if agentType == "" {
		agentType = "general-purpose"
	}

	prompt := tool.GetString(params, "prompt")
	if prompt == "" {
		return toolresult.NewErrorResult(t.Name(), "prompt is required")
	}

	description := tool.GetString(params, "description")
	agentName := tool.GetString(params, "name")
	runBackground := tool.GetBool(params, "run_in_background")
	model := tool.GetString(params, "model")
	mode := tool.GetString(params, "mode")
	resumeID := tool.GetString(params, "resume")
	isolation := tool.GetString(params, "isolation")
	teamName := tool.GetString(params, "team_name")
	fork := tool.GetBool(params, "fork")

	var onProgress tool.ProgressFunc
	if cb, ok := params["_onProgress"].(tool.ProgressFunc); ok {
		onProgress = cb
	}
	var onQuestion tool.AskQuestionFunc
	if cb, ok := params["_onQuestion"].(tool.AskQuestionFunc); ok {
		onQuestion = cb
	}

	maxTurns := tool.GetInt(params, "max_turns", 0)

	// Check executor
	if t.Executor == nil {
		return toolresult.NewErrorResult(t.Name(), "agent executor not configured")
	}

	// Validate fork + resume are mutually exclusive
	if fork && resumeID != "" {
		return toolresult.NewErrorResult(t.Name(), "fork and resume cannot be used together")
	}

	// Resolve parent messages for fork
	var parentMessages []core.Message
	if fork {
		if getter, ok := params["_messagesGetter"].(tool.MessagesGetter); ok {
			parentMessages = getter()
		}
		if len(parentMessages) == 0 {
			return toolresult.NewErrorResult(t.Name(), "fork requires parent conversation context but none is available")
		}
	}

	// Build request
	req := tool.AgentExecRequest{
		Agent:          agentType,
		Name:           agentName,
		Prompt:         prompt,
		Description:    description,
		Background:     runBackground,
		Model:          model,
		MaxTurns:       maxTurns,
		Mode:           mode,
		ResumeID:       resumeID,
		Isolation:      isolation,
		TeamName:       teamName,
		ParentMessages: parentMessages,
		OnProgress:     onProgress,
		OnQuestion:     onQuestion,
	}

	// Handle background execution
	if runBackground {
		taskInfo, err := t.Executor.RunBackground(req)
		if err != nil {
			return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("failed to start background agent: %v", err))
		}

		duration := time.Since(start)
		return toolresult.ToolResult{
			Success: true,
			Output: fmt.Sprintf("Agent started in background.\nTask ID: %s\nAgent: %s\nDescription: %s"+backgroundLaunchSuffix,
				taskInfo.TaskID, taskInfo.AgentName, description),
			HookResponse: map[string]any{
				"backgroundTask": map[string]any{
					"taskId":      taskInfo.TaskID,
					"agentName":   taskInfo.AgentName,
					"agentType":   agentType,
					"description": description,
					"outputFile":  taskInfo.OutputFile,
					"toolName":    t.Name(),
				},
			},
			Metadata: toolresult.ResultMetadata{
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
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("agent execution failed: %v", err))
	}

	duration := time.Since(start)

	if !result.Success {
		hookResponse := buildAgentHookResponse(result, agentType, prompt)
		return toolresult.ToolResult{
			Success:      false,
			Output:       result.Content,
			Error:        result.Error,
			HookResponse: hookResponse,
			Metadata: toolresult.ResultMetadata{
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
		displayName, result.Model, result.TurnCount, result.ToolUses, result.TotalTokens, tool.FormatDuration(agentDuration))
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
	return toolresult.ToolResult{
		Success:      true,
		Output:       outputBuilder.String(),
		HookResponse: hookResponse,
		Metadata: toolresult.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("%s: done (%d turns)", agentType, result.TurnCount),
			Duration: duration,
		},
	}
}

// buildAgentHookResponse creates a CC-compatible structured response for PostToolUse hooks.
func buildAgentHookResponse(result *tool.AgentExecResult, agentType, prompt string) map[string]any {
	status := "completed"
	if !result.Success {
		status = "error"
	}

	return map[string]any{
		"agentId":           result.AgentID,
		"agentType":         agentType,
		"outputFile":        result.OutputFile,
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

func init() {
	tool.Register(NewAgentTool())
}
