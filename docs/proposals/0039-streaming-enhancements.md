# Proposal: Streaming Enhancements

- **Proposal ID**: 0039
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement enhanced streaming capabilities for real-time LLM output, progressive tool results, and thinking blocks display, providing immediate feedback during long operations.

## Motivation

Current output is batch-based:

1. **Wait for completion**: No partial results
2. **No thinking visibility**: Can't see reasoning
3. **Tool output delayed**: Results only after completion
4. **Poor latency perception**: Feels slower than it is
5. **No progressive rendering**: All or nothing

Streaming provides real-time feedback and better UX.

## Claude Code Reference

Claude Code streams responses in real-time:

### Observed Features
- Character-by-character text streaming
- Thinking blocks display
- Tool execution progress
- Progressive code output

## Detailed Design

### API Design

```typescript
// src/streaming/types.ts
type StreamEventType =
  | 'text_start'
  | 'text_delta'
  | 'text_end'
  | 'thinking_start'
  | 'thinking_delta'
  | 'thinking_end'
  | 'tool_start'
  | 'tool_progress'
  | 'tool_end'
  | 'code_block_start'
  | 'code_block_delta'
  | 'code_block_end'
  | 'error'
  | 'done';

interface StreamEvent {
  type: StreamEventType;
  timestamp: number;
  data: unknown;
}

interface TextDeltaEvent extends StreamEvent {
  type: 'text_delta';
  data: {
    text: string;
    index: number;
  };
}

interface ThinkingDeltaEvent extends StreamEvent {
  type: 'thinking_delta';
  data: {
    text: string;
    summary?: string;
  };
}

interface ToolProgressEvent extends StreamEvent {
  type: 'tool_progress';
  data: {
    toolName: string;
    progress: number;      // 0-100
    message: string;
    output?: string;       // Partial output
  };
}

interface StreamConfig {
  showThinking: boolean;
  thinkingCollapsed: boolean;
  smoothScroll: boolean;
  bufferSize: number;
  renderDelay: number;     // ms between renders
}
```

### Stream Renderer

```typescript
// src/streaming/renderer.ts
class StreamRenderer {
  private config: StreamConfig;
  private buffer: string = '';
  private currentBlock: BlockType | null = null;
  private thinkingVisible: boolean = false;

  constructor(config?: Partial<StreamConfig>) {
    this.config = {
      showThinking: true,
      thinkingCollapsed: true,
      smoothScroll: true,
      bufferSize: 100,
      renderDelay: 16,  // ~60fps
      ...config
    };
  }

  async *render(events: AsyncGenerator<StreamEvent>): AsyncGenerator<string> {
    for await (const event of events) {
      yield* this.processEvent(event);
    }

    // Flush remaining buffer
    if (this.buffer) {
      yield this.buffer;
      this.buffer = '';
    }
  }

  private *processEvent(event: StreamEvent): Generator<string> {
    switch (event.type) {
      case 'text_delta':
        yield* this.renderTextDelta(event as TextDeltaEvent);
        break;

      case 'thinking_start':
        yield* this.renderThinkingStart();
        break;

      case 'thinking_delta':
        yield* this.renderThinkingDelta(event as ThinkingDeltaEvent);
        break;

      case 'thinking_end':
        yield* this.renderThinkingEnd();
        break;

      case 'tool_start':
        yield* this.renderToolStart(event);
        break;

      case 'tool_progress':
        yield* this.renderToolProgress(event as ToolProgressEvent);
        break;

      case 'tool_end':
        yield* this.renderToolEnd(event);
        break;

      case 'code_block_start':
        yield* this.renderCodeBlockStart(event);
        break;

      case 'code_block_delta':
        yield* this.renderCodeBlockDelta(event);
        break;
    }
  }

  private *renderTextDelta(event: TextDeltaEvent): Generator<string> {
    this.buffer += event.data.text;

    // Flush buffer at word boundaries or when large
    if (this.buffer.includes(' ') || this.buffer.length > this.config.bufferSize) {
      const lastSpace = this.buffer.lastIndexOf(' ');
      const toRender = lastSpace > 0
        ? this.buffer.slice(0, lastSpace + 1)
        : this.buffer;
      this.buffer = lastSpace > 0
        ? this.buffer.slice(lastSpace + 1)
        : '';
      yield toRender;
    }
  }

  private *renderThinkingStart(): Generator<string> {
    if (!this.config.showThinking) return;

    this.thinkingVisible = true;
    if (this.config.thinkingCollapsed) {
      yield chalk.gray('💭 Thinking...\n');
    } else {
      yield chalk.gray('┌─ Thinking ─────────────────────────────\n');
    }
  }

  private *renderThinkingDelta(event: ThinkingDeltaEvent): Generator<string> {
    if (!this.config.showThinking || this.config.thinkingCollapsed) return;

    yield chalk.gray(`│ ${event.data.text}`);
  }

  private *renderThinkingEnd(): Generator<string> {
    if (!this.config.showThinking) return;

    this.thinkingVisible = false;
    if (!this.config.thinkingCollapsed) {
      yield chalk.gray('└────────────────────────────────────────\n');
    }
    yield '\n';
  }

  private *renderToolProgress(event: ToolProgressEvent): Generator<string> {
    const { toolName, progress, message, output } = event.data;

    // Clear previous progress line and redraw
    yield '\r\x1b[K';  // Clear line

    const bar = this.progressBar(progress, 20);
    yield chalk.cyan(`[${toolName}] ${bar} ${progress}% ${message}`);

    if (output) {
      yield '\n' + chalk.gray(output);
    }
  }

  private progressBar(percent: number, width: number): string {
    const filled = Math.round((percent / 100) * width);
    const empty = width - filled;
    return chalk.green('█'.repeat(filled)) + chalk.gray('░'.repeat(empty));
  }
}

export const streamRenderer = new StreamRenderer();
```

