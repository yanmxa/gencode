package orchestration

import "sync"

type Worker struct {
	TaskID          string
	AgentID         string
	AgentType       string
	AgentName       string
	Description     string
	Status          string
	Running         bool
	BatchID         string
	BatchKey        string
	PendingMessages []string
}

type Batch struct {
	ID        string
	Key       string
	Subject   string
	Status    string
	Completed int
	Total     int
	Failures  int
	Workers   []BatchWorker
}

type BatchWorker struct {
	TaskID    string
	Subject   string
	Status    string
	AgentType string
}

type Launch struct {
	TaskID       string
	AgentID      string
	AgentType    string
	AgentName    string
	Description  string
	Status       string
	Running      bool
	BatchID      string
	BatchKey     string
	BatchSubject string
	BatchTotal   int
}

type Store struct {
	mu           sync.RWMutex
	workers      map[string]*Worker
	agentToTask  map[string]string
	batches      map[string]*Batch
	batchKeyToID map[string]string
}

func newStore() *Store {
	return &Store{
		workers:      make(map[string]*Worker),
		agentToTask:  make(map[string]string),
		batches:      make(map[string]*Batch),
		batchKeyToID: make(map[string]string),
	}
}

var DefaultStore = newStore()

func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers = make(map[string]*Worker)
	s.agentToTask = make(map[string]string)
	s.batches = make(map[string]*Batch)
	s.batchKeyToID = make(map[string]string)
}

func (s *Store) RecordLaunch(launch Launch) {
	if launch.TaskID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	worker, ok := s.workers[launch.TaskID]
	if !ok {
		worker = &Worker{TaskID: launch.TaskID}
		s.workers[launch.TaskID] = worker
	}
	worker.AgentType = nonEmpty(launch.AgentType, worker.AgentType)
	worker.AgentName = nonEmpty(launch.AgentName, worker.AgentName)
	worker.Description = nonEmpty(launch.Description, worker.Description)
	worker.Status = nonEmpty(launch.Status, worker.Status)
	worker.Running = launch.Running
	worker.BatchID = nonEmpty(launch.BatchID, worker.BatchID)
	worker.BatchKey = nonEmpty(launch.BatchKey, worker.BatchKey)
	if launch.AgentID != "" {
		worker.AgentID = launch.AgentID
		s.agentToTask[launch.AgentID] = launch.TaskID
	}

	if launch.BatchID != "" || launch.BatchKey != "" {
		batchID := launch.BatchID
		if batchID == "" {
			batchID = s.batchKeyToID[launch.BatchKey]
		}
		if batchID == "" {
			batchID = launch.BatchKey
		}
		batch, ok := s.batches[batchID]
		if !ok {
			batch = &Batch{ID: batchID}
			s.batches[batchID] = batch
		}
		batch.Key = nonEmpty(launch.BatchKey, batch.Key)
		batch.Subject = nonEmpty(launch.BatchSubject, batch.Subject)
		batch.Total = max(launch.BatchTotal, batch.Total)
		if launch.BatchKey != "" {
			s.batchKeyToID[launch.BatchKey] = batchID
		}
		worker.BatchID = batchID
	}
}

func (s *Store) RecordCompletion(taskID, status, agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	worker, ok := s.workers[taskID]
	if !ok {
		return
	}
	worker.Status = nonEmpty(status, worker.Status)
	worker.Running = false
	if agentID != "" {
		worker.AgentID = agentID
		s.agentToTask[agentID] = taskID
	}
}

func (s *Store) UpdateBatch(batch Batch) {
	if batch.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.batches[batch.ID]
	if !ok {
		current = &Batch{ID: batch.ID}
		s.batches[batch.ID] = current
	}
	current.Key = nonEmpty(batch.Key, current.Key)
	current.Subject = nonEmpty(batch.Subject, current.Subject)
	current.Status = nonEmpty(batch.Status, current.Status)
	current.Completed = batch.Completed
	current.Total = batch.Total
	current.Failures = batch.Failures
	current.Workers = append([]BatchWorker(nil), batch.Workers...)
	if batch.Key != "" {
		s.batchKeyToID[batch.Key] = batch.ID
	}
}

func (s *Store) SnapshotBatchForTask(taskID string) (*Batch, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	worker, ok := s.workers[taskID]
	if !ok || worker.BatchID == "" {
		return nil, false
	}
	batch, ok := s.batches[worker.BatchID]
	if !ok {
		return nil, false
	}
	copyBatch := *batch
	copyBatch.Workers = append([]BatchWorker(nil), batch.Workers...)
	return &copyBatch, true
}

func (s *Store) QueuePendingMessage(taskID, message string) bool {
	if taskID == "" || message == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	worker, ok := s.workers[taskID]
	if !ok {
		worker = &Worker{TaskID: taskID}
		s.workers[taskID] = worker
	}
	worker.PendingMessages = append(worker.PendingMessages, message)
	return true
}

func (s *Store) DrainPendingMessages(taskID, agentID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	targetTaskID := taskID
	if targetTaskID == "" && agentID != "" {
		targetTaskID = s.agentToTask[agentID]
	}
	if targetTaskID == "" {
		return nil
	}

	worker, ok := s.workers[targetTaskID]
	if !ok || len(worker.PendingMessages) == 0 {
		return nil
	}
	messages := append([]string(nil), worker.PendingMessages...)
	worker.PendingMessages = nil
	return messages
}

func (s *Store) PendingMessageCount(taskID, agentID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	targetTaskID := taskID
	if targetTaskID == "" && agentID != "" {
		targetTaskID = s.agentToTask[agentID]
	}
	if targetTaskID == "" {
		return 0
	}
	worker, ok := s.workers[targetTaskID]
	if !ok {
		return 0
	}
	return len(worker.PendingMessages)
}

func (s *Store) ResolveTaskID(agentID string) (string, bool) {
	if agentID == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	taskID, ok := s.agentToTask[agentID]
	return taskID, ok
}

func nonEmpty(next, current string) string {
	if next != "" {
		return next
	}
	return current
}

