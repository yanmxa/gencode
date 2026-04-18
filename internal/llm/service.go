package llm

import "sync"

// Service is the public contract for the llm module.
type Service interface {
	// connection
	Provider() Provider              // current active provider
	SetProvider(p Provider)          // switch provider
	ModelID() string                 // current model identifier
	CurrentModel() *CurrentModelInfo // full model metadata
	SetCurrentModel(info *CurrentModelInfo)

	// factory
	NewClient(model string, maxTokens int) *Client

	// store
	Store() *Store // underlying provider persistence store

	// registry
	ListProviders() map[Name][]Info // all registered providers with status
}

// Compile-time check: *service implements Service.
var _ Service = (*service)(nil)

// -- singleton ---------------------------------------------------------------

var (
	svcMu      sync.RWMutex
	svcInstance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	svcMu.RLock()
	s := svcInstance
	svcMu.RUnlock()
	if s == nil {
		panic("llm: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	svcInstance = s
	svcMu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	svcInstance = nil
	svcMu.Unlock()
}

// -- implementation ----------------------------------------------------------

// service wraps the legacy Setup struct to satisfy the Service interface.
// It delegates to DefaultSetup's exported fields, preserving backward
// compatibility with existing callers that access DefaultSetup directly.
type service struct {
	setup *Setup
}

func (s *service) Provider() Provider              { return s.setup.Provider }
func (s *service) SetProvider(p Provider)           { s.setup.Provider = p }
func (s *service) ModelID() string                  { return s.setup.ModelID() }
func (s *service) CurrentModel() *CurrentModelInfo  { return s.setup.CurrentModel }
func (s *service) SetCurrentModel(info *CurrentModelInfo) { s.setup.CurrentModel = info }
func (s *service) Store() *Store                    { return s.setup.Store }

func (s *service) NewClient(model string, maxTokens int) *Client {
	return NewClient(s.setup.Provider, model, maxTokens)
}

func (s *service) ListProviders() map[Name][]Info {
	return GetProvidersWithStatus(s.setup.Store)
}
