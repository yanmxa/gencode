# GenCode Documentation

This directory contains documentation for the GenCode project.

## 📚 Table of Contents

### 🚀 Getting Started
- [Main README](../README.md) - Project overview and quick start

### ✅ Testing & Quality Assurance
- [**features.md**](./features.md) - Complete feature list and release testing checklist (v0.4.1)
- [manual-testing-guide.md](./manual-testing-guide.md) - Manual testing procedures
- [interactive-testing-guide.md](./interactive-testing-guide.md) - Interactive test scenarios

### 🔧 Core Features
- [providers.md](./providers.md) - Provider management and model selection
- [permissions.md](./permissions.md) - Permission system guide
- [mcp.md](./mcp.md) - MCP (Model Context Protocol) integration
- [hooks.md](./hooks.md) - Hooks system documentation
- [memory-system.md](./memory-system.md) - Memory and context management
- [session-compression.md](./session-compression.md) - Session compression implementation

### 🛠️ Customization
- [custom-commands.md](./custom-commands.md) - Creating custom slash commands
- [custom-agents.md](./custom-agents.md) - Building custom subagents

### 📊 Comparisons & Analysis
- [GENCODE_VS_CLAUDE_COMPARISON.md](./GENCODE_VS_CLAUDE_COMPARISON.md) - Feature comparison
- [cost-tracking-comparison.md](./cost-tracking-comparison.md) - Cost tracking analysis
- [config-system-comparison.md](./config-system-comparison.md) - Configuration system comparison

### 📝 Proposals
- [proposals/](./proposals/) - Enhancement proposals for new features
- [proposals/README.md](./proposals/README.md) - Proposal index

## About GenCode

GenCode is an open-source, provider-agnostic AI coding assistant. It brings Claude Code's excellent interactive CLI experience while allowing flexible switching between different LLM providers (OpenAI, Anthropic, Google Gemini, Vertex AI).

### Key Features

- **Multi-Provider Support**: Anthropic, OpenAI, Google, Vertex AI
- **11 Built-in Tools**: Read, Write, Edit, Glob, Grep, Bash, WebFetch, WebSearch, TodoWrite, AskUserQuestion, TaskOutput
- **MCP Integration**: Full Model Context Protocol support
- **Permission System**: Fine-grained access control
- **Session Management**: Auto-save, resume, compression
- **Extensible**: Custom commands, skills, agents, hooks
- **Claude Code Compatible**: Drop-in replacement with enhanced features

## Quick Links

- [Feature List & Testing](./features.md) - All features and release testing checklist
- [Enhancement Proposals](./proposals/README.md) - Future features and detailed designs
- [Main README](../README.md) - Getting started
