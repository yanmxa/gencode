package llm

import (
	"context"
	"sync"
)

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

// Options holds configuration for Initialize.
type Options struct{}

// Initialize discovers and connects to the best available LLM provider,
// then publishes the result as the singleton Service.
func Initialize(opts Options) {
	store, _ := NewStore()
	if store == nil {
		return
	}

	defaultSetup.mu.Lock()
	defaultSetup.Store = store
	defaultSetup.CurrentModel = store.GetCurrentModel()
	defaultSetup.mu.Unlock()

	ctx := context.Background()

	defaultSetup.mu.RLock()
	cm := defaultSetup.CurrentModel
	defaultSetup.mu.RUnlock()

	if cm != nil {
		if p, err := GetProvider(ctx, cm.Provider, cm.AuthMethod); err == nil {
			defaultSetup.mu.Lock()
			defaultSetup.Provider = p
			defaultSetup.mu.Unlock()
			setSingleton()
			return
		}
	}

	for providerName, conn := range store.GetConnections() {
		if p, err := GetProvider(ctx, Name(providerName), conn.AuthMethod); err == nil {
			defaultSetup.mu.Lock()
			defaultSetup.Provider = p
			defaultSetup.mu.Unlock()
			setSingleton()
			return
		}
	}

	setSingleton()
}

// -- singleton ---------------------------------------------------------------

var (
	mu      sync.RWMutex
	instance Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	mu.RLock()
	s := instance
	mu.RUnlock()
	if s == nil {
		panic("llm: not initialized")
	}
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	mu.Lock()
	instance = s
	mu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	mu.Lock()
	instance = nil
	mu.Unlock()
}

// -- implementation ----------------------------------------------------------

// service wraps the Setup struct to satisfy the Service interface.
type service struct {
	setup *Setup
}

func (s *service) Provider() Provider {
	s.setup.mu.RLock()
	defer s.setup.mu.RUnlock()
	return s.setup.Provider
}

func (s *service) SetProvider(p Provider) {
	s.setup.mu.Lock()
	defer s.setup.mu.Unlock()
	s.setup.Provider = p
}

func (s *service) ModelID() string { return s.setup.ModelID() }

func (s *service) CurrentModel() *CurrentModelInfo {
	s.setup.mu.RLock()
	defer s.setup.mu.RUnlock()
	return s.setup.CurrentModel
}

func (s *service) SetCurrentModel(info *CurrentModelInfo) {
	s.setup.mu.Lock()
	defer s.setup.mu.Unlock()
	s.setup.CurrentModel = info
}

func (s *service) Store() *Store {
	s.setup.mu.RLock()
	defer s.setup.mu.RUnlock()
	return s.setup.Store
}

func (s *service) NewClient(model string, maxTokens int) *Client {
	p := s.Provider()
	return NewClient(p, model, maxTokens)
}

func (s *service) ListProviders() map[Name][]Info {
	st := s.Store()
	return GetProvidersWithStatus(st)
}
