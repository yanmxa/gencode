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

Sessions persist conversation history to `~/.gen/sessions/` as JSON files. Supports resume, fork, list, and delete operations.

### Markdown Rendering (`src/cli/components/markdown.ts`)

GenCode uses a custom terminal renderer for full GFM (GitHub Flavored Markdown) support:

**Core features:**
- **Tables**: Unicode table rendering via cli-table3
  - CJK character width calculation (via string-width) for proper alignment
  - Automatic word wrapping for long content
  - Styled headers (cyan + bold) and dim borders matching code block theme
- **Code blocks**: Syntax highlighting with cyan coloring and 4-space indentation
- **Lists, quotes, links**: Full support with inline formatting (bold, italic, code, strikethrough)

**Key dependencies:**
- `marked@17.0.1` - Markdown parser
- `cli-table3@0.6.5` - Table rendering engine
- `string-width@8.1.0` - CJK character width support (ESM-compatible)
- `chalk@5.6.2` - ANSI color styling

**Example table output:**
```
┌──────────┬─────────────┬─────────────────────┐
│ Priority │ From        │ Subject             │
├──────────┼─────────────┼─────────────────────┤
│ High     │ John Doe    │ Meeting Request     │
│ Medium   │ Jane Smith  │ PR Review           │
└──────────┴─────────────┴─────────────────────┘
```

## Configuration

Provider/model selection priority:
1. `GEN_PROVIDER` / `GEN_MODEL` env vars
2. Auto-detect from available API keys (ANTHROPIC_API_KEY → OPENAI_API_KEY → GOOGLE_API_KEY)
3. Default: Gemini

Proxy: Set `HTTP_PROXY` or `HTTPS_PROXY` for network proxy support.

## Commands System (`src/ext/commands/`)

GenCode supports custom slash commands via markdown templates (compatible with Claude Code):

**Discovery locations** (priority: highest to lowest):
1. `./.gen/commands/` - Project-level GenCode commands
2. `./.claude/commands/` - Project-level Claude Code commands
3. `~/.gen/commands/` - User-level GenCode commands
4. `~/.claude/commands/` - User-level Claude Code commands

**Template variables:**
- `$ARGUMENTS` - Full argument string
- `$1`, `$2`, `$3` - Positional arguments
- `$GEN_CONFIG_DIR` - Base config directory (e.g., `~/.claude`, `~/.gen`)
- `@filepath` - File content inclusion

**Environment variable usage:**
```markdown
---
description: Example command
allowed-tools: [Bash]
---

Run helper script:
```bash
$GEN_CONFIG_DIR/scripts/helper.sh $ARGUMENTS
```
```

Using `$GEN_CONFIG_DIR` prevents LLM from seeing hardcoded paths, avoiding unnecessary file exploration.

See [docs/command-environment-variables.md](docs/command-environment-variables.md) for details.

## Key Patterns

- All file paths in tools should be resolved relative to `ToolContext.cwd`
- Tool input validation uses Zod; errors returned as `ToolResult.error`
- Provider implementations handle message format conversion internally
- CLI commands start with `/` (e.g., `/sessions`, `/resume`, `/help`)
- Commands use `$GEN_CONFIG_DIR` instead of hardcoded paths

### Edit Tool with Diff Preview

GenCode's Edit tool displays a unified diff preview before modifying files, providing transparency and safety:

**Workflow:**
1. LLM calls Edit tool with `old_string` and `new_string` parameters
2. Tool generates unified diff using `diff` library (v5.1.0)
3. `context.askPermission()` request sent with diff metadata
4. `PermissionPrompt` component renders `DiffPreview` with color-coded changes:
   - **Green (+)**: Added lines
   - **Red (-)**: Removed lines
   - **Cyan (@@)**: Hunk headers
   - **Gray**: Context lines
5. User approves/rejects via keyboard shortcuts (1/y=allow, 3/n=deny)
6. File written only after approval

**DiffPreview Features:**
- Auto-collapse for diffs >50 lines (ctrl+o to expand)
- Proper CJK character handling via `string-width`
- Respects permission modes (auto-approve in accept mode)

**Implementation files:**
- `src/cli/components/DiffPreview.tsx` - Diff rendering component
- `src/core/tools/builtin/edit.ts` - Diff generation + permission request
- `src/cli/components/PermissionPrompt.tsx` - Metadata-aware UI
- `src/core/tools/types.ts` - `PermissionRequest` interface with metadata

## Reference Projects

Similar projects for learning and reference:

| Project | Path | Description |
|---------|------|-------------|
| OpenCode | `<path-to-opencode>` | Go-based AI coding assistant with TUI, multi-provider support |
| System Prompts Collection | `<path-to-system-prompts>` | Collection of system prompts from various AI tools (Claude Code, Cursor, etc.) |
| Learn Claude Code | `<path-to-learn-claude-code>` | Educational resources for understanding Claude Code internals |

### Key Learnings from References

- **OpenCode**: Go implementation with Ink-based TUI, LSP integration, conversation sessions
- **System Prompts**: Study prompt engineering patterns used by production AI tools
- **Learn Claude Code**: Understand Claude Code's tool system, agent loop, and UX patterns
