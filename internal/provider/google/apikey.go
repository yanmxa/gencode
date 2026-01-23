package google

import (
	"github.com/yanmxa/gencode/internal/provider"
)

// APIKeyMeta is the metadata for Google via API Key
var APIKeyMeta = provider.ProviderMeta{
	Provider:    provider.ProviderGoogle,
	AuthMethod:  provider.AuthAPIKey,
	EnvVars:     []string{"GOOGLE_API_KEY"},
	DisplayName: "Direct API",
}

// init registers the API Key provider
func init() {
	provider.Register(APIKeyMeta, NewAPIKeyClient)
}
