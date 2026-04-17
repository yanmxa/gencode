package agent

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

const tickInterval = 500 * time.Millisecond
const maxPerContinuation = 8

type TickMsg struct{}

func StartTicker() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

func ResetTrackerIfIdle(streamActive bool) {
	if !streamActive && tracker.DefaultStore.AllDone() {
		tracker.DefaultStore.Reset()
	}
}

func PopReadyNotifications(queue *NotificationQueue, idle bool) []Notification {
	if queue == nil || !idle {
		return nil
	}
	return queue.PopBatch(maxPerContinuation)
}

func MergeNotifications(items []Notification) Notification {
	if len(items) == 0 {
		return Notification{}
	}
	if len(items) == 1 {
		return items[0]
	}

	merged := Notification{
		Notice:             summarizeNotices(items),
		Context:            mergeContexts(items),
		ContinuationPrompt: wrapNotifications(items),
		Count:              len(items),
	}
	if batch := sharedBatch(items); batch != nil {
		merged.Batch = batch
	}
	return merged
}

func summarizeNotices(items []Notification) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0].Notice
	}

	parts := make([]string, 0, min(len(items), 3))
	for i, item := range items {
		if i >= 3 {
			break
		}
		if item.Notice != "" {
			parts = append(parts, item.Notice)
		}
	}

	summary := fmt.Sprintf("%d background tasks completed", len(items))
	if len(parts) > 0 {
		summary += ": " + strings.Join(parts, "; ")
		if len(items) > len(parts) {
			summary += "; ..."
		}
	}
	return summary
}

func mergeContexts(items []Notification) []string {
	seen := make(map[string]bool)
	merged := make([]string, 0, len(items)+1)
	merged = append(merged, fmt.Sprintf("Multiple background tasks completed while the main loop was idle. The next message may contain up to %d <task-notification> blocks. Synthesize the important results before deciding on follow-up action.", len(items)))

	for _, item := range items {
		for _, ctx := range item.Context {
			if ctx == "" || seen[ctx] {
				continue
			}
			seen[ctx] = true
			merged = append(merged, ctx)
		}
	}

	return merged
}

func wrapNotifications(items []Notification) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0].ContinuationPrompt
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<task-notifications count=\"%d\">\n", len(items))
	if batch := sharedBatch(items); batch != nil {
		sb.WriteString("<batch-summary>\n")
		sb.WriteString(renderBatchXML(batch, ""))
		sb.WriteString("</batch-summary>\n")
	}
	for _, item := range items {
		prompt := strings.TrimSpace(item.ContinuationPrompt)
		if prompt == "" {
			continue
		}
		sb.WriteString(prompt)
		sb.WriteString("\n")
	}
	sb.WriteString("</task-notifications>")
	return sb.String()
}

func sharedBatch(items []Notification) *orchestration.Batch {
	if len(items) == 0 || items[0].Batch == nil || items[0].Batch.ID == "" {
		return nil
	}
	first := items[0].Batch.ID
	for _, item := range items[1:] {
		if item.Batch == nil || item.Batch.ID != first {
			return nil
		}
	}
	// Use the last item's batch snapshot -- it was captured most recently
	// and has the most up-to-date completion counts.
	return items[len(items)-1].Batch
}

func BuildContinuationPrompt(item Notification) string {
	prompt := strings.TrimSpace(item.ContinuationPrompt)
	hint := renderCoordinatorHintXML(item)
	switch {
	case hint == "" && prompt == "":
		return ""
	case hint == "":
		return prompt
	case prompt == "":
		return hint
	default:
		return hint + "\n" + prompt
	}
}

