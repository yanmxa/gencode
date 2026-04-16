package google

import (
	"github.com/yanmxa/gencode/internal/llm"
)

// APIKeyMeta is the metadata for Google via API Key
var APIKeyMeta = llm.Meta{
	Provider:    llm.Google,
	AuthMethod:  llm.AuthAPIKey,
	EnvVars:     []string{"GOOGLE_API_KEY"},
	DisplayName: "Direct API",
}

// init registers the API Key provider
func init() {
	llm.Register(APIKeyMeta, NewAPIKeyClient)
}
