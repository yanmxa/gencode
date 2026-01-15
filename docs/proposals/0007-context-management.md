# Proposal: Context Management

- **Proposal ID**: 0007
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a comprehensive context management system that tracks token usage, manages context window limits, and automatically compacts conversations when approaching limits. This ensures the agent can handle long sessions without losing important context or crashing due to context overflow.

## Motivation

Currently, mycode has no awareness of context window limits. This leads to:

1. **Session crashes**: Long conversations exceed context limits silently
2. **Lost context**: Important early information gets truncated unexpectedly
3. **No visibility**: Users can't see how much context they're using
4. **Inefficient usage**: No automatic optimization of context space
5. **Provider differences**: Different LLMs have different context limits

A context management system solves these by tracking usage and proactively managing context.

## Claude Code Reference

Claude Code implements sophisticated context management:

1. **Token Tracking**: Counts tokens for each message and tool result
2. **Context Warnings**: Alerts when approaching limits
3. **Auto-Compaction**: Automatically summarizes old messages when needed
4. **`/compact` Command**: Manual context compaction
5. **Unlimited Context**: "The conversation has unlimited context through automatic summarization"
6. **Memory Tool (Beta)**: Persistent storage that survives context clears

Key behaviors:
- Displays context usage in status line
- Warns at 80% context usage
- Automatically compacts at 90% usage
- Preserves recent messages and important context
- Maintains conversation coherence after compaction

### Memory Tool API

Claude Code's Memory Tool (beta API `context-management-2025-06-27`) enables agents to persist information across context resets:

| Command | Description | Parameters |
|---------|-------------|------------|
| `view` | List directory or read file | `path`, `view_range?` |
| `create` | Create new file | `path`, `file_text` |
| `str_replace` | Replace text | `path`, `old_str`, `new_str` |
| `insert` | Insert at line | `path`, `insert_line`, `insert_text` |
| `delete` | Delete file/directory | `path` |
| `rename` | Rename/move | `old_path`, `new_path` |

### Context Editing Configuration

```typescript
context_management: {
  edits: [{
    type: "clear_tool_uses_20250919",
    trigger: { type: "input_tokens", value: 100000 },
    keep: { type: "tool_uses", value: 3 },
    exclude_tools: ["memory"]  // Never clear memory operations
  }]
}
```

## Detailed Design

### API Design

```typescript
// src/context/types.ts
interface TokenUsage {
  prompt: number;
  completion: number;
  total: number;
}

interface ContextLimits {
  maxContextTokens: number;
  maxOutputTokens: number;
  warningThreshold: number;  // Default: 0.8 (80%)
  compactionThreshold: number;  // Default: 0.9 (90%)
}

interface ContextStats {
  currentUsage: number;
  maxTokens: number;
  usagePercent: number;
  messageCount: number;
  oldestMessageAge: Date;
}

interface CompactionResult {
  removedMessages: number;
  savedTokens: number;
  summary: string;
}
```

```typescript
// src/context/context-manager.ts
class ContextManager {
  private usage: TokenUsage;
  private limits: ContextLimits;
  private tokenizer: Tokenizer;

  constructor(provider: string, model: string);

  // Count tokens for a message
  countTokens(content: string | MessageContent[]): number;

  // Get current context statistics
  getStats(): ContextStats;

  // Check if compaction is needed
  needsCompaction(): boolean;

  // Compact conversation history
  async compact(messages: Message[], options?: CompactionOptions): Promise<CompactionResult>;

  // Update usage after API call
  updateUsage(usage: TokenUsage): void;

  // Get provider-specific context limit
  static getContextLimit(provider: string, model: string): number;
}
```

```typescript
// src/context/tokenizer.ts
interface Tokenizer {
  encode(text: string): number[];
  decode(tokens: number[]): string;
  count(text: string): number;
}

// Provider-specific tokenizer implementations
class OpenAITokenizer implements Tokenizer { ... }
class AnthropicTokenizer implements Tokenizer { ... }
class GeminiTokenizer implements Tokenizer { ... }
```

### Implementation Approach

1. **Token Counting**: Use provider-specific tokenizers (tiktoken for OpenAI, etc.)
2. **Limit Detection**: Map model names to context limits
3. **Usage Tracking**: Track cumulative usage across conversation
4. **Compaction Strategy**:
   - Preserve system prompt and memory context
   - Keep recent N messages (configurable)
   - Summarize older messages into a concise summary
   - Preserve tool results that are still relevant
