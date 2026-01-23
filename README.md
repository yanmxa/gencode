# GenCode

Open-source AI coding assistant for the terminal, written in Go.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/yanmxa/gencode/main/install.sh | bash
```

<details>
<summary>Other methods</summary>

**Go**
```bash
go install github.com/yanmxa/gencode/cmd/gen@latest
```

**From Source**
```bash
git clone https://github.com/yanmxa/gencode.git
cd gencode && make install
```

</details>

## Features

- Multi-provider support (Anthropic, OpenAI, Google)
- Built-in tools (Read, Write, Edit, Bash, Glob, Grep, WebFetch, WebSearch)
- Interactive TUI with diff preview for file changes
- Non-interactive mode for scripting

## Usage

```bash
gen                        # Interactive mode
gen "explain this code"    # Non-interactive mode
```

## License

MIT
