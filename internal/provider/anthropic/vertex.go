package anthropic

import (
	"context"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/vertex"

	"github.com/yanmxa/gencode/internal/provider"
)

// VertexMeta is the metadata for Anthropic via Vertex AI
var VertexMeta = provider.ProviderMeta{
	Provider:    provider.ProviderAnthropic,
	AuthMethod:  provider.AuthVertex,
	EnvVars:     []string{"CLOUD_ML_REGION", "ANTHROPIC_VERTEX_PROJECT_ID"},
	DisplayName: "Vertex AI",
}

// vertexModels is the static list of Claude models available on Vertex AI.
//
// Source: https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude
// Last updated: 2025-02-07
//
// Note: Vertex AI does not provide a Models API, so we use a static list.
// TODO: Switch to dynamic fetching once upstream issue is resolved:
//       https://github.com/anthropics/anthropic-sdk-go/issues/270
// TODO: Update model list from:
//       https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude
var vertexModels = []provider.ModelInfo{
	{
		ID:               "claude-opus-4-6",
		Name:             "Claude Opus 4.6",
		DisplayName:      "Claude Opus 4.6 (Most Intelligent)",
		InputTokenLimit:  1000000,
		OutputTokenLimit: 128000,
	},
	{
		ID:               "claude-opus-4-5",
		Name:             "Claude Opus 4.5",
		DisplayName:      "Claude Opus 4.5 (Most Capable)",
		InputTokenLimit:  200000,
		OutputTokenLimit: 64000,
	},
	{
		ID:               "claude-sonnet-4-5",
		Name:             "Claude Sonnet 4.5",
		DisplayName:      "Claude Sonnet 4.5 (Balanced)",
		InputTokenLimit:  1000000,
		OutputTokenLimit: 64000,
	},
	{
		ID:               "claude-opus-4-1",
		Name:             "Claude Opus 4.1",
		DisplayName:      "Claude Opus 4.1",
		InputTokenLimit:  200000,
		OutputTokenLimit: 32000,
	},
	{
		ID:               "claude-haiku-4-5",
		Name:             "Claude Haiku 4.5",
		DisplayName:      "Claude Haiku 4.5 (Fast)",
		InputTokenLimit:  200000,
		OutputTokenLimit: 64000,
	},
	{
		ID:               "claude-opus-4",
		Name:             "Claude Opus 4",
		DisplayName:      "Claude Opus 4",
		InputTokenLimit:  200000,
		OutputTokenLimit: 32000,
	},
	{
		ID:               "claude-sonnet-4",
		Name:             "Claude Sonnet 4",
		DisplayName:      "Claude Sonnet 4",
		InputTokenLimit:  1000000,
		OutputTokenLimit: 64000,
	},
	{
		ID:               "claude-3-5-haiku",
		Name:             "Claude 3.5 Haiku",
		DisplayName:      "Claude 3.5 Haiku",
		InputTokenLimit:  200000,
		OutputTokenLimit: 8192,
	},
	{
		ID:               "claude-3-haiku",
		Name:             "Claude 3 Haiku",
		DisplayName:      "Claude 3 Haiku",
		InputTokenLimit:  200000,
		OutputTokenLimit: 8000,
	},
}

// VertexClient wraps the standard Client with Vertex-specific behavior
type VertexClient struct {
	*Client
}

// ListModels tries the Anthropic Models API first, falling back to a static
// list with a warning error when the API is unavailable (e.g. 404 on Vertex AI).
func (c *VertexClient) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if len(c.cachedModels) > 0 {
		return c.cachedModels, nil
	}

	// Try dynamic fetching first
	models, err := c.fetchModels(ctx)
	if err == nil {
		c.cachedModels = models
		return models, nil
	}

	// Fall back to static model list with warning
	c.cachedModels = vertexModels
	return vertexModels, fmt.Errorf("using static models")
}

// NewVertexClient creates a new Anthropic client using Vertex AI authentication
func NewVertexClient(ctx context.Context) (provider.LLMProvider, error) {
	region := os.Getenv("CLOUD_ML_REGION")
	if region == "" {
		region = "us-east5"
	}
	projectID := os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID")

	client := anthropic.NewClient(
		vertex.WithGoogleAuth(ctx, region, projectID),
	)

	baseClient := NewClient(client, "anthropic:vertex")
	return &VertexClient{Client: baseClient}, nil
}

// Ensure VertexClient implements LLMProvider
var _ provider.LLMProvider = (*VertexClient)(nil)

// init registers the Vertex AI provider
func init() {
	provider.Register(VertexMeta, NewVertexClient)
}
