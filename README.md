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
- [**MCP Integration**](./docs/mcp.md) - Extend with external tools via Model Context Protocol (stdio, HTTP, SSE transports) - Claude Code compatible
- **Built-in Tools** - Read, Write, Edit, Bash, Glob, Grep, WebFetch, WebSearch, TodoWrite, AskUserQuestion, Task, Skill
- **Custom Commands** - Markdown-based slash commands with variable expansion ($ARGUMENTS, $1, $2) and file inclusion (@file) - Claude Code compatible
- **Skills System** - Domain expertise via SKILL.md files (hierarchical merge from ~/.gen/skills/, ~/.claude/skills/, .gen/skills/, .claude/skills/)
- **Subagent System** - Isolated task execution with specialized agents (Explore, Plan, Bash, general-purpose)
- [**Hooks System**](./docs/hooks.md) - Event-driven automation with shell commands (PostToolUse, SessionStart, Stop, etc.) - Claude Code compatible
- **Agent Loop** - Multi-turn conversations with tool calls
- **Session Management** - Persist and resume conversations
- **Interactive CLI** - Fuzzy search, command suggestions
- [**Permission System**](./docs/permissions.md) - Pattern-based rules, prompt-based permissions, audit logging
- **Cost Tracking** - Real-time token usage and cost estimates for all providers

## License

MIT
