package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tracker"
)

const (
	backgroundTrackerKindKey      = "background_kind"
	backgroundTrackerKindBatch    = "batch"
	backgroundTrackerKindWorker   = "worker"
	backgroundTrackerBatchKey     = "background_batch_key"
	backgroundTrackerParentID     = "background_parent_id"
	backgroundTrackerTaskID       = "background_task_id"
	backgroundTrackerAgentType    = "background_agent_type"
	backgroundTrackerAgentID      = "background_agent_id"
	backgroundTrackerStatusDetail = "background_status_detail"
	backgroundTrackerCompleted    = "background_completed"
	backgroundTrackerTotal        = "background_total"
	backgroundTrackerFailures     = "background_failures"
)

type backgroundTaskLaunch struct {
	TaskID      string
	AgentName   string
	AgentType   string
	Description string
	ResumeID    string
}

func (m *model) syncBackgroundTaskTracker(msg tooluiExecResultLike) {
	launch, ok := extractBackgroundTaskLaunch(msg)
	if !ok {
		return
	}

	batchKey, batchSize := backgroundBatchSpec(m.tool.PendingCalls)
	parentID := ""
	if batchSize > 1 {
		parentID = ensureBackgroundBatchTracker(batchKey, batchSize)
	}

	childID := ensureBackgroundWorkerTracker(launch, parentID, batchKey)
	if childID == "" {
		return
	}
	if parentID != "" {
		reconcileBackgroundBatch(parentID)
	}
	recordBackgroundTaskLaunch(launch, parentID, batchKey, batchSize)

	if bgTask, found := task.DefaultManager.Get(launch.TaskID); found && !bgTask.IsRunning() {
		updateBackgroundWorkerTracker(bgTask.GetStatus())
	}
}

type tooluiExecResultLike struct {
	ToolName string
	Result   message.ToolResult
}

func extractBackgroundTaskLaunch(msg tooluiExecResultLike) (backgroundTaskLaunch, bool) {
	if !tool.IsAgentToolName(msg.ToolName) {
		return backgroundTaskLaunch{}, false
	}
	resp, ok := msg.Result.HookResponse.(map[string]any)
	if !ok {
		return backgroundTaskLaunch{}, false
	}
	bg, ok := resp["backgroundTask"].(map[string]any)
	if !ok {
		return backgroundTaskLaunch{}, false
	}

	launch := backgroundTaskLaunch{
		TaskID:      metadataString(bg, "taskId"),
		AgentName:   metadataString(bg, "agentName"),
		AgentType:   metadataString(bg, "agentType"),
		Description: metadataString(bg, "description"),
		ResumeID:    metadataString(bg, "resumeId"),
	}
	if launch.TaskID == "" {
		return backgroundTaskLaunch{}, false
	}
	return launch, true
}

func backgroundBatchSpec(calls []message.ToolCall) (string, int) {
	var ids []string
	for _, tc := range calls {
		if !tool.IsAgentToolName(tc.Name) || !toolCallRunsInBackground(tc.Input) {
			continue
		}
		ids = append(ids, tc.ID)
	}
	if len(ids) <= 1 {
		return "", len(ids)
	}
	return strings.Join(ids, ","), len(ids)
}

func toolCallRunsInBackground(input string) bool {
	params, err := message.ParseToolInput(input)
	if err != nil {
		return false
	}
	runInBackground, _ := params["run_in_background"].(bool)
	return runInBackground
}

