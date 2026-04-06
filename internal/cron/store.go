package cron

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	// DefaultExpiry is the auto-expiry duration for recurring jobs.
	DefaultExpiry = 7 * 24 * time.Hour

	// MaxJobs is the maximum number of concurrent cron jobs.
	MaxJobs = 50

	// MaxRecurringJitter bounds recurring schedule spread.
	MaxRecurringJitter = 15 * time.Minute
)

// Job represents a scheduled cron job.
type Job struct {
	ID         string    `json:"id"`
	Cron       string    `json:"cron"`      // 5-field cron expression
	Prompt     string    `json:"prompt"`    // prompt to inject when fired
	Recurring  bool      `json:"recurring"` // true = repeats, false = one-shot
	Durable    bool      `json:"durable"`   // true = persists across sessions
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`  // auto-expiry time (zero = no expiry)
	NextFire   time.Time `json:"nextFire"`   // next scheduled fire time
	LastFired  time.Time `json:"lastFired"`  // last time this job fired
	FiredCount int       `json:"firedCount"` // total times fired

	expr *Expression // parsed expression (not serialized)
}

// Store manages cron jobs with thread-safe access.
// Session-only jobs are cleared when the process exits.
// Durable jobs persist to storagePath across sessions.
type Store struct {
	mu          sync.RWMutex
	jobs        map[string]*Job
	storagePath string // file path for durable job persistence (empty = disabled)
}

// DefaultStore is the global cron store singleton.
var DefaultStore = NewStore()

// NewStore creates a new in-memory cron store.
func NewStore() *Store {
	return &Store{
		jobs: make(map[string]*Job),
	}
}

// Create adds a new cron job and returns it.
func (s *Store) Create(cronExpr, prompt string, recurring, durable bool) (*Job, error) {
	expr, err := Parse(cronExpr)
	if err != nil {
		return nil, err
	}

	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.jobs) >= MaxJobs {
		return nil, fmt.Errorf("cron: maximum number of jobs (%d) reached", MaxJobs)
	}

	expiresAt := time.Time{}
	if recurring {
		expiresAt = now.Add(DefaultExpiry)
	}

	job := &Job{
		ID:        generateID(),
		Cron:      cronExpr,
		Prompt:    prompt,
		Recurring: recurring,
		Durable:   durable,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		expr:      expr,
	}
	job.NextFire = computeNextFire(expr, now, job.ID, recurring)
	if job.NextFire.IsZero() {
		return nil, fmt.Errorf("cron: no valid fire time found for %q", cronExpr)
	}

	s.jobs[job.ID] = job

	if durable {
		s.saveDurableLocked()
	}

	return job, nil
}

// Delete removes a job by ID.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return fmt.Errorf("cron: job %q not found", id)
	}
	wasDurable := job.Durable
	delete(s.jobs, id)

	if wasDurable {
		s.saveDurableLocked()
	}
	return nil
}

// List returns all active jobs sorted by next fire time.
func (s *Store) List() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}

	sort.Slice(jobs, func(i, k int) bool {
		return jobs[i].NextFire.Before(jobs[k].NextFire)
	})
	return jobs
}

// Empty returns true if the store has no jobs.
func (s *Store) Empty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.jobs) == 0
}

// Tick checks all jobs and returns prompts for any that should fire now.
// It advances fired jobs to their next fire time or removes one-shot/expired jobs.
func (s *Store) Tick() []FiredJob {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	var fired []FiredJob
	var toDelete []string
	changed := false

	for _, job := range s.jobs {
		// Check expiry
		if !job.ExpiresAt.IsZero() && now.After(job.ExpiresAt) {
			toDelete = append(toDelete, job.ID)
			if job.Durable {
				changed = true
			}
			continue
		}

		// Check if it should fire
		if now.Before(job.NextFire) {
			continue
		}

		fired = append(fired, FiredJob{
			ID:     job.ID,
			Prompt: job.Prompt,
		})

		job.LastFired = now
		job.FiredCount++
		if job.Durable {
			changed = true
		}

		if !job.Recurring {
			toDelete = append(toDelete, job.ID)
		} else {
			if job.expr == nil {
				if expr, err := Parse(job.Cron); err == nil {
					job.expr = expr
				}
			}
			if job.expr != nil {
				job.NextFire = computeNextFire(job.expr, now, job.ID, true)
			}
		}
	}

	for _, id := range toDelete {
		delete(s.jobs, id)
	}
	if changed {
		s.saveDurableLocked()
	}

	return fired
}

// FiredJob is returned by Tick when a job fires.
type FiredJob struct {
	ID     string
	Prompt string
}

// Reset removes all jobs.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = make(map[string]*Job)
	s.saveDurableLocked()
}

// SetStoragePath sets the file path for durable job persistence.
// Call LoadDurable() after this to restore previously saved jobs.
func (s *Store) SetStoragePath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storagePath = path
}

// LoadDurable reads durable jobs from the storage file and merges them into the store.
func (s *Store) LoadDurable() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.storagePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cron: failed to read durable jobs: %w", err)
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return fmt.Errorf("cron: failed to parse durable jobs: %w", err)
	}

	now := time.Now()
	for _, job := range jobs {
		// Skip expired jobs
		if !job.ExpiresAt.IsZero() && now.After(job.ExpiresAt) {
			continue
		}
		// Re-parse expression
		expr, err := Parse(job.Cron)
		if err != nil {
			continue
		}
		job.expr = expr
		if job.Recurring {
			// Recalculate recurring jobs from "now" so long-lived schedules
			// continue cleanly after restart without replaying missed intervals.
			job.NextFire = computeNextFire(expr, now, job.ID, true)
			if job.NextFire.IsZero() {
				continue
			}
		} else {
			// One-shot durable jobs should catch up after restart instead of
			// being pushed to the cron expression's next future match.
			if job.NextFire.IsZero() {
				job.NextFire = now
			} else if !job.NextFire.After(now) {
				job.NextFire = now
			}
		}
		job.Durable = true
		s.jobs[job.ID] = job
	}

	return nil
}

func computeNextFire(expr *Expression, from time.Time, jobID string, recurring bool) time.Time {
	base := expr.NextAfter(from)
	if base.IsZero() {
		return time.Time{}
	}
	if !recurring {
		return base
	}

	jitter := computeRecurringJitter(expr, base, jobID)
	return base.Add(jitter)
}

func computeRecurringJitter(expr *Expression, base time.Time, jobID string) time.Duration {
	period := estimateRecurringPeriod(expr, base)
	if period <= 0 {
		return 0
	}

	maxJitter := period / 10
	if maxJitter > MaxRecurringJitter {
		maxJitter = MaxRecurringJitter
	}
	if maxJitter <= 0 {
		return 0
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(jobID))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(expr.Raw))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(base.Format("2006-01-02T15:04")))

	return time.Duration(h.Sum64() % uint64(maxJitter))
}

func estimateRecurringPeriod(expr *Expression, base time.Time) time.Duration {
	// Ask for the next matching base time after the current base minute.
	next := expr.NextAfter(base)
	if next.IsZero() {
		return 0
	}
	return next.Sub(base)
}

// saveDurableLocked writes all durable jobs to the storage file.
// Must be called with s.mu held.
func (s *Store) saveDurableLocked() {
	if s.storagePath == "" {
		return
	}

	var durable []*Job
	for _, job := range s.jobs {
		if job.Durable {
			durable = append(durable, job)
		}
	}

	data, err := json.MarshalIndent(durable, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.storagePath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(s.storagePath, data, 0o644)
}

func generateID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
