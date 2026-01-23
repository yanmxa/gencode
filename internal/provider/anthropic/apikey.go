package anthropic

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/myan/gencode/internal/provider"
)

// APIKeyMeta is the metadata for Anthropic via API Key
var APIKeyMeta = provider.ProviderMeta{
	Provider:    provider.ProviderAnthropic,
	AuthMethod:  provider.AuthAPIKey,
	EnvVars:     []string{"ANTHROPIC_API_KEY"},
	DisplayName: "Direct API",
}

// NewAPIKeyClient creates a new Anthropic client using API Key authentication
func NewAPIKeyClient(ctx context.Context) (provider.LLMProvider, error) {
	client := anthropic.NewClient()
	return NewClient(client, "anthropic:api_key"), nil
}

// init registers the API Key provider
func init() {
	provider.Register(APIKeyMeta, NewAPIKeyClient)
}
