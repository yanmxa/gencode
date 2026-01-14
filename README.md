# mycode

An open-source, provider-agnostic alternative to Claude Code.

## Why mycode?

Claude Code is excellent - its interactive CLI experience, tool integration, and agent loop design are impressive. However, it's locked to Anthropic's Claude models.

**mycode** aims to bring that same great experience while giving you the freedom to:

- **Switch providers freely** - Use OpenAI, Anthropic, Google Gemini, or any OpenAI-compatible API
- **Control your costs** - Choose models that fit your budget and use case
- **Stay flexible** - No vendor lock-in, use local models or any cloud provider
- **Keep the experience** - Same intuitive CLI workflow inspired by Claude Code

## Quick Start

```bash
# Install globally
npm install -g mycode-cli

# Run
mycode
```

Or run from source:

```bash
# Install dependencies
npm install

# Build
npm run build

# Run CLI
npm start
```

## Usage

### CLI Mode

```bash
# Start interactive CLI
mycode

# Resume last session
mycode -c

# Select from recent sessions
mycode -r
```

### Programmatic Usage

```typescript
import { Agent } from 'mycode-cli';

const agent = new Agent({
  provider: 'gemini',  // 'openai' | 'anthropic' | 'gemini'
  model: 'gemini-2.0-flash',
});

for await (const event of agent.run('List files in current directory')) {
  if (event.type === 'text') {
    console.log(event.text);
  }
}
```

## Configuration

Create a `.env` file:

```bash
# Choose one or more providers
OPENAI_API_KEY=sk-xxx
ANTHROPIC_API_KEY=sk-ant-xxx
GOOGLE_API_KEY=xxx

# Optional: override provider/model
MYCODE_PROVIDER=anthropic
MYCODE_MODEL=claude-sonnet-4-20250514

# Optional proxy
HTTP_PROXY=http://proxy:3128
HTTPS_PROXY=http://proxy:3128
```

## Features

| Feature | Status |
|---------|--------|
| Multi-LLM Support (OpenAI/Anthropic/Gemini) | ✅ |
| Built-in Tools (Read/Write/Edit/Bash/Glob/Grep) | ✅ |
| Permission System (auto/confirm/deny) | ✅ |
| Agent Loop (multi-turn + tool calls) | ✅ |
| Streaming Output | ✅ |
| Interactive CLI | ✅ |
| Session Management | ✅ |
| Proxy Support | ✅ |

## Project Structure

```
src/
├── agent/           # Agent system (tool loop)
├── providers/       # LLM providers (OpenAI, Anthropic, Gemini)
├── tools/           # Built-in tools
├── permissions/     # Permission management
├── session/         # Session persistence
└── cli/             # Interactive CLI
```

## License

MIT