### Provider Streaming Integration

```typescript
// Update to src/providers/types.ts
interface LLMProvider {
  name: string;
  complete(request: CompletionRequest): Promise<CompletionResponse>;
  stream(request: CompletionRequest): AsyncGenerator<StreamEvent>;
}

// Example Anthropic streaming
async function* streamAnthropic(request: CompletionRequest): AsyncGenerator<StreamEvent> {
  const response = await anthropic.messages.create({
    ...request,
    stream: true
  });

  for await (const event of response) {
    switch (event.type) {
      case 'content_block_start':
        if (event.content_block.type === 'thinking') {
          yield { type: 'thinking_start', timestamp: Date.now(), data: {} };
        }
        break;

      case 'content_block_delta':
        if (event.delta.type === 'thinking_delta') {
          yield {
            type: 'thinking_delta',
            timestamp: Date.now(),
            data: { text: event.delta.thinking }
          };
        } else if (event.delta.type === 'text_delta') {
          yield {
            type: 'text_delta',
            timestamp: Date.now(),
            data: { text: event.delta.text, index: 0 }
          };
        }
        break;

      case 'content_block_stop':
        if (currentBlock === 'thinking') {
          yield { type: 'thinking_end', timestamp: Date.now(), data: {} };
        }
        break;

      case 'message_stop':
        yield { type: 'done', timestamp: Date.now(), data: {} };
        break;
    }
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/streaming/types.ts` | Create | Type definitions |
| `src/streaming/renderer.ts` | Create | Stream rendering |
| `src/streaming/buffer.ts` | Create | Output buffering |
| `src/streaming/index.ts` | Create | Module exports |
| `src/providers/*.ts` | Modify | Add streaming support |
| `src/agent/agent.ts` | Modify | Use streaming |
| `src/cli/index.ts` | Modify | Render streams |

## User Experience

### Real-time Text Streaming
```
Agent: Let me analyze this code to understand
       the authentication flow. The AuthService
       class handles token validation and...█
```

### Thinking Block Display
```
💭 Thinking...

Agent: Based on my analysis, here are the key points...
```

### Expanded Thinking
```
┌─ Thinking ─────────────────────────────
│ First, I need to understand the current
│ authentication implementation. Looking at
│ the AuthService class...
│
│ The token validation uses JWT with HS256.
│ This could be improved by using RS256...
└────────────────────────────────────────

Agent: Based on my analysis...
```

### Tool Progress
```
[Bash] ████████░░░░░░░░░░░░ 40% Installing dependencies...
```

## Security Considerations

1. Buffer overflow prevention
2. Rate limiting output
3. Terminal escape sequence sanitization
4. Memory management for long streams

## Migration Path

1. **Phase 1**: Basic text streaming
2. **Phase 2**: Tool progress
3. **Phase 3**: Thinking blocks
4. **Phase 4**: Code block highlighting
5. **Phase 5**: Performance optimization

## References

- [Anthropic Streaming API](https://docs.anthropic.com/claude/reference/messages-streaming)
- [OpenAI Streaming](https://platform.openai.com/docs/api-reference/streaming)
- [Terminal Control Sequences](https://en.wikipedia.org/wiki/ANSI_escape_code)
