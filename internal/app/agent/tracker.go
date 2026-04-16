package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

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
	if existing := FindBatchTracker(batchKey); existing != nil {
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
			Completed: MetadataInt(existing.Metadata, BackgroundTrackerCompleted),
			Total:     total,
			Failures:  MetadataInt(existing.Metadata, BackgroundTrackerFailures),
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
	if existing := FindTrackerByMetadata(BackgroundTrackerTaskID, launch.TaskID); existing != nil {
		_ = tracker.DefaultStore.Update(existing.ID,
			tracker.WithSubject(BackgroundWorkerSubject(launch)),
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
		BackgroundWorkerSubject(launch),
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
	trackerTask := FindTrackerByMetadata(BackgroundTrackerTaskID, info.ID)
	if trackerTask == nil {
		orchestration.DefaultStore.RecordCompletion(info.ID, string(info.Status), info.AgentSessionID)
		return
	}

	subject := trackerTask.Subject
	if subject == "" {
		subject = BackgroundWorkerSubject(BackgroundTaskLaunch{
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
		AgentID:      FirstNonEmpty(info.AgentSessionID, MetadataString(trackerTask.Metadata, BackgroundTrackerAgentID)),
		AgentType:    FirstNonEmpty(info.AgentType, MetadataString(trackerTask.Metadata, BackgroundTrackerAgentType)),
		AgentName:    info.AgentName,
		Description:  FirstNonEmpty(info.Description, trackerTask.Description),
		Status:       statusDetail,
		Running:      info.Status == task.StatusRunning,
		BatchID:      MetadataString(trackerTask.Metadata, BackgroundTrackerParentID),
		BatchKey:     MetadataString(trackerTask.Metadata, BackgroundTrackerBatchKey),
		BatchSubject: BackgroundBatchSubject(MetadataString(trackerTask.Metadata, BackgroundTrackerParentID)),
	})
	orchestration.DefaultStore.RecordCompletion(info.ID, statusDetail, FirstNonEmpty(info.AgentSessionID, MetadataString(trackerTask.Metadata, BackgroundTrackerAgentID)))

	if parentID := MetadataString(trackerTask.Metadata, BackgroundTrackerParentID); parentID != "" {
		ReconcileBackgroundBatch(parentID)
	}
}

func ReconcileBackgroundBatch(parentID string) {
	parent, ok := tracker.DefaultStore.Get(parentID)
	if !ok {
		return
	}

	children := ChildTrackers(parentID)
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

func BackgroundWorkerSubject(launch BackgroundTaskLaunch) string {
	if s := joinNameDesc(launch.AgentName, launch.Description); s != "" {
		return s
	}
	if launch.AgentType != "" {
		return launch.AgentType
	}
	return launch.TaskID
}

func FindTrackerByMetadata(key, want string) *tracker.Task {
	return tracker.DefaultStore.FindByMetadata(key, want)
}

func FindBatchTracker(batchKey string) *tracker.Task {
	for _, t := range tracker.DefaultStore.List() {
		if MetadataString(t.Metadata, BackgroundTrackerKindKey) == BackgroundTrackerKindBatch &&
			MetadataString(t.Metadata, BackgroundTrackerBatchKey) == batchKey {
			return t
		}
	}
	return nil
}

func ChildTrackers(parentID string) []*tracker.Task {
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

	child := FindTrackerByMetadata(BackgroundTrackerTaskID, taskID)
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

	children := ChildTrackers(parentID)
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
		Completed: MetadataInt(parent.Metadata, BackgroundTrackerCompleted),
		Total:     MetadataInt(parent.Metadata, BackgroundTrackerTotal),
		Failures:  MetadataInt(parent.Metadata, BackgroundTrackerFailures),
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
		BatchSubject: BackgroundBatchSubject(parentID),
		BatchTotal:   batchTotal,
	})
}

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func BackgroundBatchSubject(parentID string) string {
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

func MetadataInt(metadata map[string]any, key string) int {
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
