# CLAUDE.md

Project guidance for Claude Code when working with this repository.

## Build

```bash
make build    # Output: bin/gen
make install  # Install to /usr/local/bin/gen
make clean    # Remove bin/ directory
make release  # Build all platform binaries in bin/
```

Default binary location: `bin/gen`

## Project Structure

- `cmd/gen/` - CLI entrypoint
- `internal/` - Core packages
  - `provider/` - LLM providers (Anthropic, Google, OpenAI)
  - `tui/` - Terminal UI (Bubble Tea)
  - `agent/` - Subagent execution
  - `tool/` - Tool implementations
  - `config/` - Settings and configuration
  - `mcp/` - Model Context Protocol support
  - `hooks/` - Event hooks
  - `session/` - Session management

## Vertex AI Claude Models

Source: https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude

Model definitions in `internal/provider/anthropic/vertex.go`.

## Testing

```bash
go test ./...
```

## Git

- Sign-off commits: `git commit -s`
- Don't use `git add .`
