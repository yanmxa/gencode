package provider

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// registryEntry holds a provider's metadata and factory
type registryEntry struct {
	meta    ProviderMeta
	factory ProviderFactory
}

// Registry manages provider registration and discovery
type Registry struct {
	mu       sync.RWMutex
	entries  map[string]registryEntry // key: "provider:authMethod"
}

// globalRegistry is the default registry instance
var globalRegistry = &Registry{
	entries: make(map[string]registryEntry),
}

// Register registers a provider with its metadata and factory
func Register(meta ProviderMeta, factory ProviderFactory) {
	globalRegistry.Register(meta, factory)
}

// Register registers a provider with its metadata and factory
func (r *Registry) Register(meta ProviderMeta, factory ProviderFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries[meta.Key()] = registryEntry{
		meta:    meta,
		factory: factory,
	}
}

// GetProvider returns a provider instance for the given provider and auth method
func GetProvider(ctx context.Context, provider Provider, authMethod AuthMethod) (LLMProvider, error) {
	return globalRegistry.GetProvider(ctx, provider, authMethod)
}

// GetProvider returns a provider instance for the given provider and auth method
func (r *Registry) GetProvider(ctx context.Context, provider Provider, authMethod AuthMethod) (LLMProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.entries[makeProviderKey(provider, authMethod)]
	if !ok {
		return nil, fmt.Errorf("provider not registered: %s:%s", provider, authMethod)
	}

	return entry.factory(ctx)
}

// GetMeta returns the metadata for a specific provider configuration
func GetMeta(provider Provider, authMethod AuthMethod) (ProviderMeta, bool) {
	return globalRegistry.GetMeta(provider, authMethod)
}

// GetMeta returns the metadata for a specific provider configuration
func (r *Registry) GetMeta(provider Provider, authMethod AuthMethod) (ProviderMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.entries[makeProviderKey(provider, authMethod)]
	if !ok {
		return ProviderMeta{}, false
	}
	return entry.meta, true
}

// makeProviderKey creates a unique key for provider and auth method combination
func makeProviderKey(provider Provider, authMethod AuthMethod) string {
	return string(provider) + ":" + string(authMethod)
}

// IsReady checks if all required environment variables are set for a provider
func IsReady(meta ProviderMeta) bool {
	return globalRegistry.IsReady(meta)
}

// IsReady checks if all required environment variables are set for a provider
func (r *Registry) IsReady(meta ProviderMeta) bool {
	for _, envVar := range meta.EnvVars {
		if os.Getenv(envVar) == "" {
			return false
		}
	}
	return true
}

// GetAllMetas returns all registered provider metadata
func GetAllMetas() []ProviderMeta {
	return globalRegistry.GetAllMetas()
}

// GetAllMetas returns all registered provider metadata
func (r *Registry) GetAllMetas() []ProviderMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metas := make([]ProviderMeta, 0, len(r.entries))
	for _, entry := range r.entries {
		metas = append(metas, entry.meta)
	}
	return metas
}

// GetReadyProviders returns all providers that have their required env vars configured
func GetReadyProviders() []ProviderMeta {
	return globalRegistry.GetReadyProviders()
}

// GetReadyProviders returns all providers that have their required env vars configured
func (r *Registry) GetReadyProviders() []ProviderMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ready := make([]ProviderMeta, 0)
	for _, entry := range r.entries {
		if r.IsReady(entry.meta) {
			ready = append(ready, entry.meta)
		}
	}
	return ready
}

// ProviderStatus represents the connection status of a provider
type ProviderStatus string

const (
	StatusConnected     ProviderStatus = "connected"
	StatusAvailable     ProviderStatus = "available"
	StatusNotConfigured ProviderStatus = "not_configured"
)

// ProviderInfo contains provider metadata with its current status
type ProviderInfo struct {
	Meta   ProviderMeta
	Status ProviderStatus
}

// GetProvidersWithStatus returns all providers grouped by provider name with their status
func GetProvidersWithStatus(store *Store) map[Provider][]ProviderInfo {
	return globalRegistry.GetProvidersWithStatus(store)
}

// GetProvidersWithStatus returns all providers grouped by provider name with their status
func (r *Registry) GetProvidersWithStatus(store *Store) map[Provider][]ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[Provider][]ProviderInfo)

	for _, entry := range r.entries {
		var status ProviderStatus
		if store.IsConnected(entry.meta.Provider, entry.meta.AuthMethod) {
			status = StatusConnected
		} else if r.IsReady(entry.meta) {
			status = StatusAvailable
		} else {
			status = StatusNotConfigured
		}

		info := ProviderInfo{
			Meta:   entry.meta,
			Status: status,
		}
		result[entry.meta.Provider] = append(result[entry.meta.Provider], info)
	}

	return result
}
