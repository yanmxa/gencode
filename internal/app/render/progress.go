// Task progress rendering: agent progress bars and spinner displays.
package render

import (
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
)

// renderAgentProgress renders all agent progress lines accumulated so far.
func renderAgentProgress(progress []string) string {
	if len(progress) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, p := range progress {
		sb.WriteString(ToolResultStyle.Render(fmt.Sprintf("  ⎿  %s", p)) + "\n")
	}
	return sb.String()
}

// RenderTaskProgressInline renders live progress for a parallel Agent tool call.
// Spinner is on the header line; this only renders progress lines below it.
func RenderTaskProgressInline(tc message.ToolCall, pendingCalls []message.ToolCall, parallelResults map[int]bool, taskProgress map[int][]string) string {
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

	// Check if completed in parallel results (not yet committed to messages)
	if parallelResults != nil {
		if _, done := parallelResults[idx]; done {
			return ""
		}
	}

	return renderAgentProgress(taskProgress[idx])
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

	// Agent tool: spinner is on the header line; only render progress lines here
	if toolName == "Agent" {
		return renderAgentProgress(params.TaskProgress[params.CurrentIdx])
	}

	// Standard tools: spinner is shown inline in the assistant message row,
	// no separate spinner line needed.
	return ""
}
