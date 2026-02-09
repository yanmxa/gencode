// Package provider provides interfaces and implementations for interacting with LLM providers.
package provider

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/provider/anthropic"
	"github.com/yanmxa/gencode/internal/provider/google"
	"github.com/yanmxa/gencode/internal/provider/openai"
	"github.com/yanmxa/gencode/internal/provider/moonshot"
)

// LLMProvider is an interface for interacting with different LLM providers.
type LLMProvider interface {
	Name() string
	Stream(ctx context.Context, opts CompletionOptions) <-chan StreamChunk
	ListModels(ctx context.Context) ([]ModelInfo, error)
}

// NewProvider creates a new LLMProvider based on the provider name.
func NewProvider(ctx context.Context, name string) (LLMProvider, error) {
	switch name {
	case "anthropic:api_key":
		apiKey, err := anthropic.NewAPIKeyClient(ctx)
		if err != nil {
			return nil, err
		}
		return apiKey, nil
	case "google:api_key":
		apiKey, err := google.NewAPIKeyClient(ctx)
		if err != nil {
			return nil, err
		}
		return apiKey, nil
	case "openai:api_key":
		apiKey, err := openai.NewAPIKeyClient(ctx)
		if err != nil {
			return nil, err
		}
		return apiKey, nil
	case "moonshot:api_key":
		apiKey, err := moonshot.NewAPIKeyClient(ctx)
		if err != nil {
			return nil, err
		}
		return apiKey, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
