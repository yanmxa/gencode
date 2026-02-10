package openai

import (
	"context"

	"github.com/openai/openai-go/v3"

	"github.com/yanmxa/gencode/internal/provider"
)

// APIKeyMeta is the metadata for OpenAI via API Key
var APIKeyMeta = provider.ProviderMeta{
	Provider:    provider.ProviderOpenAI,
	AuthMethod:  provider.AuthAPIKey,
	EnvVars:     []string{"OPENAI_API_KEY"},
	DisplayName: "Direct API",
}

// NewAPIKeyClient creates a new OpenAI client using API Key authentication
func NewAPIKeyClient(ctx context.Context) (provider.LLMProvider, error) {
	client := openai.NewClient()
	return NewClient(client, "openai:api_key"), nil
}

// init registers the API Key provider
func init() {
	provider.Register(APIKeyMeta, NewAPIKeyClient)
}
