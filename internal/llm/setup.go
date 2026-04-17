package llm

import "context"

// DefaultSetup is the singleton LLM setup, initialized by Initialize().
var DefaultSetup = &Setup{}

// Setup holds the initialized LLM provider state needed by the app layer.
type Setup struct {
	Store        *Store
	Provider     Provider
	CurrentModel *CurrentModelInfo
}

// Initialize discovers and connects to the best available LLM provider.
// Sets DefaultSetup as a side effect.
func Initialize() {
	store, _ := NewStore()
	if store == nil {
		return
	}

	DefaultSetup.Store = store
	DefaultSetup.CurrentModel = store.GetCurrentModel()
	ctx := context.Background()

	if DefaultSetup.CurrentModel != nil {
		if p, err := GetProvider(ctx, DefaultSetup.CurrentModel.Provider, DefaultSetup.CurrentModel.AuthMethod); err == nil {
			DefaultSetup.Provider = p
			return
		}
	}

	for providerName, conn := range store.GetConnections() {
		if p, err := GetProvider(ctx, Name(providerName), conn.AuthMethod); err == nil {
			DefaultSetup.Provider = p
			return
		}
	}
}

// ModelID returns the current model ID, or empty string if none.
func (s *Setup) ModelID() string {
	if s.CurrentModel != nil {
		return s.CurrentModel.ModelID
	}
	return ""
}
