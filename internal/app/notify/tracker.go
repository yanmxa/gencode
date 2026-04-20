package notify

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
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

func PopReady(queue *Queue, idle bool) []Message {
	if queue == nil || !idle {
		return nil
	}
	return queue.PopBatch(maxPerContinuation)
}

// --- Background task tracker (TUI display) ---

const (
	metaTaskID       = "background_task_id"
	metaStatusDetail = "background_status_detail"
)

type BackgroundTaskLaunch struct {
	TaskID      string
	AgentName   string
	AgentType   string
	Description string
}

type BackgroundTracker struct {
	tracker tracker.Service
}

func NewBackgroundTracker(t tracker.Service) *BackgroundTracker {
	return &BackgroundTracker{tracker: t}
}

func (bt *BackgroundTracker) ResetIfIdle(streamActive bool) {
	if !streamActive && bt.tracker.AllDone() {
		bt.tracker.Reset()
	}
}

func (bt *BackgroundTracker) TrackWorker(launch BackgroundTaskLaunch) {
	if existing := bt.tracker.FindByMetadata(metaTaskID, launch.TaskID); existing != nil {
		_ = bt.tracker.Update(existing.ID,
			tracker.WithSubject(workerSubject(launch)),
			tracker.WithDescription(launch.Description),
			tracker.WithStatus(tracker.StatusInProgress),
			tracker.WithMetadata(map[string]any{
				metaTaskID:       launch.TaskID,
				metaStatusDetail: string(task.StatusRunning),
			}),
		)
		return
	}

	entry := bt.tracker.Create(
		workerSubject(launch),
		launch.Description,
		"",
		map[string]any{
			metaTaskID:       launch.TaskID,
			metaStatusDetail: string(task.StatusRunning),
		},
	)
	opts := []tracker.UpdateOption{tracker.WithStatus(tracker.StatusInProgress)}
	if launch.AgentType != "" {
		opts = append(opts, tracker.WithOwner(launch.AgentType))
	}
	_ = bt.tracker.Update(entry.ID, opts...)
}

func (bt *BackgroundTracker) CompleteWorker(info task.TaskInfo) {
	entry := bt.tracker.FindByMetadata(metaTaskID, info.ID)
	if entry == nil {
		return
	}

	subject := entry.Subject
	if subject == "" {
		subject = workerSubject(BackgroundTaskLaunch{
			TaskID:      info.ID,
			AgentName:   info.AgentName,
			AgentType:   info.AgentType,
			Description: info.Description,
		})
	}

	statusDetail := string(info.Status)
	if statusDetail == "" {
		statusDetail = string(task.StatusCompleted)
	}

	_ = bt.tracker.Update(entry.ID,
		tracker.WithSubject(subject),
		tracker.WithDescription(info.Description),
		tracker.WithStatus(tracker.StatusCompleted),
		tracker.WithMetadata(map[string]any{
			metaTaskID:       info.ID,
			metaStatusDetail: statusDetail,
		}),
	)
}

// --- Helpers ---

func workerSubject(launch BackgroundTaskLaunch) string {
	if s := joinNameDesc(launch.AgentName, launch.Description); s != "" {
		return s
	}
	if launch.AgentType != "" {
		return launch.AgentType
	}
	return launch.TaskID
}

func metadataString(metadata map[string]any, key string) string {
	return kit.MapString(metadata, key)
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