func renderCoordinatorHintXML(item Notification) string {
	decision := coordinatorHintDecision(item)
	if decision.Phase == "" || decision.RecommendedAction == "" {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<coordinator-hint>\n")
	fmt.Fprintf(&sb, "<phase>%s</phase>\n", escapeXMLText(decision.Phase))
	if item.Count > 1 {
		fmt.Fprintf(&sb, "<notification-count>%d</notification-count>\n", item.Count)
	}
	if item.TaskID != "" {
		fmt.Fprintf(&sb, "<current-task-id>%s</current-task-id>\n", escapeXMLText(item.TaskID))
	}
	if item.Subject != "" {
		fmt.Fprintf(&sb, "<current-subject>%s</current-subject>\n", escapeXMLText(item.Subject))
	}
	if item.Status != "" {
		fmt.Fprintf(&sb, "<current-status>%s</current-status>\n", escapeXMLText(item.Status))
	}
	if item.Batch != nil {
		if item.Batch.ID != "" {
			fmt.Fprintf(&sb, "<batch-id>%s</batch-id>\n", escapeXMLText(item.Batch.ID))
		}
		fmt.Fprintf(&sb, "<batch-completed>%d</batch-completed>\n", item.Batch.Completed)
		fmt.Fprintf(&sb, "<batch-total>%d</batch-total>\n", item.Batch.Total)
		fmt.Fprintf(&sb, "<batch-failures>%d</batch-failures>\n", item.Batch.Failures)
	}
	fmt.Fprintf(&sb, "<recommended-action>%s</recommended-action>\n", escapeXMLText(decision.RecommendedAction))
	fmt.Fprintf(&sb, "<wait-for-remaining-workers>%t</wait-for-remaining-workers>\n", decision.WaitForRemainingWorkers)
	fmt.Fprintf(&sb, "<should-continue-failed-worker>%t</should-continue-failed-worker>\n", decision.ShouldContinueFailedWorker)
	fmt.Fprintf(&sb, "<should-spawn-verifier>%t</should-spawn-verifier>\n", decision.ShouldSpawnVerifier)
	fmt.Fprintf(&sb, "<should-finalize-summary>%t</should-finalize-summary>\n", decision.ShouldFinalizeSummary)
	sb.WriteString("</coordinator-hint>")
	return sb.String()
}

func coordinatorHintDecision(item Notification) orchestration.CoordinatorDecision {
	return orchestration.Decide(item.Status, item.Count, batchToSnapshot(item.Batch))
}

func batchToSnapshot(b *orchestration.Batch) *orchestration.BatchSnapshot {
	if b == nil {
		return nil
	}
	return &orchestration.BatchSnapshot{
		ID:        b.ID,
		Key:       b.Key,
		Subject:   b.Subject,
		Status:    b.Status,
		Completed: b.Completed,
		Total:     b.Total,
		Failures:  b.Failures,
		Workers:   b.Workers,
	}
}

func ContinuationContext(item Notification) []string {
	contexts := append([]string(nil), item.Context...)
	if policy := buildCoordinatorPolicy(item); policy != "" {
		contexts = append(contexts, policy)
	}
	return contexts
}

func buildCoordinatorPolicy(item Notification) string {
	if item.Batch == nil || item.Batch.Total <= 0 {
		return "Treat this as a coordinator signal. Synthesize the result before deciding whether any follow-up worker action or user-facing summary is needed."
	}

	if item.Batch.Completed < item.Batch.Total {
		return fmt.Sprintf(
			"This result belongs to an active background batch (%d/%d complete, %d failures so far). Do not assume the batch is finished. Synthesize what changed, decide whether the remaining workers are still worth waiting for, and only take immediate follow-up action if this result materially changes the plan.",
			item.Batch.Completed,
			item.Batch.Total,
			item.Batch.Failures,
		)
	}

	if item.Batch.Failures > 0 {
		return fmt.Sprintf(
			"This notification arrives after the background batch completed with failures (%d/%d complete, %d failures). Synthesize the finished workers together, highlight the failed ones, and decide whether to continue a failed worker, spawn a verifier, or report a partial result to the user.",
			item.Batch.Completed,
			item.Batch.Total,
			item.Batch.Failures,
		)
	}

	return fmt.Sprintf(
		"This notification arrives after the background batch fully completed (%d/%d complete). Synthesize the worker results together before responding, then decide whether any additional verification or implementation step is needed.",
		item.Batch.Completed,
		item.Batch.Total,
	)
}

// --- Background tracker/batch management ---

const (
	BackgroundTrackerKindKey      = "background_kind"
	BackgroundTrackerKindBatch    = "batch"
	BackgroundTrackerKindWorker   = "worker"
	BackgroundTrackerBatchKey     = "background_batch_key"
	BackgroundTrackerParentID     = "background_parent_id"
	BackgroundTrackerTaskID       = "background_task_id"
	BackgroundTrackerAgentType    = "background_agent_type"
	BackgroundTrackerAgentID      = "background_agent_id"
	BackgroundTrackerStatusDetail = "background_status_detail"
	BackgroundTrackerCompleted    = "background_completed"
	BackgroundTrackerTotal        = "background_total"
	BackgroundTrackerFailures     = "background_failures"
)

type BackgroundTaskLaunch struct {
	TaskID      string
	AgentName   string
	AgentType   string
	Description string
	ResumeID    string
}

func EnsureBackgroundBatchTracker(batchKey string, total int) string {
	if batchKey == "" || total <= 1 {
		return ""
	}
	if existing := findBatchTracker(batchKey); existing != nil {
		_ = tracker.DefaultStore.Update(existing.ID,
			tracker.WithStatus(tracker.StatusInProgress),
			tracker.WithMetadata(map[string]any{
				BackgroundTrackerKindKey:  BackgroundTrackerKindBatch,
				BackgroundTrackerBatchKey: batchKey,
				BackgroundTrackerTotal:    total,
			}),
		)
		orchestration.DefaultStore.UpdateBatch(orchestration.Batch{
			ID:        existing.ID,
			Key:       batchKey,
			Subject:   existing.Subject,
			Status:    tracker.StatusInProgress,
			Completed: metadataInt(existing.Metadata, BackgroundTrackerCompleted),
			Total:     total,
			Failures:  metadataInt(existing.Metadata, BackgroundTrackerFailures),
		})
		return existing.ID
	}

	batch := tracker.DefaultStore.Create(
		fmt.Sprintf("%d background agents launched", total),
		"Coordinator background worker batch",
		"",
		map[string]any{
			BackgroundTrackerKindKey:   BackgroundTrackerKindBatch,
			BackgroundTrackerBatchKey:  batchKey,
			BackgroundTrackerCompleted: 0,
			BackgroundTrackerTotal:     total,
			BackgroundTrackerFailures:  0,
		},
	)
	_ = tracker.DefaultStore.Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))
	orchestration.DefaultStore.UpdateBatch(orchestration.Batch{
		ID:        batch.ID,
		Key:       batchKey,
		Subject:   batch.Subject,
		Status:    tracker.StatusInProgress,
		Completed: 0,
		Total:     total,
		Failures:  0,
	})
	return batch.ID
}

