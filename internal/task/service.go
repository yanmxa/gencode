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

// ── singleton ──────────────────────────────────────────────

var (
	svcMu    sync.RWMutex
	instance Service
)

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

// Reset clears the singleton instance. Intended for tests.
func Reset() {
	svcMu.Lock()
	instance = nil
	svcMu.Unlock()
}

// initDefault sets the singleton to the DefaultManager if not already set.
// Called from package init to ensure backward compatibility.
func initDefault() {
	svcMu.Lock()
	defer svcMu.Unlock()
	if instance == nil {
		instance = DefaultManager
	}
}

func init() {
	initDefault()
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
