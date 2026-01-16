# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**GenCode** (npm: `gencode-ai`) is an open-source AI assistant for the terminal. Extensible tools, customizable prompts, multi-provider support.

## Build & Run Commands

```bash
npm install          # Install dependencies
npm run build        # Compile TypeScript to dist/
npm run dev          # Watch mode compilation
npm start            # Run CLI directly via tsx
npm run example      # Run examples/basic.ts
```

## Architecture

### Provider Abstraction Layer (`src/providers/`)

Unified `LLMProvider` interface abstracts API differences:
- `complete()` - Non-streaming completion
- `stream()` - Streaming completion (AsyncGenerator)

Each provider (OpenAI, Anthropic, Gemini) translates the unified message format to its native API format and back. The `createProvider()` factory instantiates providers by name.

### Tool System (`src/tools/`)

Tools are defined with Zod schemas for input validation:
```typescript
interface Tool<TInput> {
  name: string;
  description: string;
  parameters: z.ZodSchema<TInput>;
  execute(input: TInput, context: ToolContext): Promise<ToolResult>;
}
```

`ToolRegistry` manages tools and converts Zod schemas to JSON Schema for LLM consumption via `zodToJsonSchema()`.

### Agent Loop (`src/agent/agent.ts`)

The `Agent` class implements the core conversation loop:
1. User message → LLM with tools
2. If `stopReason === 'tool_use'`: execute tools, append results, loop back
3. If `stopReason !== 'tool_use'`: done

Events are yielded as `AgentEvent` (text, tool_start, tool_result, done, error).

### Session Management (`src/session/`)

Sessions persist conversation history to `~/.gencode/sessions/` as JSON files. Supports resume, fork, list, and delete operations.

## Configuration

Provider/model selection priority:
1. `GENCODE_PROVIDER` / `GENCODE_MODEL` env vars
2. Auto-detect from available API keys (ANTHROPIC_API_KEY → OPENAI_API_KEY → GOOGLE_API_KEY)
3. Default: Gemini

Proxy: Set `HTTP_PROXY` or `HTTPS_PROXY` for network proxy support.

## Key Patterns

- All file paths in tools should be resolved relative to `ToolContext.cwd`
- Tool input validation uses Zod; errors returned as `ToolResult.error`
- Provider implementations handle message format conversion internally
- CLI commands start with `/` (e.g., `/sessions`, `/resume`, `/help`)

## Reference Projects

Similar projects for learning and reference:

| Project | Path | Description |
|---------|------|-------------|
| OpenCode | `/Users/myan/Workspace/opencode` | Go-based AI coding assistant with TUI, multi-provider support |
| System Prompts Collection | `/Users/myan/Workspace/ideas/system-prompts-and-models-of-ai-tools` | Collection of system prompts from various AI tools (Claude Code, Cursor, etc.) |
| Learn Claude Code | `/Users/myan/Workspace/ideas/learn-claude-code` | Educational resources for understanding Claude Code internals |

### Key Learnings from References

- **OpenCode**: Go implementation with Ink-based TUI, LSP integration, conversation sessions
- **System Prompts**: Study prompt engineering patterns used by production AI tools
- **Learn Claude Code**: Understand Claude Code's tool system, agent loop, and UX patterns
