package search

import (
	"context"
	"time"
)

// ProviderName identifies a search provider
type ProviderName string

const (
	ProviderExa    ProviderName = "exa"
	ProviderSerper ProviderName = "serper"
	ProviderBrave  ProviderName = "brave"
)

// SearchResult represents a single search result
type SearchResult struct {
	Title   string
	URL     string
	Snippet string
}

// SearchOptions configures search behavior
type SearchOptions struct {
	NumResults     int
	AllowedDomains []string
	BlockedDomains []string
	Timeout        time.Duration
}

// DefaultOptions returns default search options
func DefaultOptions() SearchOptions {
	return SearchOptions{
		NumResults: 10,
		Timeout:    30 * time.Second,
	}
}

// truncateSnippet truncates a snippet to maxLength characters
func truncateSnippet(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}

// getTimeout returns the timeout or default if not set
func getTimeout(opts SearchOptions) time.Duration {
	if opts.Timeout <= 0 {
		return 30 * time.Second
	}
	return opts.Timeout
}

// Provider is the interface for search providers
type Provider interface {
	// Name returns the provider name
	Name() ProviderName

	// DisplayName returns the human-readable name
	DisplayName() string

	// RequiresAPIKey returns true if an API key is needed
	RequiresAPIKey() bool

	// EnvVars returns the environment variable names for credentials
	EnvVars() []string

	// IsAvailable checks if the provider is configured and ready
	IsAvailable() bool

	// Search performs a web search
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
}

// ProviderMeta contains metadata about a search provider
type ProviderMeta struct {
	Name           ProviderName
	DisplayName    string
	RequiresAPIKey bool
	EnvVars        []string
}

// AllProviders returns metadata for all search providers
func AllProviders() []ProviderMeta {
	return []ProviderMeta{
		{
			Name:           ProviderExa,
			DisplayName:    "Exa AI",
			RequiresAPIKey: false,
			EnvVars:        []string{}, // No API key required
		},
		{
			Name:           ProviderSerper,
			DisplayName:    "Serper (Google)",
			RequiresAPIKey: true,
			EnvVars:        []string{"SERPER_API_KEY"},
		},
		{
			Name:           ProviderBrave,
			DisplayName:    "Brave Search",
			RequiresAPIKey: true,
			EnvVars:        []string{"BRAVE_API_KEY"},
		},
	}
}
