package agent

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

const TickInterval = 500 * time.Millisecond
const MaxPerContinuation = 8

type TickMsg struct{}

func StartTicker() tea.Cmd {
	return tea.Tick(TickInterval, func(time.Time) tea.Msg {
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
	return queue.PopBatch(MaxPerContinuation)
}

func MergeNotifications(items []Notification) Notification {
	if len(items) == 0 {
		return Notification{}
	}
	if len(items) == 1 {
		return items[0]
	}

	merged := Notification{
		Notice:             SummarizeNotices(items),
		Context:            MergeContexts(items),
		ContinuationPrompt: WrapNotifications(items),
		Count:              len(items),
	}
	if batch := SharedBatch(items); batch != nil {
		merged.Batch = batch
	}
	return merged
}

func SummarizeNotices(items []Notification) string {
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

func MergeContexts(items []Notification) []string {
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

func WrapNotifications(items []Notification) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0].ContinuationPrompt
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<task-notifications count=\"%d\">\n", len(items))
	if batch := SharedBatch(items); batch != nil {
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

func SharedBatch(items []Notification) *orchestration.Batch {
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
	hint := RenderCoordinatorHintXML(item)
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

func RenderCoordinatorHintXML(item Notification) string {
	decision := CoordinatorHintDecision(item)
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

func CoordinatorHintDecision(item Notification) orchestration.CoordinatorDecision {
	return orchestration.Decide(item.Status, item.Count, BatchToSnapshot(item.Batch))
}

func BatchToSnapshot(b *orchestration.Batch) *orchestration.BatchSnapshot {
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
	if policy := BuildCoordinatorPolicy(item); policy != "" {
		contexts = append(contexts, policy)
	}
	return contexts
}

func BuildCoordinatorPolicy(item Notification) string {
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
