# GenCode

An open-source, provider-agnostic AI coding assistant.

## Why GenCode?

Claude Code is excellent - its interactive CLI experience, tool integration, and agent loop design are impressive. However, it's locked to Anthropic's Claude models.

**GenCode** brings that same great experience while giving you the freedom to:

- **Switch providers freely** - Use OpenAI, Anthropic, Google Gemini, or any OpenAI-compatible API
- **Control your costs** - Choose models that fit your budget and use case
- **Stay flexible** - No vendor lock-in, use local models or any cloud provider
- **Keep the experience** - Same intuitive CLI workflow inspired by Claude Code

## Quick Start

```bash
npm install -g gencode-ai
gencode
```

## Features

- [**Multi-Provider Support**](./docs/providers-and-models.md) - OpenAI, Anthropic, Google Gemini, Vertex AI
- **Built-in Tools** - Read, Write, Edit, Bash, Glob, Grep
- **Agent Loop** - Multi-turn conversations with tool calls
- **Session Management** - Persist and resume conversations
- **Interactive CLI** - Fuzzy search, command suggestions, streaming output

## License

MIT
