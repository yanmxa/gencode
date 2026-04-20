package hub

import (
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
)

const (
	metaTaskID       = "background_task_id"
	metaStatusDetail = "background_status_detail"
)

// BackgroundTaskLaunch holds metadata for a newly spawned background task.
type BackgroundTaskLaunch struct {
	TaskID      string
	AgentName   string
	AgentType   string
	Description string
}

// TrackWorker creates or updates a tracker entry for a running background task.
func TrackWorker(svc tracker.Service, launch BackgroundTaskLaunch) {
	if existing := svc.FindByMetadata(metaTaskID, launch.TaskID); existing != nil {
		_ = svc.Update(existing.ID,
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

	entry := svc.Create(
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
	_ = svc.Update(entry.ID, opts...)
}

// CompleteWorker marks a tracker entry as completed.
func CompleteWorker(svc tracker.Service, info task.TaskInfo) {
	entry := svc.FindByMetadata(metaTaskID, info.ID)
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

	_ = svc.Update(entry.ID,
		tracker.WithSubject(subject),
		tracker.WithDescription(info.Description),
		tracker.WithStatus(tracker.StatusCompleted),
		tracker.WithMetadata(map[string]any{
			metaTaskID:       info.ID,
			metaStatusDetail: statusDetail,
		}),
	)
}

func workerSubject(launch BackgroundTaskLaunch) string {
	if s := joinNameDesc(launch.AgentName, launch.Description); s != "" {
		return s
	}
	if launch.AgentType != "" {
		return launch.AgentType
	}
	return launch.TaskID
}
