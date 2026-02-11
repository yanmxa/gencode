<p align="center">
  <h1 align="center">GenCode</h1>
  <p align="center">
    <strong>Open-source AI assistant for the terminal</strong>
  </p>
  <p align="center">
    <a href="https://github.com/yanmxa/gencode/releases"><img src="https://img.shields.io/github/v/release/yanmxa/gencode?style=flat-square" alt="Release"></a>
    <a href="https://goreportcard.com/report/github.com/yanmxa/gencode"><img src="https://goreportcard.com/badge/github.com/yanmxa/gencode?style=flat-square" alt="Go Report Card"></a>
    <a href="https://pkg.go.dev/github.com/yanmxa/gencode"><img src="https://pkg.go.dev/badge/github.com/yanmxa/gencode.svg" alt="Go Reference"></a>
    <a href="LICENSE"><img src="https://img.shields.io/github/license/yanmxa/gencode?style=flat-square" alt="License"></a>
  </p>
</p>

GenCode is an AI assistant that lives in your terminal. Multi-provider support, built-in tools, and a flexible skill/agent system compatible with Claude Code.

## ✨ Features

- **Multi-provider Support** — Anthropic Claude, OpenAI, Google Gemini, Moonshot Kimi
- **Built-in Tools** — Read, Write, Edit, Bash, Glob, Grep, WebFetch, WebSearch
- **Skills System** — Reusable prompts with 3 states: disabled, enabled (slash command), active (model-aware)
- **Subagents** — Specialized agents (Explore, Plan, Bash, Review) for autonomous task execution
- **Session Persistence** — Save, resume, and manage conversation sessions
- **Non-interactive Mode** — Pipe input or pass messages directly for scripting

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

### Providers

| Provider | Auth | Environment Variables | Models |
|----------|------|----------------------|--------|
| **Anthropic Claude** | API Key, Vertex AI | `ANTHROPIC_API_KEY` or `ANTHROPIC_VERTEX_PROJECT_ID`, `CLOUD_ML_REGION` | Claude Opus, Sonnet, Haiku |
| **OpenAI** | API Key | `OPENAI_API_KEY` | GPT-5.2, GPT-5, o3/o4-mini, Codex |
| **Google Gemini** | API Key | `GOOGLE_API_KEY` (or `GEMINI_API_KEY`) | Gemini 3 Pro/Flash, Gemini 2.5 Pro/Flash |
| **Moonshot Kimi** | API Key | `MOONSHOT_API_KEY`, `MOONSHOT_BASE_URL` (optional) | Kimi K2.5, K2 Thinking |

## 🔗 Related Projects

- [Claude Code](https://claude.ai/code) — Anthropic's AI coding assistant
- [Aider](https://github.com/paul-gauthier/aider) — AI pair programming in terminal
- [Continue](https://github.com/continuedev/continue) — Open-source AI code assistant

## 🤝 Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.
