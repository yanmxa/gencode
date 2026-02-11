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

### Commands

| Command | Description |
|---------|-------------|
| `/provider` | Connect to an LLM provider |
| `/model` | Select a model |
| `/tools` | List available tools |
| `/skills` | Manage skills (disable/enable/active) |
| `/agents` | List available agents |
| `/clear` | Clear chat history |
| `/help` | Show all commands |

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Alt+Enter` | Insert newline |
| `↑/↓` | Navigate history |
| `Esc` | Stop AI response |
| `Ctrl+C` | Clear input / Quit |

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

<details>
<summary><b>Anthropic Claude</b></summary>

- **Auth:** API Key, Vertex AI
- **Env:** `ANTHROPIC_API_KEY`
- **Vertex AI Env:** `ANTHROPIC_VERTEX_PROJECT_ID`, `CLOUD_ML_REGION` (defaults to `us-east5`)
- **Models:** Claude Opus, Sonnet, Haiku (via API or Vertex AI)

</details>

<details>
<summary><b>OpenAI</b></summary>

- **Auth:** API Key
- **Env:** `OPENAI_API_KEY`
- **Models:** GPT-4o, GPT-4, o1/o3, Codex, etc.

</details>

<details>
<summary><b>Google Gemini</b></summary>

- **Auth:** API Key
- **Env:** `GOOGLE_API_KEY` (or `GEMINI_API_KEY`)
- **Models:** Gemini 2.5 Pro/Flash, etc.

</details>

<details>
<summary><b>Moonshot Kimi</b></summary>

- **Auth:** API Key (OpenAI-compatible)
- **Env:** `MOONSHOT_API_KEY`, `MOONSHOT_BASE_URL` (optional, defaults to `https://api.moonshot.cn/v1`)
- **Models:** Kimi K2.5, K2 Thinking, Moonshot V1, etc.
- **Features:** Thinking mode, multimodal (image/video for supported models)

</details>

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `GOOGLE_API_KEY` | Google AI API key |
| `MOONSHOT_API_KEY` | Moonshot (Kimi) API key |
| `GEN_DEBUG` | Set to `1` to enable debug logging |

## 🛠️ Built-in Tools

| Tool | Description |
|------|-------------|
| `Read` | Read file contents |
| `Write` | Create or overwrite files |
| `Edit` | Edit files with diff preview |
| `Bash` | Execute shell commands |
| `Glob` | Find files by pattern |
| `Grep` | Search file contents |
| `WebFetch` | Fetch web page content |
| `WebSearch` | Search the web |
| `Task` | Spawn subagents for complex tasks |
| `Skill` | Invoke custom skills |

## 🔗 Related Projects

- [Claude Code](https://claude.ai/code) — Anthropic's AI coding assistant
- [Aider](https://github.com/paul-gauthier/aider) — AI pair programming in terminal
- [Continue](https://github.com/continuedev/continue) — Open-source AI code assistant

## 🤝 Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## 📄 License

MIT License - see [LICENSE](LICENSE) for details.
