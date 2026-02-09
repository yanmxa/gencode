package moonshot

import (
	"context"
	"os"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/yanmxa/gencode/internal/provider"
)

// APIKeyMeta is the metadata for Moonshot via API Key
var APIKeyMeta = provider.ProviderMeta{
	Provider:    provider.ProviderMoonshot,
	AuthMethod:  provider.AuthAPIKey,
	EnvVars:     []string{"MOONSHOT_API_KEY"},
	DisplayName: "Direct API",
}

// NewAPIKeyClient creates a new Moonshot client using API Key authentication.
// The Moonshot API is OpenAI-compatible, so we use the OpenAI SDK with a custom base URL.
func NewAPIKeyClient(ctx context.Context) (provider.LLMProvider, error) {
	baseURL := os.Getenv("MOONSHOT_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.moonshot.cn/v1"
	}

	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("MOONSHOT_API_KEY")),
		option.WithBaseURL(baseURL),
	)
	return NewClient(client, "moonshot:api_key"), nil
}

// init registers the API Key provider
func init() {
	provider.Register(APIKeyMeta, NewAPIKeyClient)
}
