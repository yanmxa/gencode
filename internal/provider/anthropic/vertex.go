package anthropic

import (
	"context"
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

	return NewClient(client, "anthropic:vertex"), nil
}

// init registers the Vertex AI provider
func init() {
	provider.Register(VertexMeta, NewVertexClient)
}
