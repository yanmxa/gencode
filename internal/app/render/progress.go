// Task progress rendering: agent progress bars and spinner displays.
package render

import (
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
)

// maxAgentProgressLines is the maximum number of progress lines to display.
// Older lines scroll off the top, keeping the view compact.
const maxAgentProgressLines = 8

// renderAgentProgress renders the most recent agent progress lines,
// capped at maxAgentProgressLines to keep the view height bounded.
func renderAgentProgress(progress []string) string {
	if len(progress) == 0 {
		return ""
	}

	// Only show the most recent lines
	visible := progress
	if len(visible) > maxAgentProgressLines {
		visible = visible[len(visible)-maxAgentProgressLines:]
	}

	var sb strings.Builder
	for _, p := range visible {
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
	// Width is the terminal width for label truncation.
	Width int
	// SuppressAgentLabel avoids duplicating the active agent title when the
	// assistant message already rendered it above the progress lines.
	SuppressAgentLabel bool
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

	// Agent tool: render agent label + progress lines
	if tool.IsAgentToolName(toolName) {
		var sb strings.Builder
		// Show Agent label so it remains visible after the assistant message scrolls off.
		if !params.SuppressAgentLabel && params.PendingCalls != nil && params.CurrentIdx < len(params.PendingCalls) {
			tc := params.PendingCalls[params.CurrentIdx]
			label := FormatAgentLabel(tc.Input)
			sb.WriteString(renderToolLineWithIcon(label, params.Width, params.SpinnerView) + "\n")
		}
		sb.WriteString(renderAgentProgress(params.TaskProgress[params.CurrentIdx]))
		return sb.String()
	}

	// Standard tools: spinner is shown inline in the assistant message row,
	// no separate spinner line needed.
	return ""
}