func ensureBackgroundBatchTracker(batchKey string, total int) string {
	if batchKey == "" || total <= 1 {
		return ""
	}
	if existing := findBatchTracker(batchKey); existing != nil {
		_ = tracker.DefaultStore.Update(existing.ID,
			tracker.WithStatus(tracker.StatusInProgress),
			tracker.WithMetadata(map[string]any{
				backgroundTrackerKindKey:  backgroundTrackerKindBatch,
				backgroundTrackerBatchKey: batchKey,
				backgroundTrackerTotal:    total,
			}),
		)
		orchestration.DefaultStore.UpdateBatch(orchestration.Batch{
			ID:        existing.ID,
			Key:       batchKey,
			Subject:   existing.Subject,
			Status:    tracker.StatusInProgress,
			Completed: metadataInt(existing.Metadata, backgroundTrackerCompleted),
			Total:     total,
			Failures:  metadataInt(existing.Metadata, backgroundTrackerFailures),
		})
		return existing.ID
	}

	batch := tracker.DefaultStore.Create(
		fmt.Sprintf("%d background agents launched", total),
		"Coordinator background worker batch",
		"",
		map[string]any{
			backgroundTrackerKindKey:   backgroundTrackerKindBatch,
			backgroundTrackerBatchKey:  batchKey,
			backgroundTrackerCompleted: 0,
			backgroundTrackerTotal:     total,
			backgroundTrackerFailures:  0,
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

func ensureBackgroundWorkerTracker(launch backgroundTaskLaunch, parentID, batchKey string) string {
	if existing := findTrackerByMetadata(backgroundTrackerTaskID, launch.TaskID); existing != nil {
		_ = tracker.DefaultStore.Update(existing.ID,
			tracker.WithSubject(backgroundWorkerSubject(launch)),
			tracker.WithDescription(launch.Description),
			tracker.WithStatus(tracker.StatusInProgress),
			tracker.WithMetadata(map[string]any{
				backgroundTrackerKindKey:      backgroundTrackerKindWorker,
				backgroundTrackerParentID:     parentID,
				backgroundTrackerBatchKey:     batchKey,
				backgroundTrackerTaskID:       launch.TaskID,
				backgroundTrackerAgentType:    launch.AgentType,
				backgroundTrackerAgentID:      launch.ResumeID,
				backgroundTrackerStatusDetail: string(task.StatusRunning),
			}),
		)
		return existing.ID
	}

	trackerEntry := tracker.DefaultStore.Create(
		backgroundWorkerSubject(launch),
		launch.Description,
		"",
		map[string]any{
			backgroundTrackerKindKey:      backgroundTrackerKindWorker,
			backgroundTrackerParentID:     parentID,
			backgroundTrackerBatchKey:     batchKey,
			backgroundTrackerTaskID:       launch.TaskID,
			backgroundTrackerAgentType:    launch.AgentType,
			backgroundTrackerAgentID:      launch.ResumeID,
			backgroundTrackerStatusDetail: string(task.StatusRunning),
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

func updateBackgroundWorkerTracker(info task.TaskInfo) {
	trackerTask := findTrackerByMetadata(backgroundTrackerTaskID, info.ID)
	if trackerTask == nil {
		orchestration.DefaultStore.RecordCompletion(info.ID, string(info.Status), info.AgentSessionID)
		return
	}

	subject := trackerTask.Subject
	if subject == "" {
		subject = backgroundWorkerSubject(backgroundTaskLaunch{
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
		backgroundTrackerKindKey:      backgroundTrackerKindWorker,
		backgroundTrackerTaskID:       info.ID,
		backgroundTrackerAgentType:    info.AgentType,
		backgroundTrackerAgentID:      info.AgentSessionID,
		backgroundTrackerStatusDetail: statusDetail,
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
		AgentID:      firstNonEmpty(info.AgentSessionID, metadataString(trackerTask.Metadata, backgroundTrackerAgentID)),
		AgentType:    firstNonEmpty(info.AgentType, metadataString(trackerTask.Metadata, backgroundTrackerAgentType)),
		AgentName:    info.AgentName,
		Description:  firstNonEmpty(info.Description, trackerTask.Description),
		Status:       statusDetail,
		Running:      info.Status == task.StatusRunning,
		BatchID:      metadataString(trackerTask.Metadata, backgroundTrackerParentID),
		BatchKey:     metadataString(trackerTask.Metadata, backgroundTrackerBatchKey),
		BatchSubject: backgroundBatchSubject(metadataString(trackerTask.Metadata, backgroundTrackerParentID)),
	})
	orchestration.DefaultStore.RecordCompletion(info.ID, statusDetail, firstNonEmpty(info.AgentSessionID, metadataString(trackerTask.Metadata, backgroundTrackerAgentID)))

	if parentID := metadataString(trackerTask.Metadata, backgroundTrackerParentID); parentID != "" {
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
			switch metadataString(child.Metadata, backgroundTrackerStatusDetail) {
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
			backgroundTrackerKindKey:   backgroundTrackerKindBatch,
			backgroundTrackerCompleted: completed,
			backgroundTrackerTotal:     total,
			backgroundTrackerFailures:  failures,
		}),
	)

	workers := make([]orchestration.BatchWorker, 0, len(children))
	for _, child := range children {
		workers = append(workers, orchestration.BatchWorker{
			TaskID:    metadataString(child.Metadata, backgroundTrackerTaskID),
			Subject:   child.Subject,
			Status:    metadataString(child.Metadata, backgroundTrackerStatusDetail),
			AgentType: metadataString(child.Metadata, backgroundTrackerAgentType),
		})
	}
	orchestration.DefaultStore.UpdateBatch(orchestration.Batch{
		ID:        parent.ID,
		Key:       metadataString(parent.Metadata, backgroundTrackerBatchKey),
		Subject:   parent.Subject,
		Status:    status,
		Completed: completed,
		Total:     total,
		Failures:  failures,
		Workers:   workers,
	})
}

func backgroundWorkerSubject(launch backgroundTaskLaunch) string {
	if s := joinNameDesc(launch.AgentName, launch.Description); s != "" {
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

// findBatchTracker finds a batch tracker entry by its batch key.
// Unlike findTrackerByMetadata, this also verifies the entry is a batch (not a worker),
// since both batch and worker entries share the same batchKey metadata.
func findBatchTracker(batchKey string) *tracker.Task {
	for _, t := range tracker.DefaultStore.List() {
		if metadataString(t.Metadata, backgroundTrackerKindKey) == backgroundTrackerKindBatch &&
			metadataString(t.Metadata, backgroundTrackerBatchKey) == batchKey {
			return t
		}
	}
	return nil
}

func childTrackers(parentID string) []*tracker.Task {
	var children []*tracker.Task
	for _, t := range tracker.DefaultStore.List() {
		if metadataString(t.Metadata, backgroundTrackerParentID) == parentID {
			children = append(children, t)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].CreatedAt.Before(children[j].CreatedAt)
	})
	return children
}

func snapshotBackgroundBatchForTask(taskID string) *orchestration.Batch {
	if batch, ok := orchestration.DefaultStore.SnapshotBatchForTask(taskID); ok {
		return batch
	}

	child := findTrackerByMetadata(backgroundTrackerTaskID, taskID)
	if child == nil {
		return nil
	}
	parentID := metadataString(child.Metadata, backgroundTrackerParentID)
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
		status := metadataString(c.Metadata, backgroundTrackerStatusDetail)
		if status == "" {
			status = c.Status
		}
		workers = append(workers, orchestration.BatchWorker{
			TaskID:    metadataString(c.Metadata, backgroundTrackerTaskID),
			Subject:   c.Subject,
			Status:    status,
			AgentType: metadataString(c.Metadata, backgroundTrackerAgentType),
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
		Completed: metadataInt(parent.Metadata, backgroundTrackerCompleted),
		Total:     metadataInt(parent.Metadata, backgroundTrackerTotal),
		Failures:  metadataInt(parent.Metadata, backgroundTrackerFailures),
		Workers:   workers,
	}
}

func recordBackgroundTaskLaunch(launch backgroundTaskLaunch, parentID, batchKey string, batchTotal int) {
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

func metadataString(metadata map[string]any, key string) string {
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

