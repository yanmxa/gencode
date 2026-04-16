package moonshot

import (
	"context"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/yanmxa/gencode/internal/llm"
)

// APIKeyMeta is the metadata for Moonshot via API Key
var APIKeyMeta = llm.ProviderMeta{
	Provider:    llm.ProviderMoonshot,
	AuthMethod:  llm.AuthAPIKey,
	EnvVars:     []string{"MOONSHOT_API_KEY"},
	DisplayName: "Direct API",
}

// NewAPIKeyClient creates a new Moonshot client using API Key authentication.
// The Moonshot API is OpenAI-compatible, so we use the OpenAI SDK with a custom base URL.
func NewAPIKeyClient(ctx context.Context) (llm.LLMProvider, error) {
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
	llm.Register(APIKeyMeta, NewAPIKeyClient)
}
