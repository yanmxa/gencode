# GenCode Documentation

This directory contains documentation for the GenCode project.

## 📚 Table of Contents

### 🚀 Getting Started

- [Main README](../README.md) - Project overview and quick start

### 🔧 Core Features

- [providers.md](./providers.md) - Provider management and model selection
- [permissions.md](./permissions.md) - Permission system guide
- [memory-system.md](./memory-system.md) - Memory and context management
- [session-compression.md](./session-compression.md) - Session compression implementation

### 🧩 Extensibility System

- [Slash Commands](#slash-commands) - Custom markdown-based commands ([detailed guide](./custom-commands.md))
- [Skills System](#skills-system) - Domain expertise files
- [Subagent System](#subagent-system) - Specialized agents ([detailed guide](./custom-agents.md))
- [MCP Integration](#mcp-integration) - Model Context Protocol ([detailed guide](./mcp.md))
- [Hooks System](#hooks-system) - Event-driven automation ([detailed guide](./hooks.md))

### 📝 Proposals

- [proposals/](./proposals/) - Enhancement proposals for new features
- [proposals/README.md](./proposals/README.md) - Proposal index

## About GenCode

GenCode is an open-source, provider-agnostic AI coding assistant. It brings Claude Code's excellent interactive CLI experience while allowing flexible switching between different LLM providers (OpenAI, Anthropic, Google Gemini, Vertex AI).

### Key Features

#### Core Capabilities
- **Multi-Provider Support**: Anthropic, OpenAI, Google, Vertex AI
- **12 Built-in Tools**: Read, Write, Edit, Glob, Grep, Bash, WebFetch, WebSearch, TodoWrite, AskUserQuestion, TaskOutput, Skill
- **Permission System**: Fine-grained access control with pattern-based rules
- **Session Management**: Auto-save, resume, compression
- **Cost Tracking**: Real-time token usage and cost estimates

#### Extensibility Features
- **Slash Commands**: Markdown-based custom commands with variable expansion ($ARGUMENTS, $1, $2) and file inclusion (@file)
- **Skills System**: Domain expertise files with hierarchical merge from multiple locations
- **Subagent System**: Specialized agents for isolated task execution (Explore, Plan, Bash, general-purpose)
- **MCP Integration**: Full Model Context Protocol support (stdio, HTTP, SSE transports)
- **Hooks System**: Event-driven automation with shell commands (PreToolUse, PostToolUse, SessionStart, Stop, etc.)

#### Compatibility
- **Claude Code Compatible**: Drop-in replacement with full compatibility for commands, agents, hooks, and MCP configurations

### Slash Commands

Custom slash commands are markdown-based prompt templates that support variable expansion and file inclusion. They're fully compatible with Claude Code's command format.

**Key Features:**
- Variable expansion: `$ARGUMENTS`, `$1`, `$2`, etc.
- File inclusion: `@README.md`, `@src/index.ts`
- Pre-authorization: `allowed-tools` in frontmatter
- Model override: Specify different model for command execution
- Hierarchical loading: Project > User > Claude Code fallback

**Example:**
```markdown
---
description: Commit changes with a message
argument-hint: <message>
allowed-tools:
  - Bash(git *)
---
Please commit all changes with this message: $1
```

**Learn more:** [custom-commands.md](./custom-commands.md)

### Skills System

Skills are domain expertise files that provide specialized knowledge to the agent. They can contain instructions, examples, and context about specific domains or workflows.

**Key Features:**
- Hierarchical merge from multiple locations
- Priority: `.gen/skills/` > `~/.gen/skills/` > `.claude/skills/` > `~/.claude/skills/`
- Supports nested directory structure
- Markdown format with optional frontmatter
- Automatically injected into agent context when invoked via `Skill` tool

**Locations:**
1. `~/.claude/skills/` - User-level Claude Code skills
2. `~/.gen/skills/` - User-level GenCode skills (overrides Claude Code)
3. `.claude/skills/` - Project-level Claude Code skills
4. `.gen/skills/` - Project-level GenCode skills (overrides Claude Code)

**Usage:**
```typescript
// In conversation
> /myskill arg1 arg2

// Via Skill tool
Skill({
  skill: "myskill",
  args: "arg1 arg2"
})
```

### Subagent System

Subagents are specialized agents that run isolated task execution with their own context and tool access. They can be built-in or custom-defined.

**Built-in Subagents:**
- **Explore**: Fast codebase exploration and pattern searching
- **Plan**: Software architecture and implementation planning
- **Bash**: Command execution specialist
- **general-purpose**: Multi-step task automation

**Key Features:**
- Isolated execution context with `permissionMode` control
- Configurable tool access via `allowedTools`
- Model override with `defaultModel`
- Turn limit control with `maxTurns`
- JSON or Markdown format
- Merge strategy: GenCode agents override Claude Code agents

**Example:**
```json
{
  "name": "code-reviewer",
  "type": "custom",
  "description": "Expert code review specialist",
  "allowedTools": ["Read", "Grep", "Glob"],
  "defaultModel": "claude-sonnet-4",
  "maxTurns": 15,
  "permissionMode": "permissive",
  "systemPrompt": "You are a senior code reviewer..."
}
```

**Learn more:** [custom-agents.md](./custom-agents.md)

### MCP Integration

Model Context Protocol (MCP) allows extending GenCode with external tools and data sources through MCP servers.

**Supported Transports:**
- **stdio**: Local process communication
- **HTTP**: Remote server with StreamableHTTP
- **SSE**: Server-Sent Events (legacy)

**Key Features:**
- Tool namespacing: `mcp_<server>_<tool>`
- OAuth 2.0 authentication support
- Environment variable expansion
- Multiple configuration scopes (managed, local, project, user)
- Compatible with Claude Code MCP configs

**Example:**
```json
{
  "mcpServers": {
    "github": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

**Learn more:** [mcp.md](./mcp.md)

### Hooks System

The hooks system enables event-driven automation by executing shell commands in response to events like tool execution and session lifecycle.

**Available Events:**
- `PreToolUse` - Before tool execution (can block)
- `PostToolUse` - After successful tool execution
- `PostToolUseFailure` - After failed tool execution
- `SessionStart` - When session starts or resumes
- `Stop` - When agent completes

**Key Features:**
- Pattern-based matchers: `Write|Edit`, `Bash(git *)`, `*`
- Blocking hooks with exit code 2
- Environment variables: `$TOOL_NAME`, `$FILE_PATH`, `$SESSION_ID`
- JSON context via stdin
- Compatible with Claude Code hooks

**Example:**
```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "prettier --write $FILE_PATH",
            "timeout": 5000
          }
        ]
      }
    ]
  }
}
```

**Learn more:** [hooks.md](./hooks.md)

## Quick Links

- [Enhancement Proposals](./proposals/README.md) - Future features and detailed designs
- [Main README](../README.md) - Getting started
