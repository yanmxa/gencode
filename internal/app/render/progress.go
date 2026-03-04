// Task progress rendering: agent progress bars and spinner displays.
package render

import (
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
)

// RenderTaskProgressInline renders live progress for a parallel Task tool call.
// Shows spinner+progress while running, or a done marker when completed.
func RenderTaskProgressInline(tc message.ToolCall, pendingCalls []message.ToolCall, parallelResults map[int]bool, taskProgress map[int][]string, spinnerView string) string {
	idx := -1
	for i, pending := range pendingCalls {
		if pending.ID == tc.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ""
	}

	var sb strings.Builder

	// Check if completed in parallel results (not yet committed to messages)
	if parallelResults != nil {
		if _, done := parallelResults[idx]; done {
			sb.WriteString(ToolResultStyle.Render("  ✓ Done") + "\n")
			return sb.String()
		}
	}

	// Show spinner and progress lines
	progress := taskProgress[idx]
	status := "starting..."
	if len(progress) > 0 {
		status = "running..."
	}
	sb.WriteString(ThinkingStyle.Render(fmt.Sprintf("  %s %s", spinnerView, status)) + "\n")
	for _, p := range progress {
		sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("     %s", p)) + "\n")
	}
	return sb.String()
}

// PendingToolSpinnerParams holds the parameters for rendering a pending tool spinner.
type PendingToolSpinnerParams struct {
	// InteractivePromptActive indicates if an interactive prompt is currently active.
	InteractivePromptActive bool
	// ParallelMode indicates parallel tool execution.
	ParallelMode bool
	// HasParallelTaskTools indicates if any parallel tools are Task tools.
	HasParallelTaskTools bool
	// BuildingTool is the tool name being built during streaming.
	BuildingTool string
	// PendingCalls are the pending tool calls.
	PendingCalls []message.ToolCall
	// CurrentIdx is the index of the current sequential tool.
	CurrentIdx int
	// TaskProgress tracks agent progress messages by index.
	TaskProgress map[int][]string
	// SpinnerView is the current spinner frame.
	SpinnerView string
}

// RenderPendingToolSpinner renders the spinner for a tool being executed.
func RenderPendingToolSpinner(params PendingToolSpinnerParams) string {
	if params.InteractivePromptActive {
		return ""
	}

	// Parallel mode with Task tools: progress rendered inline by RenderToolCalls
	if params.ParallelMode && params.HasParallelTaskTools {
		return ""
	}

	// Determine which tool is active
	var toolName string
	if params.BuildingTool != "" {
		toolName = params.BuildingTool
	} else if params.PendingCalls != nil && params.CurrentIdx < len(params.PendingCalls) {
		toolName = params.PendingCalls[params.CurrentIdx].Name
	} else {
		return ""
	}

	var sb strings.Builder

	// Task tool has special rendering with per-agent progress
	if toolName == "Task" {
		progress := params.TaskProgress[params.CurrentIdx]
		status := "Agent starting..."
		if len(progress) > 0 {
			status = "Agent running..."
		}
		sb.WriteString(ThinkingStyle.Render(fmt.Sprintf("  %s %s", params.SpinnerView, status)) + "\n")
		for _, p := range progress {
			sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("     %s", p)) + "\n")
		}
		return sb.String()
	}

	// Standard tool spinner
	sb.WriteString(ThinkingStyle.Render(fmt.Sprintf("  %s %s", params.SpinnerView, GetToolExecutionDesc(toolName))) + "\n")
	return sb.String()
}
