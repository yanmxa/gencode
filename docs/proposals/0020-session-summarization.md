# Proposal: Session Summarization

- **Proposal ID**: 0020
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement automatic session summarization to compress long conversations while preserving essential context. This enables unlimited conversation length through intelligent context management without hitting token limits.

## Motivation

LLM context windows are limited, causing issues with long conversations:

1. **Token limits**: Conversations get truncated or fail
2. **Cost increase**: Long contexts cost more per request
3. **Latency**: Large contexts slow response time
4. **Lost context**: Important early context gets dropped
5. **No continuity**: Can't resume long projects effectively

Automatic summarization maintains conversation quality indefinitely.

## Claude Code Reference

Claude Code mentions automatic summarization:

### From System Prompt
```
The conversation has unlimited context through automatic summarization.
```

### Expected Behavior
- Long conversations are automatically summarized
- Critical context (file changes, decisions) preserved
- Summaries replace older message blocks
- Agent maintains awareness of full conversation history
- Seamless experience for user

## Detailed Design

### API Design

```typescript
// src/session/summarization/types.ts
interface SummarizationConfig {
  enabled: boolean;
  trigger_threshold: number;       // Messages before summarizing
  target_compression: number;      // Target compression ratio (e.g., 0.3)
  preserve_recent: number;         // Keep N recent messages intact
  preserve_system: boolean;        // Always keep system prompts
  model?: string;                  // Model for summarization
}

interface ConversationSummary {
  id: string;
  covering_messages: [number, number];  // Range of summarized messages
  content: string;
  key_decisions: string[];
  files_modified: string[];
  tools_used: ToolUsageSummary[];
  generated_at: string;
  token_count: number;
}

interface ToolUsageSummary {
  tool: string;
  count: number;
  notable_uses: string[];
}

interface SummarizedSession {
  metadata: SessionMetadata;
  summaries: ConversationSummary[];
  recent_messages: Message[];
  full_message_count: number;
}
```

### Summarization Engine

```typescript
// src/session/summarization/engine.ts
class SummarizationEngine {
  private provider: LLMProvider;
  private config: SummarizationConfig;

  constructor(provider: LLMProvider, config?: Partial<SummarizationConfig>) {
    this.provider = provider;
    this.config = {
      enabled: true,
      trigger_threshold: 50,
      target_compression: 0.3,
      preserve_recent: 10,
      preserve_system: true,
      ...config
    };
  }

  async shouldSummarize(session: Session): Promise<boolean> {
    if (!this.config.enabled) return false;
    return session.messages.length > this.config.trigger_threshold;
  }

  async summarize(
    messages: Message[],
    range: [number, number]
  ): Promise<ConversationSummary> {
    const toSummarize = messages.slice(range[0], range[1] + 1);

    // Extract structured information
    const filesModified = this.extractFilesModified(toSummarize);
    const toolsUsed = this.extractToolUsage(toSummarize);
    const keyDecisions = await this.extractKeyDecisions(toSummarize);

    // Generate narrative summary
    const summaryContent = await this.generateSummary(toSummarize);

    return {
      id: generateId(),
      covering_messages: range,
      content: summaryContent,
      key_decisions: keyDecisions,
      files_modified: filesModified,
      tools_used: toolsUsed,
      generated_at: new Date().toISOString(),
      token_count: countTokens(summaryContent)
    };
  }

  private async generateSummary(messages: Message[]): Promise<string> {
    const prompt = `Summarize this conversation segment concisely.
Focus on:
1. What the user was trying to accomplish
2. What actions were taken (files changed, commands run)
3. Any important decisions or discoveries
4. Current state at the end of this segment

Keep the summary focused and technical. Use bullet points.

Conversation:
${this.formatMessages(messages)}