func EnsureBackgroundWorkerTracker(launch BackgroundTaskLaunch, parentID, batchKey string) string {
	if existing := findTrackerByMetadata(BackgroundTrackerTaskID, launch.TaskID); existing != nil {
		_ = tracker.DefaultStore.Update(existing.ID,
			tracker.WithSubject(backgroundWorkerSubject(launch)),
			tracker.WithDescription(launch.Description),
			tracker.WithStatus(tracker.StatusInProgress),
			tracker.WithMetadata(map[string]any{
				BackgroundTrackerKindKey:      BackgroundTrackerKindWorker,
				BackgroundTrackerParentID:     parentID,
				BackgroundTrackerBatchKey:     batchKey,
				BackgroundTrackerTaskID:       launch.TaskID,
				BackgroundTrackerAgentType:    launch.AgentType,
				BackgroundTrackerAgentID:      launch.ResumeID,
				BackgroundTrackerStatusDetail: string(task.StatusRunning),
			}),
		)
		return existing.ID
	}

	trackerEntry := tracker.DefaultStore.Create(
		backgroundWorkerSubject(launch),
		launch.Description,
		"",
		map[string]any{
			BackgroundTrackerKindKey:      BackgroundTrackerKindWorker,
			BackgroundTrackerParentID:     parentID,
			BackgroundTrackerBatchKey:     batchKey,
			BackgroundTrackerTaskID:       launch.TaskID,
			BackgroundTrackerAgentType:    launch.AgentType,
			BackgroundTrackerAgentID:      launch.ResumeID,
			BackgroundTrackerStatusDetail: string(task.StatusRunning),
		},
	)
	opts := []tracker.UpdateOption{
		tracker.WithStatus(tracker.StatusInProgress),
	}
	if launch.AgentType != "" {
		opts = append(opts, tracker.WithOwner(launch.AgentType))
	}
	_ = tracker.DefaultStore.Update(trackerEntry.ID, opts...)
	return trackerEntry.ID
}

