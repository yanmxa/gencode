<p align="center">
  <h1 align="center">< GEN ✦ /></h1>
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

An open-source AI coding assistant for the terminal built with Go. Multi-provider LLM support, event-driven multi-agent orchestration, and full compatibility with [Claude Code](https://claude.ai/code) extensions, plugins, and project instructions.

## Features

- **Multi-provider** — Anthropic, OpenAI, Google, Moonshot, Alibaba — switch with `/model`
- **Tools & MCP** — Built-in tools (Edit, Bash, Glob, Grep, WebSearch, etc.) + [MCP](https://modelcontextprotocol.io) integration
- **Skills, Subagents & Plugins** — [Claude Code](https://claude.ai/code) compatible format, marketplace install
- **Event-driven multi-agent** — Parallel agent execution with decoupled event-based coordination
- **Hooks** — Lifecycle extensibility via shell, LLM, agent, or HTTP hooks
- **Session** — Auto-persist, resume, fork, auto-compact
- **Performance** — Minimal context injection, fast response, low token consumption
- **Other** — Prompt prediction, configurable thinking effort, scheduled loops, permission control, etc.

### Providers

| Provider | Models | Environment Variables |
|:---------|:-------|:----------------------|
| **Anthropic** | Claude Opus 4.6, Sonnet 4.6 | `ANTHROPIC_API_KEY` or [Vertex AI](https://cloud.google.com/vertex-ai/generative-ai/docs/partner-models/claude) |
| **OpenAI** | GPT-5.2, GPT-5, o3, o4-mini, Codex | `OPENAI_API_KEY` |
| **Google** | Gemini 3 Pro/Flash, 2.5 Pro/Flash | `GOOGLE_API_KEY` |
| **Moonshot** | Kimi K2.5, K2 Thinking | `MOONSHOT_API_KEY` |
| **Alibaba** | Qwen3.5 Plus, Qwen3 Max/Plus/Flash, QwQ, DeepSeek-V3/R1 | `DASHSCOPE_API_KEY` |


## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/yanmxa/gencode/main/install.sh | bash
```

Re-run to upgrade. To uninstall:

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

## Usage

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
2. Use `/model` to connect a provider and select a model
3. Start chatting!

### Commands

`/model` `/tools` `/skills` `/agents` `/mcp` `/plugin` `/compact` `/think` `/search` `/loop` `/resume` `/fork` `/clear` `/init` `/memory` — type `/help` for details.

Keyboard: `Shift+Tab` toggle permission mode, `Ctrl+O` expand tool details, `Ctrl+C` cancel, `Ctrl+D` exit.

## Configuration

GenCode stores configuration in `~/.gen/`:

```
~/.gen/
├── providers.json    # Provider connections and current model
├── settings.json     # User settings (permissions, hooks, env)
├── skills.json       # Skill states
├── projects/         # Project-scoped session transcripts + indexes
├── skills/           # Custom skill definitions
├── agents/           # Custom agent definitions
├── commands/         # Custom slash commands
└── plugins/          # Installed plugins
```

### Project Instructions

Place a `GEN.md` (or `CLAUDE.md`) in your project root to provide project-specific instructions. These are automatically loaded into the system prompt. Project-level settings can also be placed in `.gen/settings.json`.

## Benchmark: GenCode vs Claude Code

Compared with [Claude Code](https://claude.ai/code) v2.1.112 on Apple Silicon, same model (`claude-sonnet-4-6`):

| Metric | GenCode | Claude Code | Advantage |
|--------|---------|-------------|-----------|
| Download size | 12 MB | 63 MB (+ Node.js 112 MB) | **5x smaller** |
| Disk footprint | 38 MB | 175 MB | **4.6x smaller** |
| Startup time | ~0.01s | ~0.20s | **20x faster** |
| Startup memory | ~32 MB | ~189 MB | **5.8x less** |
| Simple task | ~2.4s / 39 MB | ~10.4s / 286 MB | **4.3x faster, 7.3x less memory** |
| Tool-use task | ~3.3s / 39 MB | ~26.0s / 285 MB | **7.9x faster, 7.2x less memory** |

Both tools have comparable features (hooks, skills, plugins, session, MCP, etc.). The performance gap comes from Go's native compilation vs Node.js V8/JIT/GC runtime overhead.

See full details: [docs/benchmark-gencode-vs-claudecode.md](docs/benchmark-gencode-vs-claudecode.md)

## Related Projects

- [Claude Code](https://claude.ai/code) — Anthropic's AI coding assistant
- [Aider](https://github.com/paul-gauthier/aider) — AI pair programming in terminal
- [Continue](https://github.com/continuedev/continue) — Open-source AI code assistant

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE) for details.
