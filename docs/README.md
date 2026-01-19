# GenCode Documentation

This directory contains documentation for the GenCode project.

## 📚 Table of Contents

### 🚀 Getting Started

- [Main README](../README.md) - Project overview and quick start

### 🔧 Core Features

- [providers.md](./providers.md) - Provider management and model selection
- [permissions.md](./permissions.md) - Permission system guide
- [plan-mode.md](./plan-mode.md) - Plan mode for implementation planning
- [memory-system.md](./memory-system.md) - Memory and context management
- [session-compression.md](./session-compression.md) - Session compression implementation

### 🧩 Extensibility System

- [Slash Commands](#slash-commands) - Custom markdown-based commands ([detailed guide](./custom-commands.md))
- [Skills System](#skills-system) - Domain expertise files -> algin https://agentskills.io/home 
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
- **13 Built-in Tools**: Read, Write, Edit, Glob, Grep, Bash, Task, TaskOutput, WebFetch, WebSearch, TodoWrite, AskUserQuestion, Skill
- **Permission System**: Fine-grained access control with pattern-based rules
- **Plan Mode**: Interactive implementation planning workflow with user approval
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


## Quick Links

- [Enhancement Proposals](./proposals/README.md) - Future features and detailed designs
- [Main README](../README.md) - Getting started
