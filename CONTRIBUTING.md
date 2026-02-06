# Contributing to GenCode

Thanks for your interest in contributing! This guide will help you get started.

## Quick Start

```bash
git clone https://github.com/yanmxa/gencode.git
cd gencode
go build -o gen ./cmd/gen
./gen
```

## Development

### Prerequisites

- Go 1.21+
- An LLM API key (Anthropic, OpenAI, or Google)

### Project Structure

```
cmd/gen/           # CLI entry point
internal/
├── provider/      # LLM providers (anthropic, openai, google)
├── tool/          # Built-in tools (read, write, edit, bash, etc.)
├── tui/           # Terminal UI (Bubble Tea)
├── mcp/           # MCP protocol support
├── config/        # Settings and permissions
└── system/        # System prompt generation
```

### Run Tests

```bash
go test ./...
```

### Debug Mode

```bash
GEN_DEBUG=1 ./gen
# Logs written to ~/.gen/debug.log
```

## How to Contribute

### Report Bugs

Open an issue with:
- Steps to reproduce
- Expected vs actual behavior
- OS, Go version, and provider used

### Suggest Features

Open an issue describing:
- The problem you're solving
- Your proposed solution
- Alternative approaches considered

### Submit Code

1. Fork the repo
2. Create a branch: `git checkout -b feature/your-feature`
3. Make changes and test
4. Commit with sign-off: `git commit -s -m "feat: add feature"`
5. Push and open a PR

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new feature
fix: resolve bug
docs: update documentation
refactor: restructure code
test: add tests
chore: maintenance tasks
```

## Areas for Contribution

| Area | Description |
|------|-------------|
| **Providers** | Add new LLM providers (Ollama, Mistral, etc.) |
| **Tools** | Create new built-in tools |
| **MCP** | Improve MCP server support |
| **TUI** | Enhance terminal UI/UX |
| **Docs** | Improve documentation |
| **Tests** | Increase test coverage |

## Code of Conduct

Be respectful and constructive. We welcome contributors of all backgrounds and experience levels.

## Questions?

Open an issue or start a discussion. We're happy to help!