func UpdateBackgroundWorkerTracker(info task.TaskInfo) {
	trackerTask := findTrackerByMetadata(BackgroundTrackerTaskID, info.ID)
	if trackerTask == nil {
		orchestration.DefaultStore.RecordCompletion(info.ID, string(info.Status), info.AgentSessionID)
		return
	}

	subject := trackerTask.Subject
	if subject == "" {
		subject = backgroundWorkerSubject(BackgroundTaskLaunch{
			TaskID:      info.ID,
			AgentName:   info.AgentName,
			AgentType:   info.AgentType,
			Description: info.Description,
			ResumeID:    info.AgentSessionID,
		})
	}

	statusDetail := string(info.Status)
	if statusDetail == "" {
		statusDetail = string(task.StatusCompleted)
	}

	metadata := map[string]any{
		BackgroundTrackerKindKey:      BackgroundTrackerKindWorker,
		BackgroundTrackerTaskID:       info.ID,
		BackgroundTrackerAgentType:    info.AgentType,
		BackgroundTrackerAgentID:      info.AgentSessionID,
		BackgroundTrackerStatusDetail: statusDetail,
	}

	opts := []tracker.UpdateOption{
		tracker.WithSubject(subject),
		tracker.WithDescription(info.Description),
		tracker.WithStatus(tracker.StatusCompleted),
		tracker.WithMetadata(metadata),
	}
	_ = tracker.DefaultStore.Update(trackerTask.ID, opts...)
	orchestration.DefaultStore.RecordLaunch(orchestration.Launch{
		TaskID:       info.ID,
		AgentID:      firstNonEmpty(info.AgentSessionID, MetadataString(trackerTask.Metadata, BackgroundTrackerAgentID)),
		AgentType:    firstNonEmpty(info.AgentType, MetadataString(trackerTask.Metadata, BackgroundTrackerAgentType)),
		AgentName:    info.AgentName,
		Description:  firstNonEmpty(info.Description, trackerTask.Description),
		Status:       statusDetail,
		Running:      info.Status == task.StatusRunning,
		BatchID:      MetadataString(trackerTask.Metadata, BackgroundTrackerParentID),
		BatchKey:     MetadataString(trackerTask.Metadata, BackgroundTrackerBatchKey),
		BatchSubject: backgroundBatchSubject(MetadataString(trackerTask.Metadata, BackgroundTrackerParentID)),
	})
	orchestration.DefaultStore.RecordCompletion(info.ID, statusDetail, firstNonEmpty(info.AgentSessionID, MetadataString(trackerTask.Metadata, BackgroundTrackerAgentID)))

	if parentID := MetadataString(trackerTask.Metadata, BackgroundTrackerParentID); parentID != "" {
		reconcileBackgroundBatch(parentID)
	}
}

func reconcileBackgroundBatch(parentID string) {
	parent, ok := tracker.DefaultStore.Get(parentID)
	if !ok {
		return
	}

	children := childTrackers(parentID)
	total := len(children)
	if total == 0 {
		return
	}

	completed := 0
	failures := 0
	for _, child := range children {
		if child.Status == tracker.StatusCompleted {
			completed++
			switch MetadataString(child.Metadata, BackgroundTrackerStatusDetail) {
			case string(task.StatusFailed), string(task.StatusKilled):
				failures++
			}
		}
	}

	status := tracker.StatusInProgress
	if completed == total {
		status = tracker.StatusCompleted
	}

	_ = tracker.DefaultStore.Update(parent.ID,
		tracker.WithStatus(status),
		tracker.WithMetadata(map[string]any{
			BackgroundTrackerKindKey:   BackgroundTrackerKindBatch,
			BackgroundTrackerCompleted: completed,
			BackgroundTrackerTotal:     total,
			BackgroundTrackerFailures:  failures,
		}),
	)

	workers := make([]orchestration.BatchWorker, 0, len(children))
	for _, child := range children {
		workers = append(workers, orchestration.BatchWorker{
			TaskID:    MetadataString(child.Metadata, BackgroundTrackerTaskID),
			Subject:   child.Subject,
			Status:    MetadataString(child.Metadata, BackgroundTrackerStatusDetail),
			AgentType: MetadataString(child.Metadata, BackgroundTrackerAgentType),
		})
	}
	orchestration.DefaultStore.UpdateBatch(orchestration.Batch{
		ID:        parent.ID,
		Key:       MetadataString(parent.Metadata, BackgroundTrackerBatchKey),
		Subject:   parent.Subject,
		Status:    status,
		Completed: completed,
		Total:     total,
		Failures:  failures,
		Workers:   workers,
	})
}