5. **Auto-Compaction**: Trigger when usage exceeds threshold

```typescript
// Context limits by provider/model
const CONTEXT_LIMITS: Record<string, number> = {
  'gpt-4': 8192,
  'gpt-4-turbo': 128000,
  'gpt-4o': 128000,
  'claude-3-opus': 200000,
  'claude-3-sonnet': 200000,
  'claude-3-haiku': 200000,
  'gemini-1.5-pro': 2000000,
  'gemini-1.5-flash': 1000000,
};
```

### Memory Tool Implementation

```typescript
// src/context/memory-tool.ts
import { Tool, ToolContext, ToolResult } from '../tools/types';
import { z } from 'zod';
import * as fs from 'fs';
import * as path from 'path';

const MemoryInputSchema = z.object({
  command: z.enum(['view', 'create', 'str_replace', 'insert', 'delete', 'rename']),
  path: z.string(),
  file_text: z.string().optional(),
  view_range: z.tuple([z.number(), z.number()]).optional(),
  old_str: z.string().optional(),
  new_str: z.string().optional(),
  insert_line: z.number().optional(),
  insert_text: z.string().optional(),
  old_path: z.string().optional(),
  new_path: z.string().optional(),
});

export class MemoryTool implements Tool<z.infer<typeof MemoryInputSchema>> {
  name = 'memory';
  description = `Store and retrieve information across context resets.
Commands: view, create, str_replace, insert, delete, rename.
Use /memories as the base path.`;

  parameters = MemoryInputSchema;
  private baseDir: string;

  constructor(baseDir?: string) {
    this.baseDir = baseDir || path.join(process.env.HOME || '~', '.mycode', 'memories');
    if (!fs.existsSync(this.baseDir)) {
      fs.mkdirSync(this.baseDir, { recursive: true });
    }
  }

  async execute(input: z.infer<typeof MemoryInputSchema>): Promise<ToolResult> {
    const sanitizedPath = this.sanitizePath(input.path);
    if (!sanitizedPath) {
      return { error: `Invalid path: ${input.path}` };
    }

    switch (input.command) {
      case 'view':
        return this.handleView(sanitizedPath, input.view_range);
      case 'create':
        return this.handleCreate(sanitizedPath, input.file_text || '');
      case 'str_replace':
        return this.handleStrReplace(sanitizedPath, input.old_str!, input.new_str!);
      case 'insert':
        return this.handleInsert(sanitizedPath, input.insert_line!, input.insert_text!);
      case 'delete':
        return this.handleDelete(sanitizedPath);
      case 'rename':
        return this.handleRename(sanitizedPath, this.sanitizePath(input.new_path!)!);
      default:
        return { error: `Unknown command: ${input.command}` };
    }
  }

  private sanitizePath(inputPath: string): string | null {
    const normalized = inputPath.replace(/^\/memories\/?/, '');
    const fullPath = path.join(this.baseDir, normalized);
    const resolved = path.resolve(fullPath);
    if (!resolved.startsWith(this.baseDir)) return null;
    return resolved;
  }

  private handleView(filePath: string, range?: [number, number]): ToolResult {
    if (!fs.existsSync(filePath)) {
      return { error: `Path does not exist: ${filePath}` };
    }
    const stat = fs.statSync(filePath);
    if (stat.isDirectory()) {
      const entries = fs.readdirSync(filePath);
      return { content: `Directory contents:\n${entries.join('\n')}` };
    }
    const content = fs.readFileSync(filePath, 'utf-8');
    const lines = content.split('\n');
    const start = range ? range[0] - 1 : 0;
    const end = range ? range[1] : lines.length;
    return {
      content: lines.slice(start, end).map((l, i) =>
        `${String(start + i + 1).padStart(6)} ${l}`
      ).join('\n')
    };
  }

  private handleCreate(filePath: string, content: string): ToolResult {
    if (fs.existsSync(filePath)) {
      return { error: `File already exists: ${filePath}` };
    }
    fs.mkdirSync(path.dirname(filePath), { recursive: true });
    fs.writeFileSync(filePath, content, 'utf-8');
    return { content: `File created: ${filePath}` };
  }

  private handleStrReplace(filePath: string, oldStr: string, newStr: string): ToolResult {
    const content = fs.readFileSync(filePath, 'utf-8');
    if (!content.includes(oldStr)) {
      return { error: `String not found: ${oldStr}` };
    }
    fs.writeFileSync(filePath, content.replace(oldStr, newStr), 'utf-8');
    return { content: 'File updated successfully' };
  }

  private handleInsert(filePath: string, line: number, text: string): ToolResult {
    const content = fs.readFileSync(filePath, 'utf-8');
    const lines = content.split('\n');
    lines.splice(line, 0, text);
    fs.writeFileSync(filePath, lines.join('\n'), 'utf-8');
    return { content: `Inserted at line ${line}` };
  }

  private handleDelete(filePath: string): ToolResult {
    fs.rmSync(filePath, { recursive: true });
    return { content: `Deleted: ${filePath}` };
  }

  private handleRename(oldPath: string, newPath: string): ToolResult {
    fs.renameSync(oldPath, newPath);
    return { content: `Renamed to: ${newPath}` };
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/context/types.ts` | Create | Context management types |
| `src/context/context-manager.ts` | Create | Core context manager |
| `src/context/memory-tool.ts` | Create | Memory tool implementation |
| `src/context/tokenizer.ts` | Create | Token counting implementations |
| `src/context/compactor.ts` | Create | Conversation compaction logic |
| `src/context/index.ts` | Create | Module exports |
| `src/agent/agent.ts` | Modify | Integrate context management |
| `src/session/session-manager.ts` | Modify | Store context stats in session |
| `src/providers/types.ts` | Modify | Add token usage to responses |

