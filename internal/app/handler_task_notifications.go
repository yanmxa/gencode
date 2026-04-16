package app

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tracker"
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

	// Reset tracker store when all tasks are completed and the LLM is idle.
	// This was previously done inside the render function, but side effects
	// belong in the Update cycle, not in View.
	if !m.conv.Stream.Active && tracker.DefaultStore.AllDone() {
		tracker.DefaultStore.Reset()
	}

	if m.taskNotifications == nil {
		return tea.Batch(cmds...)
	}
	if m.conv.Stream.Active || m.isToolPhaseActive() {
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

func sharedTaskNotificationBatch(items []taskNotification) *orchestration.Batch {
	if len(items) == 0 || items[0].Batch == nil || items[0].Batch.ID == "" {
		return nil
	}
	first := items[0].Batch.ID
	for _, item := range items[1:] {
		if item.Batch == nil || item.Batch.ID != first {
			return nil
		}
	}
	// Use the last item's batch snapshot — it was captured most recently
	// and has the most up-to-date completion counts.
	return items[len(items)-1].Batch
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

func coordinatorHintDecision(item taskNotification) orchestration.CoordinatorDecision {
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

// --- Task notification building (XML rendering) ---

func buildTaskNotification(info task.TaskInfo) (taskNotification, bool) {
	status := formatTaskNotificationStatus(info.Status)
	if status == "" {
		return taskNotification{}, false
	}

	subject := taskSubject(info)
	notice := fallbackTaskCompletionNotice(subject, status)
	summary := buildTaskNotificationSummary(info, status)
	batch := snapshotBackgroundBatchForTask(info.ID)
	result := strings.TrimSpace(info.Output)
	if len(result) > 4000 {
		result = strings.TrimSpace(result[:4000]) + "\n...[truncated]"
	}

	var b strings.Builder
	b.WriteString("<task-notification>\n")
	fmt.Fprintf(&b, "<task-id>%s</task-id>\n", escapeXMLText(info.ID))
	fmt.Fprintf(&b, "<task-type>%s</task-type>\n", escapeXMLText(string(info.Type)))
	fmt.Fprintf(&b, "<status>%s</status>\n", escapeXMLText(status))
	fmt.Fprintf(&b, "<summary>%s</summary>\n", escapeXMLText(summary))
	if info.OutputFile != "" {
		fmt.Fprintf(&b, "<output-file>%s</output-file>\n", escapeXMLText(info.OutputFile))
	}
	if info.Type == task.TaskTypeAgent {
		if info.AgentType != "" {
			fmt.Fprintf(&b, "<agent-type>%s</agent-type>\n", escapeXMLText(info.AgentType))
		}
		if info.AgentName != "" {
			fmt.Fprintf(&b, "<agent>%s</agent>\n", escapeXMLText(info.AgentName))
		}
		if info.AgentSessionID != "" {
			fmt.Fprintf(&b, "<agent-id>%s</agent-id>\n", escapeXMLText(info.AgentSessionID))
		}
		if info.TurnCount > 0 || info.TokenUsage > 0 {
			b.WriteString("<usage>\n")
			if info.TurnCount > 0 {
				fmt.Fprintf(&b, "  <turns>%d</turns>\n", info.TurnCount)
			}
			if info.TokenUsage > 0 {
				fmt.Fprintf(&b, "  <total_tokens>%d</total_tokens>\n", info.TokenUsage)
			}
			b.WriteString("</usage>\n")
		}
	}
	if batch != nil {
		b.WriteString(renderTaskNotificationBatchXML(batch, info.ID))
	}
	if result != "" {
		fmt.Fprintf(&b, "<result>%s</result>\n", escapeXMLText(result))
	}
	if info.Error != "" {
		fmt.Fprintf(&b, "<error>%s</error>\n", escapeXMLText(info.Error))
	}
	b.WriteString("</task-notification>")

	return taskNotification{
		Notice:             notice,
		Context:            backgroundTaskNotificationContext(),
		ContinuationPrompt: b.String(),
		Count:              1,
		TaskID:             info.ID,
		Subject:            subject,
		Status:             status,
		Batch:              batch,
	}, true
}

func buildTaskNotificationSummary(info task.TaskInfo, status string) string {
	subject := taskSubject(info)
	switch info.Type {
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

func formatTaskNotificationStatus(status task.TaskStatus) string {
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

func renderTaskNotificationBatchXML(batch *orchestration.Batch, currentTaskID string) string {
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
		fmt.Fprintf(&b, "<summary>%s</summary>\n", escapeXMLText(describeBackgroundBatch(batch)))
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

func describeBackgroundBatch(batch *orchestration.Batch) string {
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
