package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// ModelCacheTTL is the time-to-live for cached models
	ModelCacheTTL = 24 * time.Hour
)

// ConnectionInfo stores connection information for a provider
type ConnectionInfo struct {
	AuthMethod  AuthMethod `json:"authMethod"`
	ConnectedAt time.Time  `json:"connectedAt"`
}

// ModelCache stores cached model information
type ModelCache struct {
	CachedAt time.Time   `json:"cachedAt"`
	Models   []ModelInfo `json:"models"`
}

// CurrentModelInfo stores the current model with its provider info
type CurrentModelInfo struct {
	ModelID    string     `json:"modelId"`
	Provider   Provider   `json:"provider"`
	AuthMethod AuthMethod `json:"authMethod"`
}

// StoreData is the persisted data structure
type StoreData struct {
	Connections    map[string]ConnectionInfo `json:"connections"`              // key: provider
	Models         map[string]ModelCache     `json:"models"`                   // key: provider:authMethod
	Current        *CurrentModelInfo         `json:"current"`                  // current model with provider info
	SearchProvider *string                   `json:"searchProvider,omitempty"` // search provider name (exa, serper, brave)
}

// Store manages provider configuration persistence
type Store struct {
	mu       sync.RWMutex
	path     string
	data     StoreData
}

// NewStore creates a new Store instance
func NewStore() (*Store, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Join(homeDir, ".gen")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	store := &Store{
		path: filepath.Join(configDir, "providers.json"),
		data: StoreData{
			Connections: make(map[string]ConnectionInfo),
			Models:      make(map[string]ModelCache),
		},
	}

	// Load existing data if available
	if err := store.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return store, nil
}

// load reads the store data from disk
func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, &s.data); err != nil {
		return err
	}

	// Initialize maps if nil
	s.ensureMapsInitialized()
	return nil
}

// ensureMapsInitialized ensures all map fields are non-nil
func (s *Store) ensureMapsInitialized() {
	if s.data.Connections == nil {
		s.data.Connections = make(map[string]ConnectionInfo)
	}
	if s.data.Models == nil {
		s.data.Models = make(map[string]ModelCache)
	}
}

// save writes the store data to disk
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

// Connect saves a connection for a provider
func (s *Store) Connect(provider Provider, authMethod AuthMethod) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Connections[string(provider)] = ConnectionInfo{
		AuthMethod:  authMethod,
		ConnectedAt: time.Now(),
	}

	return s.save()
}

// Disconnect removes a connection for a provider
func (s *Store) Disconnect(provider Provider) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data.Connections, string(provider))
	return s.save()
}

// IsConnected checks if a provider is connected with the specified auth method
func (s *Store) IsConnected(provider Provider, authMethod AuthMethod) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn, ok := s.data.Connections[string(provider)]
	if !ok {
		return false
	}
	return conn.AuthMethod == authMethod
}

// GetConnection returns the connection info for a provider
func (s *Store) GetConnection(provider Provider) (ConnectionInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conn, ok := s.data.Connections[string(provider)]
	return conn, ok
}

// GetConnections returns all connections
func (s *Store) GetConnections() map[string]ConnectionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]ConnectionInfo)
	for k, v := range s.data.Connections {
		result[k] = v
	}
	return result
}

// CacheModels saves model information for a provider
func (s *Store) CacheModels(provider Provider, authMethod AuthMethod, models []ModelInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Models[makeModelCacheKey(provider, authMethod)] = ModelCache{
		CachedAt: time.Now(),
		Models:   models,
	}

	return s.save()
}

// GetCachedModels returns cached models if they exist and are not expired
func (s *Store) GetCachedModels(provider Provider, authMethod AuthMethod) ([]ModelInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cache, ok := s.data.Models[makeModelCacheKey(provider, authMethod)]
	if !ok || time.Since(cache.CachedAt) > ModelCacheTTL {
		return nil, false
	}

	return cache.Models, true
}

// makeModelCacheKey creates a cache key for provider and auth method
func makeModelCacheKey(provider Provider, authMethod AuthMethod) string {
	return string(provider) + ":" + string(authMethod)
}

// GetAllCachedModels returns all cached models grouped by provider key
func (s *Store) GetAllCachedModels() map[string][]ModelInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]ModelInfo)
	for key, cache := range s.data.Models {
		// Skip expired caches
		if time.Since(cache.CachedAt) > ModelCacheTTL {
			continue
		}
		result[key] = cache.Models
	}
	return result
}

// SetCurrentModel sets the current model with provider info
func (s *Store) SetCurrentModel(modelID string, provider Provider, authMethod AuthMethod) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Current = &CurrentModelInfo{
		ModelID:    modelID,
		Provider:   provider,
		AuthMethod: authMethod,
	}
	return s.save()
}

// GetCurrentModel returns the current model info
func (s *Store) GetCurrentModel() *CurrentModelInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.data.Current
}

// ClearModelCache clears all cached models
func (s *Store) ClearModelCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Models = make(map[string]ModelCache)
	return s.save()
}

// GetSearchProvider returns the current search provider name
func (s *Store) GetSearchProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.SearchProvider == nil {
		return "" // Will use default (exa)
	}
	return *s.data.SearchProvider
}

// SetSearchProvider sets the search provider
func (s *Store) SetSearchProvider(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.SearchProvider = &name
	return s.save()
}

// ClearSearchProvider clears the search provider (use default)
func (s *Store) ClearSearchProvider() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.SearchProvider = nil
	return s.save()
}
