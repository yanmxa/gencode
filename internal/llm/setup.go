package llm

// defaultSetup is the package-level LLM setup, initialized by Initialize().
// External callers should use Default() to get the Service singleton.
var defaultSetup = &Setup{}

// Setup holds the initialized LLM provider state needed by the app layer.
type Setup struct {
	Store        *Store
	Provider     Provider
	CurrentModel *CurrentModelInfo
}

// setSingleton publishes defaultSetup as the singleton Service.
func setSingleton() {
	svcMu.Lock()
	svcInstance = &service{setup: defaultSetup}
	svcMu.Unlock()
}

// ModelID returns the current model ID, or empty string if none.
func (s *Setup) ModelID() string {
	if s.CurrentModel != nil {
		return s.CurrentModel.ModelID
	}
	return ""
}