Summary:`;

    const response = await this.provider.complete({
      messages: [{ role: 'user', content: prompt }],
      max_tokens: 1000
    });

    return response.content[0].text;
  }

  private extractFilesModified(messages: Message[]): string[] {
    const files = new Set<string>();

    for (const msg of messages) {
      if (typeof msg.content !== 'string') {
        for (const block of msg.content) {
          if (block.type === 'tool_use') {
            if (['Write', 'Edit'].includes(block.name)) {
              files.add(block.input.file_path);
            }
          }
        }
      }
    }

    return Array.from(files);
  }

  private extractToolUsage(messages: Message[]): ToolUsageSummary[] {
    const toolStats = new Map<string, { count: number; uses: string[] }>();

    for (const msg of messages) {
      if (typeof msg.content !== 'string') {
        for (const block of msg.content) {
          if (block.type === 'tool_use') {
            const stats = toolStats.get(block.name) || { count: 0, uses: [] };
            stats.count++;
            if (stats.uses.length < 3) {
              stats.uses.push(summarizeToolUse(block));
            }
            toolStats.set(block.name, stats);
          }
        }
      }
    }

    return Array.from(toolStats.entries()).map(([tool, stats]) => ({
      tool,
      count: stats.count,
      notable_uses: stats.uses
    }));
  }

  private async extractKeyDecisions(messages: Message[]): Promise<string[]> {
    // Look for decision patterns in conversation
    const decisions: string[] = [];

    for (const msg of messages) {
      const content = typeof msg.content === 'string' ? msg.content : '';

      // Look for decision indicators
      if (content.includes('decided to') ||
          content.includes('chose to') ||
          content.includes('will use') ||
          content.includes('going with')) {
        decisions.push(this.extractDecisionContext(content));
      }
    }

    return decisions.slice(0, 5);  // Keep top 5
  }
}
```

### Session Manager Integration

```typescript
// Updated src/session/manager.ts
class SessionManager {
  private summarizer: SummarizationEngine;

  async addMessage(message: Message): Promise<void> {
    this.currentSession.messages.push(message);

    // Check if summarization needed
    if (await this.summarizer.shouldSummarize(this.currentSession)) {
      await this.performSummarization();
    }

    await this.save();
  }

  private async performSummarization(): Promise<void> {
    const session = this.currentSession;
    const preserve = this.summarizer.config.preserve_recent;

    // Determine range to summarize (keep recent messages)
    const summarizeEnd = session.messages.length - preserve - 1;
    const summarizeStart = this.getLastSummaryEnd() + 1;

    if (summarizeEnd <= summarizeStart) return;

    // Generate summary
    const summary = await this.summarizer.summarize(
      session.messages,
      [summarizeStart, summarizeEnd]
    );

    // Store summary and compress messages
    session.summaries = session.summaries || [];
    session.summaries.push(summary);

    // Replace summarized messages with placeholder
    const preserved = session.messages.slice(0, summarizeStart);
    const recent = session.messages.slice(summarizeEnd + 1);

    session.messages = [
      ...preserved,
      {
        role: 'system',
        content: `[Previous conversation summarized - ${summary.covering_messages[1] - summary.covering_messages[0] + 1} messages]\n\n${summary.content}`
      },
      ...recent
    ];

    session.full_message_count = session.full_message_count || session.messages.length;
  }

  getContextForLLM(): Message[] {
    const session = this.currentSession;

    // Build context with summaries
    const context: Message[] = [];

    // Add system prompt if exists
    const systemMsg = session.messages.find(m => m.role === 'system');
    if (systemMsg) context.push(systemMsg);

    // Add summary context
    if (session.summaries?.length) {
      context.push({
        role: 'system',
        content: this.formatSummariesForContext(session.summaries)
      });
    }

    // Add recent messages (skip system prompts already added)
    const recentMessages = session.messages.filter(m =>
      m.role !== 'system' || !m.content.includes('[Previous conversation')
    );
    context.push(...recentMessages);

    return context;
  }

  private formatSummariesForContext(summaries: ConversationSummary[]): string {
    let context = 'Previous conversation context:\n\n';

    for (const summary of summaries) {
      context += `--- Earlier in conversation ---\n`;
      context += summary.content + '\n';
      if (summary.files_modified.length) {
        context += `Files modified: ${summary.files_modified.join(', ')}\n`;
      }
      context += '\n';
    }

    return context;
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/session/summarization/types.ts` | Create | Type definitions |
| `src/session/summarization/engine.ts` | Create | Summarization logic |
| `src/session/summarization/index.ts` | Create | Module exports |
| `src/session/manager.ts` | Modify | Integrate summarization |
| `src/agent/agent.ts` | Modify | Use summarized context |

