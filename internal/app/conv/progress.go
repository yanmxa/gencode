package conv

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
)

type progressUpdate struct {
	Index   int
	Message string
}

// ProgressUpdateMsg carries a task progress update from an agent.
type ProgressUpdateMsg struct {
	Index   int
	Message string
}

type progressQuestionUpdate struct {
	Index   int
	Request *tool.QuestionRequest
	Reply   chan *tool.QuestionResponse
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
	ch  chan progressUpdate
	qch chan progressQuestionUpdate
}

// NewProgressHub creates a new progress hub with the given buffer size.
func NewProgressHub(buffer int) *ProgressHub {
	if buffer <= 0 {
		buffer = 100
	}
	return &ProgressHub{
		ch:  make(chan progressUpdate, buffer),
		qch: make(chan progressQuestionUpdate, buffer),
	}
}

// SendForAgent enqueues a progress message for a specific agent index.
func (h *ProgressHub) SendForAgent(index int, msg string) {
	select {
	case h.ch <- progressUpdate{Index: index, Message: msg}:
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
	case h.qch <- progressQuestionUpdate{Index: index, Request: req, Reply: reply}:
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
			return ProgressQuestionMsg{Index: q.Index, Request: q.Request, Reply: q.Reply}
		case u := <-h.ch:
			return ProgressUpdateMsg(u)
		default:
			return ProgressCheckTickMsg{}
		}
	})
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

// maxVisibleTasks is the maximum number of tasks shown before collapsing.
const maxVisibleTasks = 8

// TrackerListParams holds the parameters for rendering a tracker list.
type TrackerListParams struct {
	Tasks        []*tracker.Task
	AllDone      bool
	StreamActive bool
	Width        int
	SpinnerView  string
	Blockers     func(taskID string) []string
	WorkerSnap   func(taskID, agentID string) (*orchestration.Snapshot, bool)
}

// RenderTrackerList renders a compact task list above the input area.
// Shows all tasks including completed ones. Returns empty string when
// there are no tasks or when all tasks are completed and the LLM is idle.
//
// Note: this function is pure — it does not mutate tracker state.
// The caller is responsible for resetting the store when appropriate
// (see AllDone field in TrackerListParams).
func RenderTrackerList(params TrackerListParams) string {
	if len(params.Tasks) == 0 {
		return ""
	}

	if params.AllDone && !params.StreamActive {
		return ""
	}

	completed := 0
	for _, t := range params.Tasks {
		if t.Status == tracker.StatusCompleted {
			completed++
		}
	}
	total := len(params.Tasks)

	var sb strings.Builder

	headerStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)

	sb.WriteString("  " + headerStyle.Render("Tasks") + " " + mutedStyle.Render(fmt.Sprintf("(%d/%d)", completed, total)) + "\n")

	sb.WriteString(renderTasksHierarchical(params.Tasks, params.Width, params.SpinnerView, params.Blockers, params.WorkerSnap))

	return sb.String()
}

// renderTasksHierarchical renders tracker tasks, grouping background worker batches.
func renderTasksHierarchical(tasks []*tracker.Task, width int, spinnerView string, blockers func(string) []string, workerSnap func(string, string) (*orchestration.Snapshot, bool)) string {
	childrenByParent := make(map[string][]*tracker.Task)
	childIDs := make(map[string]bool)
	for _, t := range tasks {
		if parentID := trackerMetadataString(t.Metadata, "background_parent_id"); parentID != "" {
			childrenByParent[parentID] = append(childrenByParent[parentID], t)
			childIDs[t.ID] = true
		}
	}

	var sb strings.Builder
	renderedRoots := 0
	for _, t := range tasks {
		if childIDs[t.ID] {
			continue
		}
		if renderedRoots >= maxVisibleTasks && t.Status != tracker.StatusInProgress {
			continue
		}
		if isBackgroundBatchTask(t) {
			sb.WriteString(renderBackgroundBatchTask(t, childrenByParent[t.ID], width, spinnerView, workerSnap))
		} else {
			sb.WriteString(renderTrackerTask(t, width, spinnerView, blockers, workerSnap))
		}
		renderedRoots++
	}
	return sb.String()
}

// renderTrackerTask renders a single task line.
func renderTrackerTask(t *tracker.Task, width int, spinnerView string, blockers func(string) []string, workerSnap func(string, string) (*orchestration.Snapshot, bool)) string {
	return renderTrackerTaskIndented(t, width, spinnerView, "", blockers, workerSnap)
}

