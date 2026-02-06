# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

**GenCode** is an open-source AI coding assistant for the terminal, written in Go.

## Build & Run

```bash
go build -o gen ./cmd/gen    # Build binary
./gen                         # Run interactive mode
./gen "your message"          # Non-interactive mode
```

## Architecture

```
main.go                  # CLI entry point
internal/
├── provider/            # LLM provider abstraction
│   ├── anthropic/       # Claude (API key + Vertex AI)
│   ├── openai/          # GPT models
│   ├── google/          # Gemini models
│   ├── search/          # Web search providers (Brave, Exa, Serper)
│   ├── registry.go      # Provider registry
│   ├── store.go         # Connection persistence (~/.gen/store.json)
│   └── types.go         # Common types (Message, ToolCall, etc.)
├── tool/                # Tool implementations
│   ├── read.go          # Read file contents
│   ├── write.go         # Write files (with permission)
│   ├── edit.go          # Edit files (with diff preview)
│   ├── bash.go          # Execute shell commands
│   ├── glob.go          # Find files by pattern
│   ├── grep.go          # Search file contents
│   ├── webfetch.go      # Fetch web pages
│   ├── websearch.go     # Web search
│   ├── permission/      # Permission system (diff generation)
│   ├── ui/              # Tool output rendering
│   ├── registry.go      # Tool registry
│   ├── schema.go        # JSON Schema generation
│   └── types.go         # Tool interfaces
├── tui/                 # Terminal UI (Bubble Tea)
│   ├── app.go           # Main TUI model and update loop
│   ├── commands.go      # Slash command handling
│   ├── permissionprompt.go  # Permission UI
│   ├── diffpreview.go   # Diff rendering
│   ├── bashpreview.go   # Bash command preview
│   ├── selector.go      # Provider/model selection UI
│   └── suggestions.go   # Command autocomplete
├── config/              # Settings and permissions
├── system/              # System prompt generation
├── log/                 # Debug logging
└── mcp/                 # MCP (Model Context Protocol) support
    ├── transport/       # STDIO, HTTP, SSE transports
    ├── client.go        # MCP client implementation
    ├── config.go        # Configuration loading
    └── registry.go      # Server management
```

## Key Interfaces

### LLMProvider (`internal/provider/types.go`)

```go
type LLMProvider interface {
    Name() string
    Stream(ctx context.Context, opts CompletionOptions) <-chan StreamChunk
    Complete(ctx context.Context, opts CompletionOptions) (CompletionResponse, error)
    ListModels(ctx context.Context) ([]ModelInfo, error)
}
```

### Tool (`internal/tool/types.go`)

```go
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]any  // JSON Schema for parameters
    Execute(ctx context.Context, params map[string]any, cwd string) (ToolOutput, error)
}

type PermissionAwareTool interface {
    Tool
    RequiresPermission() bool
    PreparePermission(ctx context.Context, params map[string]any, cwd string) (*permission.PermissionRequest, error)
    ExecuteApproved(ctx context.Context, params map[string]any, cwd string) ToolOutput
}
```

## Key Patterns

- Provider/model state persisted to `~/.gen/store.json`
- Tools return `ToolOutput` with structured content for LLM
- Permission-aware tools (Edit, Write, Bash) show preview before execution
- TUI uses Bubble Tea with viewport, textarea, and spinner components
- Markdown rendered via glamour

## Token Limits

Token limits track context window usage and prevent exceeding model limits.

| Command | Action |
|---------|--------|
| `/tokenlimit` | Show limits or auto-fetch if not available |
| `/tokenlimit <input> <output>` | Set custom limits |

**Read Priority:** `tokenLimits` (custom) → Model Cache (API) → 0

**Usage Indicator:** Shows `⚡ 180K/200K (90%)` when >= 80% of limit.

See [docs/token-limits.md](docs/token-limits.md) for detailed documentation.

## MCP (Model Context Protocol) Support

GenCode supports MCP servers for extending functionality with external tools.

### Configuration

MCP servers are configured in JSON files:

| Scope | Path | Git | Purpose |
|-------|------|-----|---------|
| user | `~/.gen/mcp.json` | N/A | Global, cross-project |
| project | `./.gen/mcp.json` | Commit | Team shared |
| local | `./.gen/mcp.local.json` | Ignore | Personal, not shared |

**Priority:** Local overrides Project overrides User.

### CLI Commands

```bash
# Add servers
gen mcp add <name> -- <command> [args...]           # STDIO
gen mcp add --transport http <name> <url>           # HTTP
gen mcp add --transport sse <name> <url>            # SSE
gen mcp add-json <name> '<json>'                    # From JSON

# Manage
gen mcp list                    # List all servers
gen mcp get <name>              # Show server details
gen mcp remove <name>           # Remove server
```

### TUI Commands

- `/mcp` - Show server status and tools
- `/mcp connect <name>` - Connect to server
- `/mcp disconnect <name>` - Disconnect from server

### Tool Naming

MCP tools are exposed with the pattern: `mcp__<server>__<tool>`

Example: `mcp__filesystem__read_file`

## Debug Logging

Set `GEN_DEBUG=1` to enable debug logging to `~/.gen/debug.log`.
