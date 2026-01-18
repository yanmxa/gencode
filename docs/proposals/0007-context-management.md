# Proposal: Context Management

- **Proposal ID**: 0007
- **Author**: mycode team
- **Status**: Implemented - Pending Verification
- **Created**: 2025-01-15
- **Updated**: 2026-01-18

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

## Implementation Status

### ✅ Implemented (Phase 1-3)

**Session Compression System**:
- ✅ `CompressionEngine` class with Layer 1 (Pruning) and Layer 2 (Compaction)
  - Message deduplication and quality scoring
  - Context-aware summarization
  - Intelligent message selection (recent, high-value, tool results)
  - Configurable thresholds and parameters
- ✅ Integration with `SessionManager`
  - Automatic compression when approaching context limits
  - Compression statistics tracking
  - Persistent compression metadata in session files

**CLI Commands**:
- ✅ `/compact` - Manual conversation compaction
  - Triggers compression immediately
  - Shows statistics (active/total messages, summaries, saved %)
  - Visual ASCII box display with progress bars
- ✅ `/context` - Context usage statistics
  - Shows active vs total message counts
  - Displays compression status (Compressed/Uncompressed)
  - Progress bar visualization
  - ASCII box display with colored status

**UI Rendering Fixes** (2026-01-18):
- ✅ Fixed info icon "ℹ" appearing on separate line before box output
  - Added box content detection in `renderHistoryItem()` (App.tsx:1389-1396)
  - Box content now renders directly without InfoMessage wrapper
- ✅ Fixed right border alignment for `/context` and `/compact` commands
  - Corrected padding calculation from `-2` to `-3` (App.tsx:904, 965)
  - All border characters (`+`, `|`) now perfectly aligned
  - Consistent 50-character width across all lines

**Visual Output** (After Fixes):
```
+------------------------------------------------+
| Context Usage Statistics                       |
+------------------------------------------------+
| Active Messages      12                        |
| Total Messages       45                        |
| Summaries             2                        |
|                                                |
| Usage  [#####...............]  27%             |
|                                                |
| Status: Compressed                             |
+------------------------------------------------+
```

### ✅ Newly Implemented (2026-01-18 - Pending Verification)

**Token Counting & Tracking**:
- ✅ **Cumulative token tracking** from API responses
  - `SessionManager.cumulativeTokens` tracks input/output/total
  - `calculateCumulativeTokens()` sums from session metadata
  - `updateTokenUsageFromLatestCompletion()` for incremental updates
  - `getTokenUsage()` public getter for current usage
  - Persisted to `session.metadata.tokenUsage` on save
- ✅ **Actual API token usage** instead of 4:1 estimates
  - Token usage passed to `CompressionEngine.needsCompression()`
  - Uses provider-returned `inputTokens` and `outputTokens`
  - Falls back to 4:1 estimate if API doesn't return usage

**Auto-Compaction with Thresholds**:
- ✅ **Threshold-based compression triggering**
  - 80% warning threshold: emits `context-warning` event
  - 90% auto-compact threshold: triggers compression automatically
  - Returns `usagePercent` and `shouldWarn` flags from `needsCompression()`
- ✅ **Event-driven architecture** with EventEmitter
  - `context-warning` - Emitted at 80% usage
  - `auto-compacting` - Emitted before compression at 90%
  - `compaction-complete` - Emitted after compression finishes
- ✅ **User feedback in UI**
  - ⚠️ "Context usage at 82% - Consider using /compact"
  - 📦 "Auto-compacting (91% usage, strategy: prune)..."
  - ✓ "Compaction complete (prune)"
  - Smart warning deduplication (shows once per session)

**Status Display**:
- ✅ **Context usage in header**
  - Format: `Context: 45/120 msgs (37%)`
  - Real-time updates after each completion
  - Only shown when activeMessages > 0
  - Calculates percentage from actual token usage vs context window
- ✅ **Real-time token tracking**
  - Header refreshes on every render
  - Pulls from `SessionManager.getTokenUsage()` and `getCompressionStats()`

**Implementation Details**:
- ✅ **Files Modified**:
  - `src/session/manager.ts` - Token tracking, event emission
  - `src/session/compression/engine.ts` - Threshold logic
  - `src/cli/components/Header.tsx` - Context stats display
  - `src/cli/components/App.tsx` - Event listeners, header updates
- ✅ **Backward Compatible**: Works with existing sessions
- ✅ **Build Status**: TypeScript compilation successful

### ❌ Not Implemented (Deferred - Low Priority)

**Provider-Specific Tokenizers**:
- ❌ Client-side tokenizer implementations
  - No OpenAITokenizer, AnthropicTokenizer, or GeminiTokenizer classes
  - Not needed: Using actual API token counts instead
  - Could be added later for pre-submission estimates
  - **Decision**: Deferred - API usage is more accurate

**Memory Tool**:
- ❌ Claude Code-style Memory Tool implementation
  - No persistent storage across context resets
  - Memory system exists but uses different approach (GEN.md files)
  - **Decision**: Out of scope for this proposal

### 📋 Verification & Testing Required

**Core functionality implemented - needs real-world testing:**

