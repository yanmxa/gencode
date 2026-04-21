package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

type continuedAgentTarget struct {
	taskID    string
	agentID   string
	agentType string
	name      string
}

const IconContinueAgent = tool.IconAgent

// ContinueAgentTool resumes a previously spawned worker from saved session state.
type ContinueAgentTool struct {
	executor tool.AgentExecutor
}

func NewContinueAgentTool() *ContinueAgentTool {
	return &ContinueAgentTool{}
}

func (t *ContinueAgentTool) Name() string { return tool.ToolContinueAgent }
func (t *ContinueAgentTool) Description() string {
	return "Continue a previously spawned subagent from saved conversation state"
}
func (t *ContinueAgentTool) Icon() string { return IconContinueAgent }

func (t *ContinueAgentTool) RequiresPermission() bool { return true }

func (t *ContinueAgentTool) SetExecutor(executor tool.AgentExecutor) {
	t.executor = executor
}

func (t *ContinueAgentTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*perm.PermissionRequest, error) {
	prompt, err := tool.RequireString(params, "prompt")
	if err != nil {
		return nil, err
	}
	if t.executor == nil {
		return nil, fmt.Errorf("agent executor not configured")
	}

	target, err := resolveContinuationTarget(params)
	if err != nil {
		return nil, err
	}
	if target.taskID != "" {
		if err := ensureContinuationTaskStopped(target.taskID, "ContinueAgent"); err != nil {
			return nil, err
		}
	}

	config, ok := t.executor.GetAgentConfig(target.agentType)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", target.agentType)
	}

	description := tool.GetString(params, "description")
	if description == "" {
		description = "Continue agent task"
	}

	runBackground := tool.GetBool(params, "run_in_background")
	effectiveModel := tool.GetString(params, "model")
	if effectiveModel == "" {
		effectiveModel = t.executor.GetParentModelID()
	}
	if effectiveModel == "" {
		effectiveModel = "claude-sonnet-4-20250514"
	}

	desc := fmt.Sprintf("Continue %s agent: %s", config.Name, description)
	if target.taskID != "" {
		desc += fmt.Sprintf(" (task %s)", target.taskID)
	}
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

func (t *ContinueAgentTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return t.execute(ctx, params, cwd)
}

func (t *ContinueAgentTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return t.execute(ctx, params, cwd)
}

