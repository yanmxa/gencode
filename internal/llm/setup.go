package llm

import "sync"

// defaultSetup is the package-level LLM setup, initialized by Initialize().
// External callers should use Default() to get the Service singleton.
var defaultSetup = &Setup{}

// Setup holds the initialized LLM provider state needed by the app layer.
type Setup struct {
	mu           sync.RWMutex
	Store        *Store
	Provider     Provider
	CurrentModel *CurrentModelInfo
}

// setSingleton publishes defaultSetup as the singleton Service.
func setSingleton() {
	mu.Lock()
	instance = &service{setup: defaultSetup}
	mu.Unlock()
}

// ModelID returns the current model ID, or empty string if none.
func (s *Setup) ModelID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.CurrentModel != nil {
		return s.CurrentModel.ModelID
	}
	return ""
}
