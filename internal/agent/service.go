package agent

import (
	"sync"

	"github.com/yanmxa/gencode/internal/core"
)

// Service manages the main agent session lifecycle.
type Service interface {
	// Start builds a core.Agent from params, starts its goroutine.
	// If messages is non-empty, they are loaded as conversation history.
	Start(params BuildParams, messages []core.Message) error

	// Stop cancels the agent goroutine and cleans up.
	Stop()

	// Active reports whether an agent session is running.
	Active() bool

	// Send pushes a user message to the agent's inbox. No-op if not active.
	Send(content string, images []core.Image)

	// Outbox returns the agent's event channel. Nil if not active.
	Outbox() <-chan core.Event

	// PermissionBridge returns the current session's permission bridge.
	PermissionBridge() *PermissionBridge

	// PendingPermission gets the pending permission request.
	PendingPermission() *PermBridgeRequest

	// SetPendingPermission tracks a pending permission request for TUI approval.
	SetPendingPermission(req *PermBridgeRequest)
}

// Options holds dependencies for initialization.
type Options struct{}

// ── singleton ──────────────────────────────────────────────

var (
	mu       sync.RWMutex
	instance Service
)

func init() {
	mu.Lock()
	if instance == nil {
		instance = &service{}
	}
	mu.Unlock()
}

func Initialize(opts Options) {
	mu.Lock()
	instance = &service{}
	mu.Unlock()
}

func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("agent: not initialized")
	}
	return s
}

func SetDefault(s Service) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

func ResetService() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}
