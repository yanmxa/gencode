package agent

import (
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
)

// TaskNotificationInput holds the inputs needed to build a task notification.
type TaskNotificationInput struct {
	Info    task.TaskInfo
	Subject string
	Batch   *orchestration.Batch
}

// BuildTaskNotification creates a Notification from a completed task.
// Returns (Notification{}, false) if the task status is not a terminal state.
func BuildTaskNotification(in TaskNotificationInput) (Notification, bool) {
	status := formatStatus(in.Info.Status)
	if status == "" {
		return Notification{}, false
	}

	notice := fallbackNotice(in.Subject, status)
	summary := buildNotificationSummary(in.Info.Type, in.Subject, status)
	result := strings.TrimSpace(in.Info.Output)
	if len(result) > 4000 {
		result = strings.TrimSpace(result[:4000]) + "\n...[truncated]"
	}

	var b strings.Builder
	b.WriteString("<task-notification>\n")
	fmt.Fprintf(&b, "<task-id>%s</task-id>\n", escapeXMLText(in.Info.ID))
	fmt.Fprintf(&b, "<task-type>%s</task-type>\n", escapeXMLText(string(in.Info.Type)))
	fmt.Fprintf(&b, "<status>%s</status>\n", escapeXMLText(status))
	fmt.Fprintf(&b, "<summary>%s</summary>\n", escapeXMLText(summary))
	if in.Info.OutputFile != "" {
		fmt.Fprintf(&b, "<output-file>%s</output-file>\n", escapeXMLText(in.Info.OutputFile))
	}
	if in.Info.Type == task.TaskTypeAgent {
		if in.Info.AgentType != "" {
			fmt.Fprintf(&b, "<agent-type>%s</agent-type>\n", escapeXMLText(in.Info.AgentType))
		}
		if in.Info.AgentName != "" {
			fmt.Fprintf(&b, "<agent>%s</agent>\n", escapeXMLText(in.Info.AgentName))
		}
		if in.Info.AgentSessionID != "" {
			fmt.Fprintf(&b, "<agent-id>%s</agent-id>\n", escapeXMLText(in.Info.AgentSessionID))
		}
		if in.Info.TurnCount > 0 || in.Info.TokenUsage > 0 {
			b.WriteString("<usage>\n")
			if in.Info.TurnCount > 0 {
				fmt.Fprintf(&b, "  <turns>%d</turns>\n", in.Info.TurnCount)
			}
			if in.Info.TokenUsage > 0 {
				fmt.Fprintf(&b, "  <total_tokens>%d</total_tokens>\n", in.Info.TokenUsage)
			}
			b.WriteString("</usage>\n")
		}
	}
	if in.Batch != nil {
		b.WriteString(renderBatchXML(in.Batch, in.Info.ID))
	}
	if result != "" {
		fmt.Fprintf(&b, "<result>%s</result>\n", escapeXMLText(result))
	}
	if in.Info.Error != "" {
		fmt.Fprintf(&b, "<error>%s</error>\n", escapeXMLText(in.Info.Error))
	}
	b.WriteString("</task-notification>")

	return Notification{
		Notice:             notice,
		Context:            backgroundNotificationContext(),
		ContinuationPrompt: b.String(),
		Count:              1,
		TaskID:             in.Info.ID,
		Subject:            in.Subject,
		Status:             status,
		Batch:              in.Batch,
	}, true
}
