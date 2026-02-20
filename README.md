<p align="center">
  <h1 align="center">❮ GEN ✦ /❯</h1>
  <p align="center">
    <strong>Open-source AI coding assistant for the terminal</strong>
  </p>
  <p align="center">
    <a href="https://github.com/yanmxa/gencode/releases"><img src="https://img.shields.io/github/v/release/yanmxa/gencode?style=flat-square" alt="Release"></a>
    <a href="https://goreportcard.com/report/github.com/yanmxa/gencode"><img src="https://goreportcard.com/badge/github.com/yanmxa/gencode?style=flat-square" alt="Go Report Card"></a>
    <a href="https://pkg.go.dev/github.com/yanmxa/gencode"><img src="https://pkg.go.dev/badge/github.com/yanmxa/gencode.svg" alt="Go Reference"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue?style=flat-square" alt="License"></a>
  </p>
</p>

An open-source AI coding assistant for the terminal. Multi-provider support, flexible context management, compatible with [Claude Code](https://claude.ai/code) extensions and plugins.

## ✨ Features

- **Multi-provider** — Anthropic, OpenAI, Gemini, Moonshot, etc. — switch with `/provider`
- **Tools** — Built-in (Edit, Bash, WebSearch, etc.) + [MCP](https://modelcontextprotocol.io), dynamic enable/disable for context control
- **Skills** — Model visibility control (off/command/aware), [Claude Code](https://claude.ai/code) compatible
- **Subagents** — Dedicated LLM instances with isolated context and tools, background execution support
- **Plugins** — Bundle skills/agents/hooks/MCP, marketplace install, [Claude Code](https://claude.ai/code) compatible
- **Session** — Persist, resume, search, auto-cleanup, with context compact
- **Others** — Plan mode, task management, hooks, etc.

### Providers

| Provider | Models | Auth | Environment Variables |
|:---------|:-------|:-----|:----------------------|
| **Anthropic** | Claude Opus 4.6, Sonnet 4.6 | API Key / [Vertex AI](https://code.claude.com/docs/en/google-vertex-ai) | `ANTHROPIC_API_KEY` |
| **OpenAI** | GPT-5.2, GPT-5, o3, o4-mini, Codex | API Key | `OPENAI_API_KEY` |
| **Google** | Gemini 3 Pro/Flash, 2.5 Pro/Flash | API Key | `GOOGLE_API_KEY` |
| **Moonshot** | Kimi K2.5, K2 Thinking | API Key | `MOONSHOT_API_KEY` |
| **Alibaba** | Qwen 3, Qwen 2.5 | _Planning_ | — |

## 🚀 Installation

```bash
curl -fsSL https://raw.githubusercontent.com/yanmxa/gencode/main/install.sh | bash
```

Re-run the same command to upgrade. To uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/yanmxa/gencode/main/install.sh | bash -s uninstall
```

<details>
<summary><b>Other methods</b></summary>

**Go Install**

```bash
go install github.com/yanmxa/gencode/cmd/gen@latest
```

**Build from Source**

```bash
git clone https://github.com/yanmxa/gencode.git
cd gencode
go build -o gen ./cmd/gen
mkdir -p ~/.local/bin && mv gen ~/.local/bin/
```

</details>

## 📖 Usage

```bash
# Interactive mode
gen

# Non-interactive mode
gen "explain this function"
cat main.go | gen "review this code"

# Resume previous session
gen --continue        # Resume most recent
gen --resume          # Select from list
```

### Quick Start

1. Run `gen` to start interactive mode
2. Use `/provider` to connect to an LLM provider
3. Use `/model` to select a model
4. Start chatting!


## 🔧 Configuration

GenCode stores configuration in `~/.gen/`:

```
~/.gen/
├── providers.json    # Provider connections and current model
├── settings.json     # User settings
├── skills.json       # Skill states
├── sessions/         # Saved conversation sessions
├── skills/           # Custom skills
└── agents/           # Custom agents
```


## 🔗 Related Projects

- [Claude Code](https://claude.ai/code) — Anthropic's AI coding assistant
- [Aider](https://github.com/paul-gauthier/aider) — AI pair programming in terminal
- [Continue](https://github.com/continuedev/continue) — Open-source AI code assistant

## 🤝 Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## 📄 License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
