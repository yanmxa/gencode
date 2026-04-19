package task

import (
	"context"
	"os/exec"
	"sync"
)

// Service is the public contract for the task module.
type Service interface {
	// lifecycle
	RegisterTask(t BackgroundTask)
	CreateBashTask(cmd *exec.Cmd, command, description string, ctx context.Context, cancel context.CancelFunc) *BashTask
	Get(id string) (BackgroundTask, bool)
	List() []BackgroundTask
	ListRunning() []BackgroundTask
	Kill(id string) error
	Remove(id string)

	// observer
	SetObserver(obs CompletionObserver)

	// output
	SetOutputDir(dir string) error
}

// Compile-time check: *Manager implements Service.
var _ Service = (*Manager)(nil)

// Options holds all dependencies for initialization.
type Options struct {
	OutputDir string
}

// ── singleton ──────────────────────────────────────────────

var (
	svcMu    sync.RWMutex
	instance Service
)

// Initialize creates a new Manager, applies options, and sets the singleton.
func Initialize(opts Options) {
	m := NewManager()
	if opts.OutputDir != "" {
		m.SetOutputDir(opts.OutputDir)
	}
	svcMu.Lock()
	instance = m
	svcMu.Unlock()
}

// Default returns the singleton Service instance.
// Panics if not initialized.
func Default() Service {
	svcMu.RLock()
	s := instance
	svcMu.RUnlock()
	if s == nil {
		panic("task: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	instance = s
	svcMu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	instance = nil
	svcMu.Unlock()
}

// ── Service methods on Manager ─────────────────────────────

// SetObserver implements Service by delegating to the package-level
// SetCompletionObserver function.
func (m *Manager) SetObserver(obs CompletionObserver) {
	SetCompletionObserver(obs)
}

// SetOutputDir implements Service by delegating to the package-level
// SetOutputDir function.
func (m *Manager) SetOutputDir(dir string) error {
	return setOutputDir(dir)
}
