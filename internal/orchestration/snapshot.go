package orchestration

type WorkerSnapshot struct {
	TaskID              string
	AgentID             string
	AgentType           string
	AgentName           string
	Description         string
	Status              string
	Running             bool
	PendingMessageCount int
}

type BatchSnapshot struct {
	ID        string
	Key       string
	Subject   string
	Status    string
	Completed int
	Total     int
	Failures  int
	Workers   []BatchWorker
}

type CoordinatorDecision struct {
	Phase                      string
	RecommendedAction          string
	WaitForRemainingWorkers    bool
	ShouldContinueFailedWorker bool
	ShouldSpawnVerifier        bool
	ShouldFinalizeSummary      bool
}

type Snapshot struct {
	Worker   WorkerSnapshot
	Batch    *BatchSnapshot
	Decision CoordinatorDecision
}

func (s *Store) Snapshot(taskID, agentID, status string, notificationCount int) (*Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	targetTaskID := taskID
	if targetTaskID == "" && agentID != "" {
		targetTaskID = s.agentToTask[agentID]
	}
	if targetTaskID == "" {
		return nil, false
	}

	worker, ok := s.workers[targetTaskID]
	if !ok {
		return nil, false
	}

	snapshot := &Snapshot{
		Worker: WorkerSnapshot{
			TaskID:              worker.TaskID,
			AgentID:             worker.AgentID,
			AgentType:           worker.AgentType,
			AgentName:           worker.AgentName,
			Description:         worker.Description,
			Status:              nonEmpty(status, worker.Status),
			Running:             worker.Running,
			PendingMessageCount: len(worker.PendingMessages),
		},
	}

	if worker.BatchID != "" {
		if batch, ok := s.batches[worker.BatchID]; ok {
			snapshot.Batch = &BatchSnapshot{
				ID:        batch.ID,
				Key:       batch.Key,
				Subject:   batch.Subject,
				Status:    batch.Status,
				Completed: batch.Completed,
				Total:     batch.Total,
				Failures:  batch.Failures,
				Workers:   append([]BatchWorker(nil), batch.Workers...),
			}
		}
	}

	snapshot.Decision = Decide(snapshot.Worker.Status, notificationCount, snapshot.Batch)
	return snapshot, true
}

func Decide(status string, notificationCount int, batch *BatchSnapshot) CoordinatorDecision {
	if batch == nil || batch.Total <= 0 {
		if notificationCount > 1 {
			return CoordinatorDecision{
				Phase:                 "batched_notifications",
				RecommendedAction:     "synthesize_notifications_before_followup",
				ShouldFinalizeSummary: true,
			}
		}
		switch status {
		case "failed", "killed":
			return CoordinatorDecision{
				Phase:                      "single_failure",
				RecommendedAction:          "synthesize_then_decide_recovery",
				ShouldContinueFailedWorker: true,
			}
		default:
			return CoordinatorDecision{
				Phase:                 "single_completion",
				RecommendedAction:     "synthesize_then_decide_followup",
				ShouldFinalizeSummary: true,
			}
		}
	}

	if batch.Completed < batch.Total {
		if batch.Failures > 0 || status == "failed" || status == "killed" {
			return CoordinatorDecision{
				Phase:                      "partial_batch_with_failures",
				RecommendedAction:          "synthesize_partial_results_and_decide_recovery_or_wait",
				WaitForRemainingWorkers:    true,
				ShouldContinueFailedWorker: true,
			}
		}
		return CoordinatorDecision{
			Phase:                   "partial_batch",
			RecommendedAction:       "synthesize_partial_results_and_decide_wait_or_replan",
			WaitForRemainingWorkers: true,
		}
	}

	if batch.Failures > 0 {
		return CoordinatorDecision{
			Phase:                      "completed_batch_with_failures",
			RecommendedAction:          "synthesize_batch_and_recover_failed_workers",
			ShouldContinueFailedWorker: true,
			ShouldSpawnVerifier:        true,
			ShouldFinalizeSummary:      true,
		}
	}
	return CoordinatorDecision{
		Phase:                 "completed_batch",
		RecommendedAction:     "synthesize_batch_and_finalize_or_verify",
		ShouldSpawnVerifier:   true,
		ShouldFinalizeSummary: true,
	}
}
