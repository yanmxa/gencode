# Providers & Models

GenCode supports multiple LLM providers. Use the `/provider` command to manage provider connections.

## Supported Providers

### Anthropic (Claude)

Claude models from Anthropic with multiple connection options:

| Connection Method | Environment Variables | Description |
|-------------------|----------------------|-------------|
| API Key | `ANTHROPIC_API_KEY` | Direct API access |
| Google Vertex AI | `ANTHROPIC_VERTEX_PROJECT_ID` or `GOOGLE_CLOUD_PROJECT` | Claude via GCP |
| Amazon Bedrock | `AWS_ACCESS_KEY_ID` or `AWS_PROFILE` | Claude via AWS (coming soon) |

### OpenAI

GPT models from OpenAI:

| Connection Method | Environment Variables | Description |
|-------------------|----------------------|-------------|
| API Key | `OPENAI_API_KEY` | Direct API access |

### Google Gemini

Gemini models from Google:

| Connection Method | Environment Variables | Description |
|-------------------|----------------------|-------------|
| API Key | `GOOGLE_API_KEY` or `GEMINI_API_KEY` | Direct API access |

## Commands

### `/provider` - Provider Management

Opens the provider management interface where you can:

- View connected and available providers
- Connect to new providers
- Refresh model lists for connected providers
- Remove provider connections

**Keyboard shortcuts:**
- `↑↓` - Navigate between providers
- `Enter` - Connect (available) or Refresh (connected)
- `r` - Remove a connected provider
- `Esc` - Exit

### `/model` - Model Selection

Opens the model selector to switch between models from all connected providers:

- Models are grouped by provider
- Shows cached models (fast, no API call)
- Supports fuzzy search filtering

**Keyboard shortcuts:**
- `↑↓` - Navigate between models
- `Enter` - Select model
- `Esc` - Cancel

## Google Vertex AI Setup

To use Claude models via Google Vertex AI:

### 1. Set Environment Variables

```bash
# Required: Your GCP project ID
export ANTHROPIC_VERTEX_PROJECT_ID="your-project-id"

# Optional: Region (defaults to us-east5)
export ANTHROPIC_VERTEX_REGION="us-east5"
```

Alternative variables also supported:
- `GOOGLE_CLOUD_PROJECT` - GCP project ID
- `CLOUD_ML_REGION` - GCP region

### 2. Authenticate with Google Cloud

```bash
# Login and set up Application Default Credentials
gcloud auth application-default login

# Verify authentication
gcloud auth application-default print-access-token
```

### 3. Enable Vertex AI API

Ensure the Vertex AI API is enabled in your GCP project:

```bash
gcloud services enable aiplatform.googleapis.com
```

### 4. Connect in GenCode

```
/provider
# Select "Anthropic"
# Choose "Google Vertex AI" connection method
# Press Enter to connect
```

## Configuration Storage

Provider connections and cached models are stored in:

```
~/.gencode/
├── settings.json      # Current model and provider settings
└── providers.json     # Provider connections and model cache
```

### providers.json Structure

```json
{
  "connections": {
    "anthropic": {
      "method": "vertex",
      "connectedAt": "2025-01-15T10:00:00Z"
    }
  },
  "models": {
    "anthropic": {
      "cachedAt": "2025-01-15T10:00:00Z",
      "list": [
        { "id": "claude-sonnet-4-5@20250929", "name": "Claude Sonnet 4.5" }
      ]
    }
  }
}
```

## Troubleshooting

### "Not configured" status

If a provider shows "(not configured)", set the required environment variables and restart GenCode.

### Model list is empty

Use `/provider` and press Enter on a connected provider to refresh the model cache.

### Vertex AI authentication errors

1. Verify you're logged in: `gcloud auth application-default print-access-token`
2. Check project ID: `echo $ANTHROPIC_VERTEX_PROJECT_ID`
3. Ensure Vertex AI API is enabled in your project
