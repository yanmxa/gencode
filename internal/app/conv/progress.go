package conv

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool"
)

// ProgressUpdateMsg carries a task progress update from an agent.
type ProgressUpdateMsg struct {
	Index   int
	Message string
}

// ProgressQuestionMsg carries an agent question request to the TUI.
type ProgressQuestionMsg struct {
	Index   int
	Request *tool.QuestionRequest
	Reply   chan *tool.QuestionResponse
}

// ProgressCheckTickMsg triggers a check for new progress updates.
type ProgressCheckTickMsg struct{}

// ProgressHub is an instance-scoped progress transport.
type ProgressHub struct {
	ch  chan ProgressUpdateMsg
	qch chan ProgressQuestionMsg
}

// NewProgressHub creates a new progress hub with the given buffer size.
func NewProgressHub(buffer int) *ProgressHub {
	if buffer <= 0 {
		buffer = 100
	}
	return &ProgressHub{
		ch:  make(chan ProgressUpdateMsg, buffer),
		qch: make(chan ProgressQuestionMsg, buffer),
	}
}

// SendForAgent enqueues a progress message for a specific agent index.
func (h *ProgressHub) SendForAgent(index int, msg string) {
	select {
	case h.ch <- ProgressUpdateMsg{Index: index, Message: msg}:
	default:
	}
}

// Ask enqueues an interactive question and waits for the user's response.
func (h *ProgressHub) Ask(ctx context.Context, index int, req *tool.QuestionRequest) (*tool.QuestionResponse, error) {
	if h == nil {
		return nil, fmt.Errorf("progress hub not initialized")
	}

	reply := make(chan *tool.QuestionResponse, 1)
	select {
	case h.qch <- ProgressQuestionMsg{Index: index, Request: req, Reply: reply}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case resp := <-reply:
		if resp == nil {
			return nil, fmt.Errorf("question prompt closed without a response")
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Check returns a tea.Cmd that polls this hub for the next update.
func (h *ProgressHub) Check() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		select {
		case q := <-h.qch:
			return q
		case u := <-h.ch:
			return u
		default:
			return ProgressCheckTickMsg{}
		}
	})
}

// DrainPendingQuestions cancels any pending questions left in the channel.
// Called when the agent stops to prevent orphaned questions from appearing later.
func (h *ProgressHub) DrainPendingQuestions() {
	if h == nil {
		return
	}
	for {
		select {
		case q := <-h.qch:
			select {
			case q.Reply <- &tool.QuestionResponse{Cancelled: true}:
			default:
			}
		default:
			return
		}
	}
}

// Drain pulls all pending updates into taskProgress.
func (h *ProgressHub) Drain(taskProgress map[int][]string) map[int][]string {
	for {
		select {
		case u := <-h.ch:
			if taskProgress == nil {
				taskProgress = make(map[int][]string)
			}
			taskProgress[u.Index] = append(taskProgress[u.Index], u.Message)
			if len(taskProgress[u.Index]) > 5 {
				taskProgress[u.Index] = taskProgress[u.Index][1:]
			}
		default:
			return taskProgress
		}
	}
}

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
		sb.WriteString(toolResultStyle.Render(fmt.Sprintf("  ⎿  %s", p)) + "\n")
	}
	return sb.String()
}

// renderTaskProgressInline renders live progress for a parallel Agent tool call.
// Spinner is on the header line; this only renders progress lines below it.
func renderTaskProgressInline(tc core.ToolCall, pendingCalls []core.ToolCall, parallelResults map[int]bool, taskProgress map[int][]string) string {
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
	PendingCalls []core.ToolCall
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
			label := formatAgentLabel(tc.Input)
			sb.WriteString(renderToolLineWithIcon(label, params.Width, params.SpinnerView) + "\n")
		}
		sb.WriteString(renderAgentProgress(params.TaskProgress[params.CurrentIdx]))
		return sb.String()
	}

	// Standard tools: spinner is shown inline in the assistant message row,
	// no separate spinner line needed.
	return ""
}