// renderTrackerTaskIndented renders a single task line with optional extra indentation.
func renderTrackerTaskIndented(t *tracker.Task, width int, spinnerView string, extraIndent string, blockers func(string) []string, workerSnap func(string, string) (*orchestration.Snapshot, bool)) string {
	indent := extraIndent + "  "
	idTag := fmt.Sprintf("#%s ", t.ID)
	maxTextLen := width - len(indent) - len(idTag) - 6 // icon + spaces + margin
	subject := kit.TruncateText(t.Subject, maxTextLen)
	worker := lookupWorkerSnapshot(t, workerSnap)

	mutedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	idStr := mutedStyle.Render(idTag)
	statusDetail := trackerMetadataString(t.Metadata, "background_status_detail")
	if worker != nil && worker.Worker.Status != "" {
		statusDetail = worker.Worker.Status
	}
	queueSuffix := ""
	if worker != nil && worker.Worker.PendingMessageCount > 0 {
		queueSuffix = " " + mutedStyle.Render(fmt.Sprintf("[%d queued]", worker.Worker.PendingMessageCount))
	}

	switch t.Status {
	case tracker.StatusCompleted:
		if statusDetail == "failed" || statusDetail == "killed" {
			failedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Error)
			return indent + failedStyle.Render("!") + " " + idStr + failedStyle.Render(subject) + queueSuffix + " " + mutedStyle.Render("["+statusDetail+"]") + "\n"
		}
		return indent + trackerCompletedStyle.Render("✓") + " " + idStr + trackerCompletedStyle.Render(subject) + queueSuffix + "\n"

	case tracker.StatusInProgress:
		displayText := subject
		if t.ActiveForm != "" {
			displayText = kit.TruncateText(t.ActiveForm, maxTextLen)
		}
		line := indent + trackerInProgressStyle.Render(spinnerView) + " " + idStr + trackerInProgressStyle.Render(displayText) + queueSuffix
		if elapsed := formatElapsedTime(t.StatusChangedAt); elapsed != "" {
			line += " " + mutedStyle.Render(elapsed)
		}
		return line + "\n"

	default:
		if blockers != nil {
			if bl := blockers(t.ID); len(bl) > 0 {
				blockerRefs := make([]string, len(bl))
				for i, b := range bl {
					blockerRefs[i] = "#" + b
				}
				blockedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Error)
				suffix := " " + blockedStyle.Render("← "+strings.Join(blockerRefs, ", "))
				return indent + trackerPendingStyle.Render("○") + " " + idStr + trackerPendingStyle.Render(subject) + suffix + "\n"
			}
		}
		return indent + trackerPendingStyle.Render("○") + " " + idStr + trackerPendingStyle.Render(subject) + "\n"
	}
}

func renderBackgroundBatchTask(batch *tracker.Task, children []*tracker.Task, width int, spinnerView string, workerSnap func(string, string) (*orchestration.Snapshot, bool)) string {
	var sb strings.Builder
	sb.WriteString(renderBatchHeader(batch, children, width, spinnerView, workerSnap))
	for _, child := range children {
		sb.WriteString(renderTrackerTaskIndented(child, width, spinnerView, "  ", nil, workerSnap))
	}
	return sb.String()
}

func renderBatchHeader(t *tracker.Task, children []*tracker.Task, width int, spinnerView string, workerSnap func(string, string) (*orchestration.Snapshot, bool)) string {
	indent := "  "
	idTag := fmt.Sprintf("#%s ", t.ID)
	mutedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	idStr := mutedStyle.Render(idTag)
	subject := t.Subject
	total := trackerMetadataInt(t.Metadata, "background_total")
	completed := trackerMetadataInt(t.Metadata, "background_completed")
	failures := trackerMetadataInt(t.Metadata, "background_failures")
	if snapshot := lookupBatchSnapshot(children, workerSnap); snapshot != nil {
		if snapshot.Subject != "" {
			subject = snapshot.Subject
		}
		total = snapshot.Total
		completed = snapshot.Completed
		failures = snapshot.Failures
	}

	countSuffix := ""
	if total > 0 {
		countSuffix = mutedStyle.Render(fmt.Sprintf(" (%d/%d)", completed, total))
		if failures > 0 {
			countSuffix += " " + lipgloss.NewStyle().Foreground(kit.CurrentTheme.Error).Render(fmt.Sprintf("%d failed", failures))
		}
	}

	maxTextLen := width - len(indent) - len(idTag) - 10
	subject = kit.TruncateText(subject, maxTextLen)

	switch t.Status {
	case tracker.StatusCompleted:
		return indent + trackerCompletedStyle.Render("✓") + " " + idStr + trackerCompletedStyle.Render(subject) + countSuffix + "\n"
	case tracker.StatusInProgress:
		line := indent + trackerInProgressStyle.Render(spinnerView) + " " + idStr + trackerInProgressStyle.Render(subject) + countSuffix
		if elapsed := formatElapsedTime(t.StatusChangedAt); elapsed != "" {
			line += " " + mutedStyle.Render(elapsed)
		}
		return line + "\n"
	default:
		return indent + trackerPendingStyle.Render("○") + " " + idStr + trackerPendingStyle.Render(subject) + countSuffix + "\n"
	}
}

func isBackgroundBatchTask(t *tracker.Task) bool {
	return trackerMetadataString(t.Metadata, "background_kind") == "batch"
}

func lookupWorkerSnapshot(t *tracker.Task, workerSnap func(string, string) (*orchestration.Snapshot, bool)) *orchestration.Snapshot {
	if workerSnap == nil {
		return nil
	}
	taskID := trackerMetadataString(t.Metadata, "background_task_id")
	agentID := trackerMetadataString(t.Metadata, "background_agent_id")
	if taskID == "" && agentID == "" {
		return nil
	}
	snapshot, ok := workerSnap(taskID, agentID)
	if !ok {
		return nil
	}
	return snapshot
}

func lookupBatchSnapshot(children []*tracker.Task, workerSnap func(string, string) (*orchestration.Snapshot, bool)) *orchestration.BatchSnapshot {
	for _, child := range children {
		if snapshot := lookupWorkerSnapshot(child, workerSnap); snapshot != nil && snapshot.Batch != nil {
			return snapshot.Batch
		}
	}
	return nil
}

func trackerMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	if value, ok := metadata[key]; ok && value != nil {
		return fmt.Sprint(value)
	}
	return ""
}

func trackerMetadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// formatElapsedTime returns a human-readable elapsed time string since the given time.
// Returns empty string if the time is zero.
func formatElapsedTime(since time.Time) string {
	if since.IsZero() {
		return ""
	}
	d := time.Since(since)
	if d < time.Second {
		return ""
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