## User Experience

### Status Display
Show context usage in the CLI status line:

```
mycode v0.2.0 | gpt-4o | Context: 45% (58K/128K tokens)
```

### Warning Messages
Warn users when approaching limits:

```
⚠️ Context usage at 80% (102K/128K tokens)
Consider using /compact to summarize older messages
```

### Auto-Compaction
Automatically compact when needed:

```
📦 Auto-compacting conversation (90% context usage)
Summarized 45 messages, saved 52K tokens
Conversation coherence preserved
```

### Manual Compaction
Users can manually compact:

```
> /compact
Compacting conversation...
Removed: 32 messages
Saved: 41K tokens
Summary: "Discussion about implementing authentication..."
```

### Context Command
View detailed context information:

```
> /context
Context Usage:
  Current: 58,432 tokens (45.6%)
  Maximum: 128,000 tokens
  Messages: 47
  Oldest: 2 hours ago

Breakdown:
  System prompt: 1,200 tokens
  Memory context: 800 tokens
  Conversation: 56,432 tokens
```

## Alternatives Considered

### Alternative 1: Simple Truncation
Just remove oldest messages when limit reached.

**Pros**: Simple implementation
**Cons**: Loses important context, poor UX
**Decision**: Rejected - Summarization preserves more value

### Alternative 2: External Summarization API
Use a separate API call to summarize.

**Pros**: Better summaries
**Cons**: Additional cost, latency
**Decision**: Partially adopted - Use same provider for summarization

### Alternative 3: No Auto-Compaction
Only manual compaction via command.

**Pros**: User control
**Cons**: Sessions crash unexpectedly
**Decision**: Rejected - Auto-compaction is essential for UX

## Security Considerations

1. **Token Counting Accuracy**: Inaccurate counting could lead to context overflow
2. **Compaction Privacy**: Summaries should not leak sensitive information
3. **Rate Limiting**: Compaction uses API calls, respect rate limits
4. **Caching**: Don't cache sensitive token counts

## Testing Strategy

1. **Unit Tests**:
   - Token counting accuracy for each provider
   - Limit detection for all supported models
   - Compaction logic with various message types

2. **Integration Tests**:
   - End-to-end context tracking
   - Auto-compaction triggering
   - Session persistence of context stats

3. **Load Testing**:
   - Very long conversations
   - Large tool outputs
   - Rapid message sequences

## Migration Path

1. **Phase 1**: Token counting and limit detection
2. **Phase 2**: Context stats display and warnings
3. **Phase 3**: Manual compaction command
4. **Phase 4**: Auto-compaction implementation
5. **Phase 5**: Optimized compaction strategies

Existing sessions will work without context stats; stats begin tracking on first message after upgrade.

## References

- [Claude Code Context Management](https://code.claude.com/docs/en/context)
- [OpenAI Tokenizer (tiktoken)](https://github.com/openai/tiktoken)
- [Anthropic Token Counting](https://docs.anthropic.com/en/docs/tokens)
