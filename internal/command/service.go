package command

import "sync"

// Service is the public contract for the command module.
type Service interface {
	// Get returns a command by exact name (builtin, dynamic, or custom).
	Get(name string) (Info, bool)
	// List returns all commands (builtin + dynamic + custom).
	List() []Info
	// ListCustom returns all custom (user-defined) commands.
	ListCustom() []CustomCommand
	// GetMatching returns commands whose names fuzzy-match the given prefix.
	GetMatching(prefix string) []Info
	// IsCustomCommand checks whether the given command name matches a custom command.
	IsCustomCommand(cmd string) (*CustomCommand, bool)
	// BuiltinNames returns the set of built-in command names.
	BuiltinNames() map[string]Info
	// GetCustomCommands returns Info entries for all custom commands.
	GetCustomCommands() []Info
}

// Options holds all dependencies for initialization.
type Options struct {
	CWD              string
	DynamicProviders []func() []Info
}

// ── singleton ──────────────────────────────────────────────

var (
	mu       sync.RWMutex
	instance Service
)

// Default returns the singleton Service instance. Panics if not initialized.
func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("command: not initialized")
	}
	return s
}

// SetDefault replaces the singleton (for tests).
func SetDefault(s Service) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

// ResetService clears the singleton (for tests).
func ResetService() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}
