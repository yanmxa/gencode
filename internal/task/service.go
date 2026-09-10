// Package task tracks background bash and subagent tasks for the TUI's
// task panel and the agent's TaskOutput / TaskList tools. Exposes
// *Manager directly.
package task

// Options holds all dependencies for initialization.
type Options struct {
	OutputDir string
}

// Initialize creates the package-level *Manager and configures it.
func Initialize(opts Options) error {
	m := NewManager()
	if opts.OutputDir != "" {
		if err := m.SetOutputDir(opts.OutputDir); err != nil {
			return err
		}
	}
	defaultManager = m
	return nil
}

// Default returns the package-level *Manager.
func Default() *Manager {
	return defaultManager
}

// SetDefaultManager replaces the package-level *Manager. Intended for
// tests. A nil argument restores a fresh empty *Manager.
func SetDefaultManager(m *Manager) {
	if m == nil {
		defaultManager = NewManager()
		return
	}
	defaultManager = m
}

// ResetDefaultManager restores a fresh empty *Manager. Intended for
// tests.
func ResetDefaultManager() {
	defaultManager = NewManager()
}

var defaultManager = NewManager()
