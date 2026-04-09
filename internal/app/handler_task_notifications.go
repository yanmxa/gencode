package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/orchestration"
)

const taskNotificationTickInterval = 500 * time.Millisecond
const maxTaskNotificationsPerContinuation = 8

type taskNotificationTickMsg struct{}

func startTaskNotificationTicker() tea.Cmd {
	return tea.Tick(taskNotificationTickInterval, func(time.Time) tea.Msg {
		return taskNotificationTickMsg{}
	})
}

func (m *model) updateTaskNotifications(msg tea.Msg) (tea.Cmd, bool) {
	if _, ok := msg.(taskNotificationTickMsg); !ok {
		return nil, false
	}
	return m.handleTaskNotificationTick(), true
}

func (m *model) handleTaskNotificationTick() tea.Cmd {
	cmds := []tea.Cmd{startTaskNotificationTicker()}
	if m.taskNotifications == nil {
		return tea.Batch(cmds...)
	}
	if m.conv.Stream.Active || m.hasPendingToolExecution() {
		return tea.Batch(cmds...)
	}

	items := m.taskNotifications.PopBatch(maxTaskNotificationsPerContinuation)
	if len(items) == 0 {
		return tea.Batch(cmds...)
	}

	cmds = append(cmds, m.injectTaskNotificationContinuation(mergeTaskNotifications(items)))
	return tea.Batch(cmds...)
}

func (m *model) injectTaskNotificationContinuation(item taskNotification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleNotice,
			Content: item.Notice,
		})
	}
	if m.provider.LLM == nil {
		if item.Notice == "" {
			m.conv.Append(message.ChatMessage{
				Role:    message.RoleNotice,
				Content: "A background task completed, but no provider is connected.",
			})
		}
		return tea.Batch(m.commitMessages()...)
	}
	if item.ContinuationPrompt == "" {
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleNotice,
			Content: "A background task completed, but no task notification payload was available.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	return m.startConversationStream(m.buildInternalContinuationRequest(taskNotificationContinuationContext(item), buildTaskNotificationContinuationPrompt(item)))
}

func backgroundTaskNotificationContext() []string {
	return []string{
		"A background task completed while the main loop was idle. Treat the next <task-notification> user message as an internal completion signal from a background worker or command. If the notification includes <output-file>, that path is the normal place to inspect detailed output when the user explicitly asks. Summarize the important result for the user and decide whether follow-up action is needed.",
	}
}

func taskNotificationContinuationContext(item taskNotification) []string {
	contexts := append([]string(nil), item.Context...)
	if policy := buildTaskNotificationCoordinatorPolicy(item); policy != "" {
		contexts = append(contexts, policy)
	}
	return contexts
}

func buildTaskNotificationCoordinatorPolicy(item taskNotification) string {
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

func mergeTaskNotifications(items []taskNotification) taskNotification {
	if len(items) == 0 {
		return taskNotification{}
	}
	if len(items) == 1 {
		return items[0]
	}

	merged := taskNotification{
		Notice:             summarizeTaskNotificationNotices(items),
		Context:            mergeTaskNotificationContexts(items),
		ContinuationPrompt: wrapTaskNotifications(items),
		Count:              len(items),
	}
	if batch := sharedTaskNotificationBatch(items); batch != nil {
		merged.Batch = batch
	}
	return merged
}

func summarizeTaskNotificationNotices(items []taskNotification) string {
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

func mergeTaskNotificationContexts(items []taskNotification) []string {
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

func wrapTaskNotifications(items []taskNotification) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return items[0].ContinuationPrompt
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "<task-notifications count=\"%d\">\n", len(items))
	if batch := sharedTaskNotificationBatch(items); batch != nil {
		sb.WriteString("<batch-summary>\n")
		sb.WriteString(renderTaskNotificationBatchXML(batch, ""))
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

func sharedTaskNotificationBatch(items []taskNotification) *backgroundBatchSnapshot {
	if len(items) == 0 || items[0].Batch == nil || items[0].Batch.BatchID == "" {
		return nil
	}
	first := items[0].Batch.BatchID
	for _, item := range items[1:] {
		if item.Batch == nil || item.Batch.BatchID != first {
			return nil
		}
	}
	return items[0].Batch
}

func fallbackTaskCompletionNotice(subject, status string) string {
	if subject == "" {
		subject = "Background task"
	}
	return fmt.Sprintf("%s %s", subject, status)
}

func buildTaskNotificationContinuationPrompt(item taskNotification) string {
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

func renderCoordinatorHintXML(item taskNotification) string {
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
		if item.Batch.BatchID != "" {
			fmt.Fprintf(&sb, "<batch-id>%s</batch-id>\n", escapeXMLText(item.Batch.BatchID))
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

func coordinatorHintDecision(item taskNotification) orchestration.CoordinatorDecision {
	return orchestration.Decide(item.Status, item.Count, toOrchestrationBatch(item.Batch))
}

func toOrchestrationBatch(batch *backgroundBatchSnapshot) *orchestration.BatchSnapshot {
	if batch == nil {
		return nil
	}
	workers := make([]orchestration.BatchWorker, 0, len(batch.Siblings))
	for _, sibling := range batch.Siblings {
		workers = append(workers, orchestration.BatchWorker{
			TaskID:    sibling.TaskID,
			Subject:   sibling.Subject,
			Status:    sibling.Status,
			AgentType: sibling.AgentType,
		})
	}
	return &orchestration.BatchSnapshot{
		ID:        batch.BatchID,
		Subject:   batch.Subject,
		Status:    batch.Status,
		Completed: batch.Completed,
		Total:     batch.Total,
		Failures:  batch.Failures,
		Workers:   workers,
	}
}
