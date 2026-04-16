package plugin

import "sync"

// ConfigObserver receives notifications when plugin operations persist changes
// to settings files.
type ConfigObserver interface {
	ConfigChanged(source, filePath string)
}

var configObserverState struct {
	mu       sync.RWMutex
	observer ConfigObserver
}

// SetConfigObserver installs or clears the plugin config observer.
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
