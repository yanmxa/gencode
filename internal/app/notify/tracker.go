package notify

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
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
	if !streamActive && tracker.Default().AllDone() {
		tracker.Default().Reset()
	}
}

func PopReadyNotifications(queue *NotificationQueue, idle bool) []Notification {
	if queue == nil || !idle {
		return nil
	}
	return queue.PopBatch(maxPerContinuation)
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

func ensureBackgroundBatchTracker(batchKey string, total int) string {
	if batchKey == "" || total <= 1 {
		return ""
	}
	if existing := findBatchTracker(batchKey); existing != nil {
		_ = tracker.Default().Update(existing.ID,
			tracker.WithStatus(tracker.StatusInProgress),
			tracker.WithMetadata(map[string]any{
				BackgroundTrackerKindKey:  BackgroundTrackerKindBatch,
				BackgroundTrackerBatchKey: batchKey,
				BackgroundTrackerTotal:    total,
			}),
		)
		orchestration.Default().UpdateBatch(orchestration.Batch{
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

	batch := tracker.Default().Create(
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
	_ = tracker.Default().Update(batch.ID, tracker.WithStatus(tracker.StatusInProgress))
	orchestration.Default().UpdateBatch(orchestration.Batch{
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
		_ = tracker.Default().Update(existing.ID,
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

	trackerEntry := tracker.Default().Create(
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
	_ = tracker.Default().Update(trackerEntry.ID, opts...)
	return trackerEntry.ID
}

func UpdateBackgroundWorkerTracker(info task.TaskInfo) {
	trackerTask := findTrackerByMetadata(BackgroundTrackerTaskID, info.ID)
	if trackerTask == nil {
		orchestration.Default().RecordCompletion(info.ID, string(info.Status), info.AgentSessionID)
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
	_ = tracker.Default().Update(trackerTask.ID, opts...)
	orchestration.Default().RecordLaunch(orchestration.Launch{
		TaskID:       info.ID,
		AgentID:      firstNonEmpty(info.AgentSessionID, metadataString(trackerTask.Metadata, BackgroundTrackerAgentID)),
		AgentType:    firstNonEmpty(info.AgentType, metadataString(trackerTask.Metadata, BackgroundTrackerAgentType)),
		AgentName:    info.AgentName,
		Description:  firstNonEmpty(info.Description, trackerTask.Description),
		Status:       statusDetail,
		Running:      info.Status == task.StatusRunning,
		BatchID:      metadataString(trackerTask.Metadata, BackgroundTrackerParentID),
		BatchKey:     metadataString(trackerTask.Metadata, BackgroundTrackerBatchKey),
		BatchSubject: backgroundBatchSubject(metadataString(trackerTask.Metadata, BackgroundTrackerParentID)),
	})
	orchestration.Default().RecordCompletion(info.ID, statusDetail, firstNonEmpty(info.AgentSessionID, metadataString(trackerTask.Metadata, BackgroundTrackerAgentID)))

	if parentID := metadataString(trackerTask.Metadata, BackgroundTrackerParentID); parentID != "" {
		reconcileBackgroundBatch(parentID)
	}
}

func reconcileBackgroundBatch(parentID string) {
	parent, ok := tracker.Default().Get(parentID)
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
			switch metadataString(child.Metadata, BackgroundTrackerStatusDetail) {
			case string(task.StatusFailed), string(task.StatusKilled):
				failures++
			}
		}
	}

	status := tracker.StatusInProgress
	if completed == total {
		status = tracker.StatusCompleted
	}

	_ = tracker.Default().Update(parent.ID,
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
			TaskID:    metadataString(child.Metadata, BackgroundTrackerTaskID),
			Subject:   child.Subject,
			Status:    metadataString(child.Metadata, BackgroundTrackerStatusDetail),
			AgentType: metadataString(child.Metadata, BackgroundTrackerAgentType),
		})
	}
	orchestration.Default().UpdateBatch(orchestration.Batch{
		ID:        parent.ID,
		Key:       metadataString(parent.Metadata, BackgroundTrackerBatchKey),
		Subject:   parent.Subject,
		Status:    status,
		Completed: completed,
		Total:     total,
		Failures:  failures,
		Workers:   workers,
	})
}

func SnapshotBackgroundBatchForTask(taskID string) *orchestration.Batch {
	if batch, ok := orchestration.Default().SnapshotBatchForTask(taskID); ok {
		return batch
	}

	child := findTrackerByMetadata(BackgroundTrackerTaskID, taskID)
	if child == nil {
		return nil
	}
	parentID := metadataString(child.Metadata, BackgroundTrackerParentID)
	if parentID == "" {
		return nil
	}

	parent, ok := tracker.Default().Get(parentID)
	if !ok {
		return nil
	}

	children := childTrackers(parentID)
	workers := make([]orchestration.BatchWorker, 0, len(children))
	for _, c := range children {
		status := metadataString(c.Metadata, BackgroundTrackerStatusDetail)
		if status == "" {
			status = c.Status
		}
		workers = append(workers, orchestration.BatchWorker{
			TaskID:    metadataString(c.Metadata, BackgroundTrackerTaskID),
			Subject:   c.Subject,
			Status:    status,
			AgentType: metadataString(c.Metadata, BackgroundTrackerAgentType),
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
	orchestration.Default().RecordLaunch(orchestration.Launch{
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

// --- Helpers ---

func backgroundWorkerSubject(launch BackgroundTaskLaunch) string {
	if s := joinNameDesc(launch.AgentName, launch.Description); s != "" {
		return s
	}
	if launch.AgentType != "" {
		return launch.AgentType
	}
	return launch.TaskID
}

func findTrackerByMetadata(key, want string) *tracker.Task {
	return tracker.Default().FindByMetadata(key, want)
}

func findBatchTracker(batchKey string) *tracker.Task {
	for _, t := range tracker.Default().List() {
		if metadataString(t.Metadata, BackgroundTrackerKindKey) == BackgroundTrackerKindBatch &&
			metadataString(t.Metadata, BackgroundTrackerBatchKey) == batchKey {
			return t
		}
	}
	return nil
}

func childTrackers(parentID string) []*tracker.Task {
	var children []*tracker.Task
	for _, t := range tracker.Default().List() {
		if metadataString(t.Metadata, BackgroundTrackerParentID) == parentID {
			children = append(children, t)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].CreatedAt.Before(children[j].CreatedAt)
	})
	return children
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
	parent, ok := tracker.Default().Get(parentID)
	if !ok {
		return ""
	}
	return parent.Subject
}

func metadataString(metadata map[string]any, key string) string {
	return kit.MapString(metadata, key)
}

func metadataInt(metadata map[string]any, key string) int {
	return kit.MapInt(metadata, key)
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