func (t *ContinueAgentTool) execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	start := time.Now()

	currentDepth := tool.GetAgentDepth(ctx)
	if currentDepth >= tool.MaxAgentNestingDepth {
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf(
			"maximum agent nesting depth (%d) exceeded — agents cannot spawn agents more than %d levels deep",
			tool.MaxAgentNestingDepth, tool.MaxAgentNestingDepth,
		))
	}
	ctx = tool.WithAgentDepth(ctx, currentDepth+1)

	if t.executor == nil {
		return toolresult.NewErrorResult(t.Name(), "agent executor not configured")
	}

	prompt := tool.GetString(params, "prompt")
	if prompt == "" {
		return toolresult.NewErrorResult(t.Name(), "prompt is required")
	}

	target, err := resolveContinuationTarget(params)
	if err != nil {
		return toolresult.NewErrorResult(t.Name(), err.Error())
	}
	if target.taskID != "" {
		if err := ensureContinuationTaskStopped(target.taskID, t.Name()); err != nil {
			return toolresult.NewErrorResult(t.Name(), err.Error())
		}
	}

	description := tool.GetString(params, "description")
	if description == "" {
		description = "Continue agent task"
	}

	agentName := tool.GetString(params, "name")
	if agentName == "" {
		agentName = target.name
	}

	var onProgress tool.ProgressFunc
	if cb, ok := params["_onProgress"].(tool.ProgressFunc); ok {
		onProgress = cb
	}
	var onQuestion tool.AskQuestionFunc
	if cb, ok := params["_onQuestion"].(tool.AskQuestionFunc); ok {
		onQuestion = cb
	}

	req := tool.AgentExecRequest{
		Agent:       target.agentType,
		Name:        agentName,
		Prompt:      prompt,
		Description: description,
		Background:  tool.GetBool(params, "run_in_background"),
		Model:       tool.GetString(params, "model"),
		MaxTurns:    tool.GetInt(params, "max_turns", 0),
		Mode:        tool.GetString(params, "mode"),
		ResumeID:    target.agentID,
		Isolation:   tool.GetString(params, "isolation"),
		OnProgress:  onProgress,
		OnQuestion:  onQuestion,
	}

	if req.Background {
		taskInfo, err := t.executor.RunBackground(req)
		if err != nil {
			return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("failed to continue background agent: %v", err))
		}

		return toolresult.ToolResult{
			Success: true,
			Output: fmt.Sprintf("Agent continuation started in background.\nTask ID: %s\nAgent: %s\nContinuation of: %s\nDescription: %s"+backgroundLaunchSuffix,
				taskInfo.TaskID, taskInfo.AgentName, target.agentID, description),
			HookResponse: map[string]any{
				"backgroundTask": map[string]any{
					"taskId":      taskInfo.TaskID,
					"agentName":   taskInfo.AgentName,
					"agentType":   target.agentType,
					"description": description,
					"outputFile":  taskInfo.OutputFile,
					"resumeId":    target.agentID,
					"toolName":    t.Name(),
				},
			},
			Metadata: toolresult.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: fmt.Sprintf("[background] %s: %s", target.agentType, taskInfo.TaskID),
				Duration: time.Since(start),
			},
		}
	}

	result, err := t.executor.Run(ctx, req)
	if err != nil {
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("agent continuation failed: %v", err))
	}

	duration := time.Since(start)
	if !result.Success {
		return toolresult.ToolResult{
			Success: false,
			Output:  result.Content,
			Error:   result.Error,
			Metadata: toolresult.ResultMetadata{
				Title:    t.Name(),
				Icon:     t.Icon(),
				Subtitle: fmt.Sprintf("%s: failed", target.agentType),
				Duration: duration,
			},
		}
	}

	return toolresult.ToolResult{
		Success: true,
		Output:  formatForegroundAgentResult(target.agentType, result, duration),
		Metadata: toolresult.ResultMetadata{
			Title:    t.Name(),
			Icon:     t.Icon(),
			Subtitle: fmt.Sprintf("%s: done (%d turns)", target.agentType, result.TurnCount),
			Duration: duration,
		},
	}
}

func resolveContinuationTarget(params map[string]any) (continuedAgentTarget, error) {
	taskID := tool.GetString(params, "task_id")
	agentID := tool.GetString(params, "agent_id")
	agentType := tool.GetString(params, "subagent_type")

	if taskID == "" && agentID == "" {
		return continuedAgentTarget{}, fmt.Errorf("task_id or agent_id is required")
	}

	target := continuedAgentTarget{
		taskID:    taskID,
		agentID:   agentID,
		agentType: agentType,
	}

	if taskID == "" {
		if target.agentType == "" {
			return continuedAgentTarget{}, fmt.Errorf("subagent_type is required when continuing by agent_id")
		}
		return target, nil
	}

	bgTask, found := task.Default().Get(taskID)
	if !found {
		return continuedAgentTarget{}, fmt.Errorf("task not found: %s", taskID)
	}

	info := bgTask.GetStatus()
	if info.Type != task.TaskTypeAgent {
		return continuedAgentTarget{}, fmt.Errorf("task %s is not an agent task", taskID)
	}
	if info.AgentSessionID == "" {
		return continuedAgentTarget{}, fmt.Errorf("task %s has no resumable agent_id yet", taskID)
	}

	target.agentID = info.AgentSessionID
	target.name = info.AgentName
	if target.agentType == "" {
		target.agentType = info.AgentType
	}
	if target.agentType == "" {
		target.agentType = "general-purpose"
	}
	return target, nil
}

func ensureContinuationTaskStopped(taskID, toolName string) error {
	bgTask, found := task.Default().Get(taskID)
	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if bgTask.IsRunning() {
		return fmt.Errorf("task %s is still running; %s requires a stopped worker — wait for completion first", taskID, toolName)
	}
	return nil
}

func formatForegroundAgentResult(agentType string, result *tool.AgentExecResult, duration time.Duration) string {
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
		displayName, result.Model, result.TurnCount, result.ToolUses, result.TotalTokens, toolresult.FormatDuration(agentDuration))
	if result.AgentID != "" {
		fmt.Fprintf(&outputBuilder, "AgentID: %s\n", result.AgentID)
	}
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
	return outputBuilder.String()
}

func init() {
	tool.Register(NewContinueAgentTool())
}
