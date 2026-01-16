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

## Quick Start

```bash
npm install -g gencode-ai
gencode
```

## Features

- [**Provider Agnostic**](./docs/providers.md) - LLM (OpenAI, Anthropic, Gemini, Vertex AI) and Search (Exa, Serper, Brave)
- **Built-in Tools** - Read, Write, Edit, Bash, Glob, Grep, WebFetch, WebSearch, TodoWrite, AskUserQuestion
- **Agent Loop** - Multi-turn conversations with tool calls
- **Session Management** - Persist and resume conversations
- **Interactive CLI** - Fuzzy search, command suggestions
- [**Permission System**](./docs/permissions.md) - Pattern-based rules, prompt-based permissions, audit logging
- **Cost Tracking** - Real-time token usage and cost estimates for all providers

## License

MIT
