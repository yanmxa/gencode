# GenCode

Open-source AI coding assistant for the terminal, written in Go.

## Install

**macOS (Apple Silicon)**
```bash
curl -sL https://github.com/yanmxa/gencode/releases/latest/download/gen_darwin_arm64.tar.gz | tar xz
sudo mv gen_darwin_arm64 /usr/local/bin/gen
```

**macOS (Intel)**
```bash
curl -sL https://github.com/yanmxa/gencode/releases/latest/download/gen_darwin_amd64.tar.gz | tar xz
sudo mv gen_darwin_amd64 /usr/local/bin/gen
```

**Linux (x86_64)**
```bash
curl -sL https://github.com/yanmxa/gencode/releases/latest/download/gen_linux_amd64.tar.gz | tar xz
sudo mv gen_linux_amd64 /usr/local/bin/gen
```

<details>
<summary>Other methods</summary>

**Auto-detect script**
```bash
curl -fsSL https://raw.githubusercontent.com/yanmxa/gencode/main/install.sh | bash
```

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
