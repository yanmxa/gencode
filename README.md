# GenCode

Open-source AI coding assistant for the terminal, written in Go.

## Install

```bash
go install github.com/yanmxa/gencode@latest
```

<details>
<summary>Other methods</summary>

**Binary**
```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -sL "https://github.com/yanmxa/gencode/releases/latest/download/gen_${OS}_${ARCH}.tar.gz" | tar xz
sudo mv gen_* /usr/local/bin/gen
```

**From Source**
```bash
git clone https://github.com/yanmxa/gencode.git
cd gencode && make install
```

</details>

## Features

- **Multi-provider support** — Anthropic, OpenAI, Google Gemini, and more
- **Built-in tools** — Read, Write, Edit, Bash, Glob, Grep, WebFetch, WebSearch
- **Skills** — Markdown prompts with 3 states: disable, enable (slash command), active (model-aware)
- **Agents** — Specialized subagents for autonomous task execution
- **Runtime management** — `/tools`, `/skills`, `/agents` to manage at runtime

## Usage

```bash
gen                        # Interactive mode
gen "explain this code"    # Non-interactive mode
```

## License

MIT