1. **Verification Tasks** (High Priority):
   - ✅ Build successful - TypeScript compilation passed
   - ⏳ **Test 80% warning trigger** - Start long conversation and verify warning appears
   - ⏳ **Test 90% auto-compact** - Continue until auto-compaction triggers
   - ⏳ **Verify token accuracy** - Compare displayed tokens vs API actual usage
   - ⏳ **Test header display** - Confirm context stats update in real-time
   - ⏳ **Test session persistence** - Reload session and verify token counts preserved
   - ⏳ **Test event deduplication** - Verify warning only shows once per session

2. **Edge Cases to Test**:
   - Session load with no token usage data (backward compatibility)
   - Session fork inherits correct token counts
   - Compression resets warning flag after compaction
   - Multiple rapid completions don't spam warnings
   - Very short sessions (< 10 messages) display correctly

3. **Future Optimizations** (Low Priority - Post-Verification):
   - Advanced compaction strategies
   - Better summarization quality
   - Provider-specific tokenizers for pre-submission estimates
   - Memory tool integration

### 📁 Implementation Files

| File | Status | Notes |
|------|--------|-------|
| `src/session/compression/engine.ts` | ✅ Complete | Layer 1 & 2 compression + threshold logic |
| `src/session/compression/types.ts` | ✅ Complete | All compression types |
| `src/session/compression/index.ts` | ✅ Complete | Module exports |
| `src/session/manager.ts` | ✅ Modified | Token tracking + EventEmitter + compression |
| `src/session/types.ts` | ✅ Modified | Token usage in metadata |
| `src/cli/components/App.tsx` | ✅ Modified | Event listeners + header stats |
| `src/cli/components/Header.tsx` | ✅ Modified | Context stats display |
| `src/context/tokenizer.ts` | ⏸️ Deferred | Using API token counts instead |
| `src/context/context-manager.ts` | ⏸️ Deferred | Context tracking in SessionManager |

### 🐛 Bug Fixes

**UI Rendering Issues** (Fixed 2026-01-18):

**Problem 1**: Info icon "ℹ" appearing on separate line before box output
- **Root Cause**: `InfoMessage` component always prepended icon, causing it to appear on separate line
- **Solution**: Added box content detection (`content.trim().startsWith('+---')`) in `renderHistoryItem()`
- **Files Changed**: `src/cli/components/App.tsx` (lines 1389-1396)

**Problem 2**: Right border `|` not aligned properly
- **Root Cause**: Padding calculation was off by 1 character
- **Before**: `w - text.length - 2` and `w - visible - 2`
- **After**: `w - text.length - 3` and `w - visible - 3`
- **Explanation**: Border line `'| ' + pad(text) + '|'` = 2 + pad + 1 = w, so pad = w - 3
- **Files Changed**: `src/cli/components/App.tsx` (lines 904, 965)

**Test Results**:
```
✅ All lines same length: true
✅ Expected: 50, Actual: 50
✅ /compact box: Passed
✅ /context box: Passed
✅ No info icon in output
✅ Perfect border alignment
```

---

## 📦 Latest Implementation (2026-01-18)

### Summary

Completed all high-priority features from the "Remaining Work" section:
- ✅ Accurate token tracking from API responses
- ✅ 80% warning threshold + 90% auto-compaction
- ✅ Real-time context display in header
- ✅ Event-driven architecture for extensibility

### Implementation Phases

**Phase 1: Token Usage Tracking** (~30 min)
- Added cumulative token tracking to SessionManager
- Implemented token calculation from session metadata
- Updated compression to use actual API token counts
- Added public `getTokenUsage()` getter

**Phase 2: Threshold Warnings** (~45 min)
- Modified `needsCompression()` to return usage % and warning flags
- Extended SessionManager with EventEmitter
- Implemented 3 events: `context-warning`, `auto-compacting`, `compaction-complete`
- Added UI event listeners with smart deduplication

**Phase 3: Context Display** (~30 min)
- Updated Header component with optional context stats
- Real-time header updates showing "Context: X/Y msgs (Z%)"
- Only displays when activeMessages > 0

### Code Changes

**Total**: ~105 lines across 4 files

| File | Changes | Lines |
|------|---------|-------|
| `src/session/manager.ts` | Token tracking, events, getters | +65 |
| `src/session/compression/engine.ts` | Threshold logic | +15 |
| `src/cli/components/Header.tsx` | Context stats display | +15 |
| `src/cli/components/App.tsx` | Event listeners, header stats | +40 |

### Key Design Decisions

1. **API Token Counts over Tokenizers**
   - Using actual usage from API responses instead of client-side estimation
   - More accurate, no external dependencies (tiktoken, etc.)
   - Falls back to 4:1 estimate if API doesn't provide usage

2. **Event-Driven Architecture**
   - SessionManager extends EventEmitter
   - Loosely coupled: compression engine doesn't need UI knowledge
   - Easy to add more listeners (logging, analytics, etc.)

3. **Smart Warning Deduplication**
   - Warning only shown once per session using `contextWarningShownRef`
   - Resets after compaction completes
   - Prevents spam during long conversations

### Testing Required

See "📋 Verification & Testing Required" section above for:
- Functional tests (80% warning, 90% auto-compact)
- Edge cases (session load, fork, persistence)
- Real-world usage validation

### References

Implementation plan: `STREAMING_IMPLEMENTATION_SUMMARY.md` (Phase 4 context management)
Related proposal: `0007-context-management.md` (this document)
