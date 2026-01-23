# GenCode

Open-source AI coding assistant for the terminal, written in Go.

## Install

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -sL "https://github.com/yanmxa/gencode/releases/latest/download/gen_${OS}_${ARCH}.tar.gz" | tar xz
sudo mv gen_* /usr/local/bin/gen
```

<details>
<summary>From Source</summary>

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
