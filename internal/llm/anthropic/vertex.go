package anthropic

import (
	"context"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/vertex"

	"github.com/yanmxa/gencode/internal/llm"
)

// VertexMeta is the metadata for Anthropic via Vertex AI
var VertexMeta = llm.ProviderMeta{
	Provider:    llm.ProviderAnthropic,
	AuthMethod:  llm.AuthVertex,
	EnvVars:     []string{"CLOUD_ML_REGION", "ANTHROPIC_VERTEX_PROJECT_ID"},
	DisplayName: "Vertex AI",
}

// vertexModels is the static list of Claude models available on Vertex AI.
//
// Source: https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude
//
//	https://platform.claude.com/docs/en/docs/about-claude/models
//
// Last updated: 2026-02-23
//
// Note: Vertex AI does not provide a Models API, so we use a static list.
// TODO: Switch to dynamic fetching once upstream issue is resolved:
//
//	https://github.com/anthropics/anthropic-sdk-go/issues/270
var vertexModels = []llm.ModelInfo{
	// Current generation
	{
		ID:               "claude-opus-4-6",
		Name:             "Claude Opus 4.6",
		DisplayName:      "Claude Opus 4.6 (Most Intelligent)",
		InputTokenLimit:  1000000, // 1M (preview)
		OutputTokenLimit: 128000,
	},
	{
		ID:               "claude-sonnet-4-6",
		Name:             "Claude Sonnet 4.6",
		DisplayName:      "Claude Sonnet 4.6 (Speed & Intelligence)",
		InputTokenLimit:  1000000, // 1M (preview)
		OutputTokenLimit: 128000,
	},
	{
		ID:               "claude-haiku-4-5",
		Name:             "Claude Haiku 4.5",
		DisplayName:      "Claude Haiku 4.5 (Fast)",
		InputTokenLimit:  200000,
		OutputTokenLimit: 64000,
	},
	// Legacy models
	{
		ID:               "claude-sonnet-4-5",
		Name:             "Claude Sonnet 4.5",
		DisplayName:      "Claude Sonnet 4.5",
		InputTokenLimit:  200000, // 1M (preview)
		OutputTokenLimit: 64000,
	},
	{
		ID:               "claude-opus-4-5",
		Name:             "Claude Opus 4.5",
		DisplayName:      "Claude Opus 4.5",
		InputTokenLimit:  200000,
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
		ID:               "claude-sonnet-4",
		Name:             "Claude Sonnet 4",
		DisplayName:      "Claude Sonnet 4",
		InputTokenLimit:  200000, // 1M (preview)
		OutputTokenLimit: 64000,
	},
	{
		ID:               "claude-opus-4",
		Name:             "Claude Opus 4",
		DisplayName:      "Claude Opus 4",
		InputTokenLimit:  200000,
		OutputTokenLimit: 32000,
	},
}

// VertexClient wraps the standard Client with Vertex-specific behavior
type VertexClient struct {
	*Client
}

// ListModels tries the Anthropic Models API first, falling back to a static
// list with a warning error when the API is unavailable (e.g. 404 on Vertex AI).
// A failed fetch does not permanently cache the fallback — subsequent calls retry.
func (c *VertexClient) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	c.modelsMu.Lock()
	defer c.modelsMu.Unlock()

	if c.cachedModels != nil {
		return c.cachedModels, nil
	}

	models, err := c.fetchModels(ctx)
	if err == nil {
		c.cachedModels = models
		return c.cachedModels, nil
	}
	// Return static fallback but don't cache it so we retry next time
	return vertexModels, nil
}

// NewVertexClient creates a new Anthropic client using Vertex AI authentication
func NewVertexClient(ctx context.Context) (llm.LLMProvider, error) {
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
var _ llm.LLMProvider = (*VertexClient)(nil)

// init registers the Vertex AI provider
func init() {
	llm.Register(VertexMeta, NewVertexClient)
}
