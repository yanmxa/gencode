package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// SendMessageTool sends a follow-up message to an existing worker.
// Running workers do not yet support live injection, so messages are queued
// and delivered on the next resume.
type SendMessageTool struct {
	Executor tool.AgentExecutor
}

func NewSendMessageTool() *SendMessageTool {
	return &SendMessageTool{}
}

func (t *SendMessageTool) Name() string { return tool.ToolSendMessage }
func (t *SendMessageTool) Description() string {
	return "Send a follow-up message to an existing subagent worker"
}
func (t *SendMessageTool) Icon() string             { return tool.IconAgent }
func (t *SendMessageTool) RequiresPermission() bool { return true }

func (t *SendMessageTool) SetExecutor(executor tool.AgentExecutor) {
	t.Executor = executor
}

func (t *SendMessageTool) PreparePermission(ctx context.Context, params map[string]any, cwd string) (*perm.PermissionRequest, error) {
	normalized := normalizeSendMessageParams(params)
	messageText, err := tool.RequireString(normalized, "prompt")
	if err != nil {
		return nil, err
	}
	if t.Executor == nil {
		return nil, fmt.Errorf("agent executor not configured")
	}

	target, err := resolveContinuationTargetForMessage(normalized)
	if err != nil {
		return nil, err
	}
	if target.taskID != "" {
		if err := ensureContinuationTaskExists(target.taskID); err != nil {
			return nil, err
		}
	}

	config, ok := t.Executor.GetAgentConfig(target.agentType)
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", target.agentType)
	}

	description := tool.GetString(normalized, "description")
	if description == "" {
		description = "Message existing worker"
	}

	runBackground := tool.GetBool(normalized, "run_in_background")
	effectiveModel := tool.GetString(normalized, "model")
	if effectiveModel == "" {
		effectiveModel = t.Executor.GetParentModelID()
	}
	if effectiveModel == "" {
		effectiveModel = "claude-sonnet-4-20250514"
	}

	desc := fmt.Sprintf("Send message to %s agent: %s", config.Name, description)
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
			Prompt:         messageText,
			Background:     runBackground,
		},
	}, nil
}

func (t *SendMessageTool) ExecuteApproved(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return t.execute(ctx, params, cwd)
}

func (t *SendMessageTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return t.execute(ctx, params, cwd)
}

