package mcp

import "sync"

// ConfigObserver receives notifications when MCP config files are written.
type ConfigObserver interface {
	ConfigChanged(source, filePath string)
}

var configObserverState struct {
	mu       sync.RWMutex
	observer ConfigObserver
}

// SetConfigObserver installs or clears the MCP config observer.
func SetConfigObserver(observer ConfigObserver) {
	configObserverState.mu.Lock()
	defer configObserverState.mu.Unlock()
	configObserverState.observer = observer
}

func notifyConfigChanged(source, filePath string) {
	configObserverState.mu.RLock()
	observer := configObserverState.observer
	configObserverState.mu.RUnlock()
	if observer != nil {
		observer.ConfigChanged(source, filePath)
	}
}

func scopeConfigSource(scope Scope) string {
	switch scope {
	case ScopeProject:
		return "project_settings"
	case ScopeLocal:
		return "local_settings"
	default:
		return "user_settings"
	}
}
