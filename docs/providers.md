# Providers

GenCode supports multiple LLM providers and Search providers. Use the `/provider` command to manage all provider connections.

## LLM Providers

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

### Google

Google Generative AI (Gemini models):

| Connection Method | Environment Variables | Description |
|-------------------|----------------------|-------------|
| API Key | `GOOGLE_API_KEY` or `GEMINI_API_KEY` | Direct API access |

## Search Providers

GenCode includes a WebSearch tool that can search the web for current information. Multiple search providers are supported:

### Exa AI (Default)

AI-native search engine using Exa's public MCP endpoint. Works out of the box without any configuration.

| Connection Method | Environment Variables | Description |
|-------------------|----------------------|-------------|
| Public MCP | (none required) | Uses public `mcp.exa.ai` endpoint |

**Features:**
- No configuration required - works immediately
- AI-optimized search results
- Live crawl support for fresh content

**Note:** Uses Exa's public MCP endpoint. For heavy usage, consider using Serper or Brave with your own API key.

### Serper.dev

Google Search results via Serper API.

| Connection Method | Environment Variables | Description |
|-------------------|----------------------|-------------|
| API Key | `SERPER_API_KEY` | Google Search via Serper |

**Features:**
- 2,500 free queries (no credit card required)
- Google Search quality results
- Fast response times

**Get API Key:** https://serper.dev

### Brave Search

Privacy-focused search from Brave.

| Connection Method | Environment Variables | Description |
|-------------------|----------------------|-------------|
| API Key | `BRAVE_API_KEY` | Privacy-focused search |

**Features:**
- 2,000 free queries per month
- Privacy-first approach
- Independent search index

**Get API Key:** https://brave.com/search/api

### Search Provider Priority

When multiple search providers are configured, the priority is:

1. **Configured in `/provider`** - Explicitly selected provider
2. **Environment variables** - `SERPER_API_KEY` > `BRAVE_API_KEY`
3. **Default** - Exa AI (no configuration needed)

## Commands

### `/provider` - Provider Management

Opens the provider management interface with two tabs:

**[L] LLM Providers Tab:**
- View connected and available LLM providers
- Connect to new providers
- Refresh model lists for connected providers
- Remove provider connections

**[S] Search Providers Tab:**
- View available search providers
- Select which search provider to use for WebSearch
- See configuration status (configured/not configured)

**Keyboard shortcuts:**
- `Tab` or `L`/`S` - Switch between tabs
- `↑↓` - Navigate between providers
- `Enter` - Connect/Select provider
- `r` - Remove a connected LLM provider
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
# Required: Enable Vertex AI
export CLAUDE_CODE_USE_VERTEX=1

# Required: Your GCP project ID
export ANTHROPIC_VERTEX_PROJECT_ID="your-project-id"

# Required: Region (use 'global' or specific region)
export CLOUD_ML_REGION=global
```

**Environment Variable Details:**

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CLAUDE_CODE_USE_VERTEX` | Yes | - | Set to `1` to enable Vertex AI |
| `ANTHROPIC_VERTEX_PROJECT_ID` | Yes | - | Your GCP project ID (also accepts `GCLOUD_PROJECT` or `GOOGLE_CLOUD_PROJECT`) |
| `CLOUD_ML_REGION` | Yes | `us-east5` | Region (use `global` or specific region like `us-east5`) |

**Documentation:** https://code.claude.com/docs/en/google-vertex-ai

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

## Architecture

GenCode uses a two-layer provider architecture:

- **Layer 1: Provider** (Semantic layer) - `anthropic` | `openai` | `google`
- **Layer 2: AuthMethod** (Implementation layer) - `api_key` | `vertex` | `bedrock` | `azure`

Each provider can support multiple authentication methods. For example, Anthropic supports:
- `api_key` - Direct API access
- `vertex` - Google Vertex AI
- `bedrock` - Amazon Bedrock (coming soon)

## Configuration Storage

Provider connections and cached models are stored in:

```
~/.gen/
├── settings.json      # Current model and provider settings
└── providers.json     # Provider connections and model cache
```

### providers.json Structure

```json
{
  "connections": {
    "anthropic": {
      "authMethod": "vertex",
      "method": "Google Vertex AI",
      "connectedAt": "2025-01-15T10:00:00Z"
    },
    "openai": {
      "authMethod": "api_key",
      "method": "Direct API",
      "connectedAt": "2025-01-15T09:00:00Z"
    }
  },
  "models": {
    "anthropic:vertex": {
      "cachedAt": "2025-01-15T10:00:00Z",
      "list": [
        { "id": "claude-3-5-sonnet@20241022", "name": "Claude 3.5 Sonnet" }
      ]
    },
    "anthropic:api_key": {
      "cachedAt": "2025-01-15T11:00:00Z",
      "list": [
        { "id": "claude-sonnet-4-5@20250929", "name": "Claude Sonnet 4.5" }
      ]
    },
    "openai:api_key": {
      "cachedAt": "2025-01-15T09:00:00Z",
      "list": [
        { "id": "gpt-4", "name": "GPT-4" }
      ]
    }
  },
  "searchProvider": "exa"
}
```

**Key points:**
- `connections` stores the active connection for each provider
  - `authMethod` - Authentication method being used
  - `method` - Display name (optional, legacy field)
- `models` uses `"provider:authMethod"` as the key
  - Supports multiple auth methods for the same provider
  - Each auth method has its own cached model list
- `searchProvider` - Selected search provider (`exa`, `serper`, `brave`)

### Migration from Old Format

If you're upgrading from an older version, run the migration script to update your configuration:

```bash
npm run migrate
```

The script will:
1. Convert old format models keys (e.g., `"anthropic"`) to new format (e.g., `"anthropic:vertex"`)
2. Add `authMethod` field to connections
3. Create a backup of your old configuration

**Old format:**
```json
{
  "models": {
    "anthropic": {
      "provider": "anthropic",
      "authMethod": "vertex",
      "cachedAt": "...",
      "list": [...]
    }
  }
}
```

**New format:**
```json
{
  "models": {
    "anthropic:vertex": {
      "cachedAt": "...",
      "list": [...]
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
