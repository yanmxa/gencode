# GenCode

Open-source AI coding assistant for the terminal, written in Go.

## Install

```bash
go install github.com/myan/gencode/cmd/gen@latest
```

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