func (t *SendMessageTool) execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	start := time.Now()

	currentDepth := tool.GetAgentDepth(ctx)
	if currentDepth >= tool.MaxAgentNestingDepth {
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf(
			"maximum agent nesting depth (%d) exceeded — agents cannot spawn agents more than %d levels deep",
			tool.MaxAgentNestingDepth, tool.MaxAgentNestingDepth,
		))
	}
	ctx = tool.WithAgentDepth(ctx, currentDepth+1)

	if t.Executor == nil {
		return toolresult.NewErrorResult(t.Name(), "agent executor not configured")
	}

	normalized := normalizeSendMessageParams(params)
	messageText := tool.GetString(normalized, "prompt")
	if messageText == "" {
		return toolresult.NewErrorResult(t.Name(), "message is required")
	}

	target, err := resolveContinuationTargetForMessage(normalized)
	if err != nil {
		return toolresult.NewErrorResult(t.Name(), err.Error())
	}
	resolvedTaskID := resolveLiveTaskID(target)
	if resolvedTaskID != "" {
		running, err := isContinuationTaskRunning(resolvedTaskID)
		if err != nil {
			return toolresult.NewErrorResult(t.Name(), err.Error())
		}
		if running {
			if !orchestration.DefaultStore.QueuePendingMessage(resolvedTaskID, messageText) {
				return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("failed to queue message for running task %s", resolvedTaskID))
			}
			queuedCount := orchestration.DefaultStore.PendingMessageCount(resolvedTaskID, "")
			return toolresult.ToolResult{
				Success: true,
				Output: fmt.Sprintf("Worker is still running. The message was queued for delivery at the worker's next safe turn boundary.\nTask ID: %s\nQueued messages: %d\n\nYou will be automatically notified when the worker completes. Continue with other work or respond to the user instead.",
					resolvedTaskID, queuedCount),
				Metadata: toolresult.ResultMetadata{
					Title:    t.Name(),
					Icon:     t.Icon(),
					Subtitle: fmt.Sprintf("%s: queued for %s", target.agentType, resolvedTaskID),
					Duration: time.Since(start),
				},
			}
		}
		if err := ensureContinuationTaskReady(resolvedTaskID); err != nil {
			return toolresult.NewErrorResult(t.Name(), err.Error())
		}
	}

	description := tool.GetString(normalized, "description")
	if description == "" {
		description = "Message existing worker"
	}

	agentName := tool.GetString(normalized, "name")
	if agentName == "" {
		agentName = target.name
	}

	var onProgress tool.ProgressFunc
	if cb, ok := normalized["_onProgress"].(tool.ProgressFunc); ok {
		onProgress = cb
	}
	var onQuestion tool.AskQuestionFunc
	if cb, ok := normalized["_onQuestion"].(tool.AskQuestionFunc); ok {
		onQuestion = cb
	}

	req := tool.AgentExecRequest{
		Agent:       target.agentType,
		Name:        agentName,
		Prompt:      composeContinuationPrompt(messageText, orchestration.DefaultStore.DrainPendingMessages(resolvedTaskID, target.agentID)),
		Description: description,
		Background:  tool.GetBool(normalized, "run_in_background"),
		Model:       tool.GetString(normalized, "model"),
		MaxTurns:    tool.GetInt(normalized, "max_turns", 0),
		Mode:        tool.GetString(normalized, "mode"),
		ResumeID:    target.agentID,
		Isolation:   tool.GetString(normalized, "isolation"),
		OnProgress:  onProgress,
		OnQuestion:  onQuestion,
	}

	if req.Background {
		taskInfo, err := t.Executor.RunBackground(req)
		if err != nil {
			return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("failed to send background message to agent: %v", err))
		}

		return toolresult.ToolResult{
			Success: true,
			Output: fmt.Sprintf("Message sent to worker in background.\nTask ID: %s\nAgent: %s\nContinuation of: %s\nDescription: %s"+backgroundLaunchSuffix,
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

	result, err := t.Executor.Run(ctx, req)
	if err != nil {
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("agent message failed: %v", err))
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

func normalizeSendMessageParams(params map[string]any) map[string]any {
	normalized := make(map[string]any, len(params)+1)
	for k, v := range params {
		normalized[k] = v
	}
	if _, ok := normalized["prompt"]; !ok {
		normalized["prompt"] = tool.GetString(params, "message")
	}
	return normalized
}

func resolveContinuationTargetForMessage(params map[string]any) (continuedAgentTarget, error) {
	target, err := resolveContinuationTarget(params)
	if err != nil {
		return continuedAgentTarget{}, err
	}
	return target, nil
}

func ensureContinuationTaskExists(taskID string) error {
	bgTask, found := task.DefaultManager.Get(taskID)
	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}
	info := bgTask.GetStatus()
	if info.Type != task.TaskTypeAgent {
		return fmt.Errorf("task %s is not an agent task", taskID)
	}
	return nil
}

func ensureContinuationTaskReady(taskID string) error {
	bgTask, found := task.DefaultManager.Get(taskID)
	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if bgTask.IsRunning() {
		return fmt.Errorf("task %s is still running; use SendMessage(task_id=%q, message=...) to queue a follow-up or wait for completion", taskID, taskID)
	}
	info := bgTask.GetStatus()
	if info.Type != task.TaskTypeAgent {
		return fmt.Errorf("task %s is not an agent task", taskID)
	}
	if info.AgentSessionID == "" {
		return fmt.Errorf("task %s has no resumable agent_id yet", taskID)
	}
	return nil
}

func isContinuationTaskRunning(taskID string) (bool, error) {
	bgTask, found := task.DefaultManager.Get(taskID)
	if !found {
		return false, fmt.Errorf("task not found: %s", taskID)
	}
	return bgTask.IsRunning(), nil
}

func resolveLiveTaskID(target continuedAgentTarget) string {
	if target.taskID != "" {
		return target.taskID
	}
	taskID, _ := orchestration.DefaultStore.ResolveTaskID(target.agentID)
	return taskID
}

func init() {
	tool.Register(NewSendMessageTool())
}
