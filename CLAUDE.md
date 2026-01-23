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
└── log/                 # Debug logging
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

## Debug Logging

Set `GEN_DEBUG=1` to enable debug logging to `~/.gen/debug.log`.
