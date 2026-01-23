# GenCode

Open-source AI coding assistant for the terminal, written in Go.

## Install

### Homebrew (macOS/Linux)

```bash
brew install yanmxa/tap/gen
```

### Shell Script

```bash
curl -fsSL https://raw.githubusercontent.com/yanmxa/gencode/main/install.sh | bash
```

### Go

```bash
go install github.com/yanmxa/gencode/cmd/gen@latest
```

### From Source

```bash
git clone https://github.com/yanmxa/gencode.git
cd gencode && go build -o gen ./cmd/gen
sudo mv gen /usr/local/bin/
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
