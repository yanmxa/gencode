package anthropic

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/yanmxa/gencode/internal/llm"
)

// APIKeyMeta is the metadata for Anthropic via API Key
var APIKeyMeta = llm.ProviderMeta{
	Provider:    llm.ProviderAnthropic,
	AuthMethod:  llm.AuthAPIKey,
	EnvVars:     []string{"ANTHROPIC_API_KEY"},
	DisplayName: "Direct API",
}

// NewAPIKeyClient creates a new Anthropic client using API Key authentication
func NewAPIKeyClient(ctx context.Context) (llm.LLMProvider, error) {
	client := anthropic.NewClient()
	return NewClient(client, "anthropic:api_key"), nil
}

// init registers the API Key provider
func init() {
	llm.Register(APIKeyMeta, NewAPIKeyClient)
}
