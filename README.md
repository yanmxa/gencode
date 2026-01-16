# GenCode

```
 ██████╗ ███████╗███╗   ██╗ ██████╗ ██████╗ ██████╗ ███████╗
██╔════╝ ██╔════╝████╗  ██║██╔════╝██╔═══██╗██╔══██╗██╔════╝
██║  ███╗█████╗  ██╔██╗ ██║██║     ██║   ██║██║  ██║█████╗
██║   ██║██╔══╝  ██║╚██╗██║██║     ██║   ██║██║  ██║██╔══╝
╚██████╔╝███████╗██║ ╚████║╚██████╗╚██████╔╝██████╔╝███████╗
 ╚═════╝ ╚══════╝╚═╝  ╚═══╝ ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝
```

Open-source AI agent. Lives in your terminal.

## Features

- **Extensible** - Modular tool system and customizable prompts
- **Multi-provider** - OpenAI, Anthropic, Gemini, or OpenAI-compatible APIs
- **Minimal** - Simple, focused, easy to extend

## Quick Start

```bash
npm install -g gencode-ai
gencode
```

### Details

- [**Multi-Provider Support**](./docs/providers.md) - Flexible provider configuration via `/provider` command
  - [LLM Providers](./docs/providers.md#llm-providers) - OpenAI, Anthropic, Google Gemini, Vertex AI
  - [Search Providers](./docs/providers.md#search-providers) - Exa AI (default), Serper.dev, Brave Search
- **Built-in Tools** - Read, Write, Edit, Bash, Glob, Grep, WebFetch, WebSearch
- **Agent Loop** - Multi-turn conversations with tool calls
- **Session Management** - Persist and resume conversations
- **Interactive CLI** - Fuzzy search, command suggestions, streaming output
- [**Permission System**](./docs/permissions.md) - Claude Code compatible permission management
  - Pattern-based rules (`Bash(git add:*)`)
  - Prompt-based permissions ("run tests", "install dependencies")
  - Session/project/global scopes
  - Persistent allowlists and audit logging

## License

MIT
