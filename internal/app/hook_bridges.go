package app

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/worktree"
)

type taskHookBridge struct {
	engine            *hooks.Engine
	taskNotifications *taskNotificationQueue
}

func (b taskHookBridge) TaskCreated(info task.TaskInfo) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.TaskCreated, hooks.HookInput{
		TaskID:          info.ID,
		TaskSubject:     taskSubject(info),
		TaskDescription: info.Description,
	})
}

func (b taskHookBridge) TaskCompleted(info task.TaskInfo) {
	if b.engine == nil {
		goto enqueue
	}
	b.engine.ExecuteAsync(hooks.TaskCompleted, hooks.HookInput{
		TaskID:          info.ID,
		TaskSubject:     taskSubject(info),
		TaskDescription: info.Description,
	})

enqueue:
	updateBackgroundWorkerTracker(info)
	if b.taskNotifications == nil {
		return
	}
	if item, ok := buildTaskNotification(info); ok {
		b.taskNotifications.Push(item)
	}
}

type worktreeHookBridge struct {
	engine *hooks.Engine
}

type configHookBridge struct {
	engine *hooks.Engine
}

func (b worktreeHookBridge) WorktreeCreated(name, path string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.WorktreeCreate, hooks.HookInput{
		Name:         name,
		WorktreePath: path,
	})
}

func (b worktreeHookBridge) WorktreeRemoved(path string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.WorktreeRemove, hooks.HookInput{
		WorktreePath: path,
	})
}

func (b configHookBridge) ConfigChanged(source, filePath string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hooks.ConfigChange, hooks.HookInput{
		Source:   source,
		FilePath: filePath,
	})
	b.engine.ExecuteAsync(hooks.FileChanged, hooks.HookInput{
		Source:   source,
		FilePath: filePath,
	})
}

func joinNameDesc(name, desc string) string {
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

func taskSubject(info task.TaskInfo) string {
	switch info.Type {
	case task.TaskTypeAgent:
		if s := joinNameDesc(info.AgentName, info.Description); s != "" {
			return s
		}
	case task.TaskTypeBash:
		if info.Command != "" {
			return info.Command
		}
	}
	return info.Description
}

func installHookBridges(engine *hooks.Engine, taskNotifications *taskNotificationQueue) {
	task.SetHookObserver(taskHookBridge{engine: engine, taskNotifications: taskNotifications})
	worktree.SetHookObserver(worktreeHookBridge{engine: engine})
	plugin.SetConfigObserver(configHookBridge{engine: engine})
	mcp.SetConfigObserver(configHookBridge{engine: engine})
}

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

func renderTaskNotificationBatchXML(batch *backgroundBatchSnapshot, currentTaskID string) string {
	if batch == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("<batch>\n")
	if batch.BatchID != "" {
		fmt.Fprintf(&b, "<id>%s</id>\n", escapeXMLText(batch.BatchID))
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
	if len(batch.Siblings) > 0 {
		b.WriteString("<tasks>\n")
		for _, sibling := range batch.Siblings {
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

func describeBackgroundBatch(batch *backgroundBatchSnapshot) string {
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