func backgroundWorkerSubject(launch BackgroundTaskLaunch) string {
	if s := JoinNameDesc(launch.AgentName, launch.Description); s != "" {
		return s
	}
	if launch.AgentType != "" {
		return launch.AgentType
	}
	return launch.TaskID
}

func findTrackerByMetadata(key, want string) *tracker.Task {
	return tracker.DefaultStore.FindByMetadata(key, want)
}

func findBatchTracker(batchKey string) *tracker.Task {
	for _, t := range tracker.DefaultStore.List() {
		if MetadataString(t.Metadata, BackgroundTrackerKindKey) == BackgroundTrackerKindBatch &&
			MetadataString(t.Metadata, BackgroundTrackerBatchKey) == batchKey {
			return t
		}
	}
	return nil
}

func childTrackers(parentID string) []*tracker.Task {
	var children []*tracker.Task
	for _, t := range tracker.DefaultStore.List() {
		if MetadataString(t.Metadata, BackgroundTrackerParentID) == parentID {
			children = append(children, t)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].CreatedAt.Before(children[j].CreatedAt)
	})
	return children
}

func SnapshotBackgroundBatchForTask(taskID string) *orchestration.Batch {
	if batch, ok := orchestration.DefaultStore.SnapshotBatchForTask(taskID); ok {
		return batch
	}

	child := findTrackerByMetadata(BackgroundTrackerTaskID, taskID)
	if child == nil {
		return nil
	}
	parentID := MetadataString(child.Metadata, BackgroundTrackerParentID)
	if parentID == "" {
		return nil
	}

	parent, ok := tracker.DefaultStore.Get(parentID)
	if !ok {
		return nil
	}

	children := childTrackers(parentID)
	workers := make([]orchestration.BatchWorker, 0, len(children))
	for _, c := range children {
		status := MetadataString(c.Metadata, BackgroundTrackerStatusDetail)
		if status == "" {
			status = c.Status
		}
		workers = append(workers, orchestration.BatchWorker{
			TaskID:    MetadataString(c.Metadata, BackgroundTrackerTaskID),
			Subject:   c.Subject,
			Status:    status,
			AgentType: MetadataString(c.Metadata, BackgroundTrackerAgentType),
		})
	}

	status := parent.Status
	if status == "" {
		status = tracker.StatusPending
	}

	return &orchestration.Batch{
		ID:        parent.ID,
		Subject:   parent.Subject,
		Status:    status,
		Completed: metadataInt(parent.Metadata, BackgroundTrackerCompleted),
		Total:     metadataInt(parent.Metadata, BackgroundTrackerTotal),
		Failures:  metadataInt(parent.Metadata, BackgroundTrackerFailures),
		Workers:   workers,
	}
}