## User Experience

### Automatic Summarization (Invisible)
```
[After 50+ messages, automatically triggers...]

Agent: I notice this conversation is getting long. I've created a summary
of our earlier discussion to maintain context efficiently.

Summary created:
- Discussed authentication implementation
- Modified 5 files (auth.ts, login.tsx, ...)
- Decided to use JWT tokens with refresh mechanism
- Ran tests successfully

Recent conversation preserved. You can continue normally.
```

### Manual Summarization
```
User: /summarize

Summarizing conversation history...

┌─ Session Summary ─────────────────────────────────┐
│ Messages: 78 → 23 (70% reduction)                │
│ Tokens saved: ~15,000                             │
│                                                   │
│ Key Topics:                                       │
│ • Authentication system design                   │
│ • Database schema updates                        │
│ • Test coverage improvements                     │
│                                                   │
│ Files Modified: 12                                │
│ Commands Run: 34                                  │
│ Key Decisions: 5                                  │
└───────────────────────────────────────────────────┘
```

### Viewing Full History
```
User: /history --full

Session history (78 messages total):

[Summary 1] Messages 1-30:
  Initial project setup and architecture discussion...

[Summary 2] Messages 31-55:
  Authentication implementation and testing...

[Recent] Messages 56-78:
  Message 56: User asked about caching...
  Message 57: Agent suggested Redis implementation...
  ...
```

## Alternatives Considered

### Alternative 1: Simple Truncation
Just drop old messages.

**Pros**: Simple, predictable
**Cons**: Loses important context
**Decision**: Rejected - Context loss is unacceptable

### Alternative 2: User-Triggered Only
Only summarize when user requests.

**Pros**: User control
**Cons**: May hit limits unexpectedly
**Decision**: Rejected - Automatic is better UX

### Alternative 3: External Summarization Service
Use dedicated summarization API.

**Pros**: Optimized summaries
**Cons**: Additional dependency, cost
**Decision**: Deferred - Use same LLM for now

## Security Considerations

1. **Data Minimization**: Summaries should not include secrets
2. **Summary Storage**: Secure storage of summarized content
3. **Original Deletion**: Option to delete original messages after summary
4. **Audit Trail**: Keep record of what was summarized
5. **User Control**: Allow disabling summarization

## Testing Strategy

1. **Unit Tests**:
   - Summary generation
   - Context reconstruction
   - Token counting
   - Decision extraction

2. **Integration Tests**:
   - Long conversation handling
   - Summary quality validation
   - Context continuity

3. **Quality Tests**:
   - Agent understands summarized context
   - No important info lost
   - Coherent conversation flow

## Migration Path

1. **Phase 1**: Basic summarization engine
2. **Phase 2**: Automatic triggering
3. **Phase 3**: Structured extraction (files, decisions)
4. **Phase 4**: User controls and visibility
5. **Phase 5**: Quality improvements

Existing sessions remain unchanged until summarized.

## References

- [Claude Context Windows](https://docs.anthropic.com/claude/docs/context-windows)
- [Retrieval Augmented Generation](https://arxiv.org/abs/2005.11401)
- [Conversation Summarization Research](https://arxiv.org/abs/2106.00829)