func RecordBackgroundTaskLaunch(launch BackgroundTaskLaunch, parentID, batchKey string, batchTotal int) {
	orchestration.DefaultStore.RecordLaunch(orchestration.Launch{
		TaskID:       launch.TaskID,
		AgentID:      launch.ResumeID,
		AgentType:    launch.AgentType,
		AgentName:    launch.AgentName,
		Description:  launch.Description,
		Status:       string(task.StatusRunning),
		Running:      true,
		BatchID:      parentID,
		BatchKey:     batchKey,
		BatchSubject: backgroundBatchSubject(parentID),
		BatchTotal:   batchTotal,
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func backgroundBatchSubject(parentID string) string {
	if parentID == "" {
		return ""
	}
	parent, ok := tracker.DefaultStore.Get(parentID)
	if !ok {
		return ""
	}
	return parent.Subject
}

func MetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func metadataInt(metadata map[string]any, key string) int {
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

func JoinNameDesc(name, desc string) string {
	name = strings.TrimSpace(name)
	desc = strings.TrimSpace(desc)
	switch {
	case name != "" && desc != "" && !strings.EqualFold(name, desc):
		return name + ": " + desc
	case desc != "":
		return desc
	case name != "":
		return name
	default:
		return ""
	}
}

// --- Unexported XML helpers (used by notification.go and within this file) ---

func fallbackNotice(subject, status string) string {
	if subject == "" {
		subject = "Background task"
	}
	return fmt.Sprintf("%s %s", subject, status)
}

func backgroundNotificationContext() []string {
	return []string{
		"A background task completed while the main loop was idle. Treat the next <task-notification> user message as an internal completion signal from a background worker or command. If the notification includes <output-file>, that path is the normal place to inspect detailed output when the user explicitly asks. Summarize the important result for the user and decide whether follow-up action is needed.",
	}
}

func buildNotificationSummary(taskType task.TaskType, subject, status string) string {
	switch taskType {
	case task.TaskTypeAgent:
		if subject == "" {
			subject = "background agent"
		}
		return fmt.Sprintf("Agent %q %s", subject, status)
	case task.TaskTypeBash:
		if subject == "" {
			subject = "background command"
		}
		return fmt.Sprintf("Command %q %s", subject, status)
	default:
		if subject == "" {
			subject = "background task"
		}
		return fmt.Sprintf("Task %q %s", subject, status)
	}
}

func formatStatus(status task.TaskStatus) string {
	switch status {
	case task.StatusCompleted:
		return "completed"
	case task.StatusFailed:
		return "failed"
	case task.StatusKilled:
		return "killed"
	default:
		return ""
	}
}

func escapeXMLText(s string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(s)); err != nil {
		return s
	}
	return b.String()
}

func renderBatchXML(batch *orchestration.Batch, currentTaskID string) string {
	if batch == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("<batch>\n")
	if batch.ID != "" {
		fmt.Fprintf(&b, "<id>%s</id>\n", escapeXMLText(batch.ID))
	}
	if batch.Subject != "" {
		fmt.Fprintf(&b, "<subject>%s</subject>\n", escapeXMLText(batch.Subject))
	}
	if batch.Status != "" {
		fmt.Fprintf(&b, "<status>%s</status>\n", escapeXMLText(batch.Status))
	}
	if batch.Total > 0 {
		fmt.Fprintf(&b, "<completed>%d</completed>\n", batch.Completed)
		fmt.Fprintf(&b, "<total>%d</total>\n", batch.Total)
		fmt.Fprintf(&b, "<failures>%d</failures>\n", batch.Failures)
		fmt.Fprintf(&b, "<summary>%s</summary>\n", escapeXMLText(describeBatch(batch)))
	}
	if len(batch.Workers) > 0 {
		b.WriteString("<tasks>\n")
		for _, sibling := range batch.Workers {
			b.WriteString("  <task")
			if sibling.TaskID != "" {
				fmt.Fprintf(&b, " task-id=\"%s\"", escapeXMLText(sibling.TaskID))
			}
			if sibling.Status != "" {
				fmt.Fprintf(&b, " status=\"%s\"", escapeXMLText(sibling.Status))
			}
			if sibling.AgentType != "" {
				fmt.Fprintf(&b, " agent-type=\"%s\"", escapeXMLText(sibling.AgentType))
			}
			if sibling.TaskID == currentTaskID {
				b.WriteString(` current="true"`)
			}
			b.WriteString(">")
			b.WriteString(escapeXMLText(sibling.Subject))
			b.WriteString("</task>\n")
		}
		b.WriteString("</tasks>\n")
	}
	b.WriteString("</batch>\n")
	return b.String()
}

func describeBatch(batch *orchestration.Batch) string {
	if batch == nil {
		return ""
	}
	subject := batch.Subject
	if subject == "" {
		subject = "background batch"
	}
	if batch.Total <= 0 {
		return subject
	}
	if batch.Failures > 0 {
		return fmt.Sprintf("%s is %d/%d complete with %d failures", subject, batch.Completed, batch.Total, batch.Failures)
	}
	return fmt.Sprintf("%s is %d/%d complete", subject, batch.Completed, batch.Total)
}
