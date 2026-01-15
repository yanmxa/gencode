# Proposal: Parallel Tool Execution

- **Proposal ID**: 0018
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement parallel tool execution to allow multiple independent tool calls to run concurrently within a single agent turn. This reduces latency and improves efficiency when the agent needs to perform multiple non-dependent operations.

## Motivation

Currently, mycode executes tools sequentially, one after another:

1. **High latency**: 3 file reads take 3x the time
2. **Wasted time**: Network requests wait for each other
3. **Poor utilization**: CPU/IO idle between calls
4. **Slow exploration**: Searching multiple directories is slow
5. **Suboptimal UX**: User waits longer than necessary

Parallel execution enables concurrent tool operations for faster results.

## Claude Code Reference

Claude Code supports parallel tool calls natively:

### From System Prompt
```
You can call multiple tools in a single response. When multiple independent
pieces of information are requested and all commands are likely to succeed,
run multiple tool calls in parallel for optimal performance.

However, if some tool calls depend on previous calls to inform dependent
values, do NOT call these tools in parallel and instead call them sequentially.
```

### Parallel Call Example
Multiple tool calls in single response:
```
Agent: I'll read both configuration files to understand the setup.
[Read: src/config.ts]
[Read: src/settings.json]
[Glob: **/*.test.ts]
```

All three execute concurrently, results return together.

### Key Rules
- Independent calls: Run in parallel
- Dependent calls: Run sequentially
- Never use placeholders for dependent values
- Agent decides based on data dependencies

## Detailed Design

### API Design

```typescript
// src/agent/parallel-executor.ts
interface ToolCall {
  id: string;
  name: string;
  input: unknown;
}

interface ToolResult {
  id: string;
  name: string;
  success: boolean;
  output: string;
  error?: string;
  duration_ms: number;
}

interface ParallelExecutionConfig {
  max_concurrent: number;      // Max concurrent executions
  timeout_per_tool: number;    // Per-tool timeout
  fail_fast: boolean;          // Stop on first error
  retry_failed: boolean;       // Retry failed tools once
}

const DEFAULT_CONFIG: ParallelExecutionConfig = {
  max_concurrent: 10,
  timeout_per_tool: 30000,
  fail_fast: false,
  retry_failed: false
};
```

### Implementation Approach

```typescript
// src/agent/parallel-executor.ts
class ParallelToolExecutor {
  private registry: ToolRegistry;
  private config: ParallelExecutionConfig;
  private semaphore: Semaphore;

  constructor(registry: ToolRegistry, config?: Partial<ParallelExecutionConfig>) {
    this.registry = registry;
    this.config = { ...DEFAULT_CONFIG, ...config };
    this.semaphore = new Semaphore(this.config.max_concurrent);
  }

  async executeParallel(
    calls: ToolCall[],
    context: ToolContext
  ): Promise<ToolResult[]> {
    const results: ToolResult[] = [];

    // Execute all calls concurrently with semaphore
    const promises = calls.map(call =>
      this.executeWithSemaphore(call, context)
    );

    // Wait for all (unless fail_fast)
    if (this.config.fail_fast) {
      const settled = await Promise.allSettled(promises);
      for (const result of settled) {
        if (result.status === 'fulfilled') {
          results.push(result.value);
          if (!result.value.success) break;
        }
      }
    } else {
      const allResults = await Promise.all(promises);
      results.push(...allResults);
    }

    return results;
  }

  private async executeWithSemaphore(
    call: ToolCall,
    context: ToolContext
  ): Promise<ToolResult> {
    await this.semaphore.acquire();

    try {
      return await this.executeSingle(call, context);
    } finally {
      this.semaphore.release();
    }
  }

  private async executeSingle(
    call: ToolCall,
    context: ToolContext
  ): Promise<ToolResult> {
    const start = Date.now();

    try {
      // Wrap with timeout
      const result = await Promise.race([
        this.registry.execute(call.name, call.input, context),
        this.timeout(this.config.timeout_per_tool)
      ]);

      return {
        id: call.id,
        name: call.name,
        success: result.success,
        output: result.output || '',
        error: result.error,
        duration_ms: Date.now() - start
      };
    } catch (error) {
      return {
        id: call.id,
        name: call.name,
        success: false,
        output: '',
        error: error instanceof Error ? error.message : 'Unknown error',
        duration_ms: Date.now() - start
      };
    }
  }

  private timeout(ms: number): Promise<never> {
    return new Promise((_, reject) =>
      setTimeout(() => reject(new Error('Tool execution timeout')), ms)
    );
  }
}
```

### Agent Loop Integration

```typescript
// Updated src/agent/agent.ts
class Agent {
  private parallelExecutor: ParallelToolExecutor;

  async *run(messages: Message[]): AsyncGenerator<AgentEvent> {
    while (true) {
      const response = await this.provider.complete({
        messages,
        tools: this.registry.getToolDefinitions()
      });

      // Yield text content
      for (const block of response.content) {
        if (block.type === 'text') {
          yield { type: 'text', content: block.text };
        }
      }

      // Check for tool calls
      const toolCalls = response.content.filter(b => b.type === 'tool_use');

      if (toolCalls.length === 0) {
        yield { type: 'done', reason: response.stopReason };
        return;
      }

      // Execute tools in parallel
      yield { type: 'tools_start', count: toolCalls.length };

      const results = await this.parallelExecutor.executeParallel(
        toolCalls.map(tc => ({
          id: tc.id,
          name: tc.name,
          input: tc.input
        })),
        { cwd: this.cwd, signal: this.signal }
      );

      // Yield individual results
      for (const result of results) {
        yield {
          type: 'tool_result',
          id: result.id,
          name: result.name,
          success: result.success,
          output: result.output,
          duration_ms: result.duration_ms
        };
      }

      // Add results to messages
      messages.push({
        role: 'assistant',
        content: response.content
      });
      messages.push({
        role: 'user',
        content: results.map(r => ({
          type: 'tool_result',
          tool_use_id: r.id,
          content: r.success ? r.output : `Error: ${r.error}`
        }))
      });
    }
  }
}
```

### Semaphore Implementation

```typescript
// src/utils/semaphore.ts
class Semaphore {
  private permits: number;
  private waiting: (() => void)[] = [];

  constructor(permits: number) {
    this.permits = permits;
  }

  async acquire(): Promise<void> {
    if (this.permits > 0) {
      this.permits--;
      return;
    }

    return new Promise(resolve => {
      this.waiting.push(resolve);
    });
  }

  release(): void {
    if (this.waiting.length > 0) {
      const next = this.waiting.shift()!;
      next();
    } else {
      this.permits++;
    }
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/agent/parallel-executor.ts` | Create | Parallel execution engine |
| `src/agent/agent.ts` | Modify | Integrate parallel execution |
| `src/utils/semaphore.ts` | Create | Concurrency control |
| `src/agent/types.ts` | Modify | Add parallel-related events |

## User Experience

### Parallel Execution Display
```
Agent: I'll check multiple files to understand the project structure.

┌─ Parallel Tool Execution (4 calls) ───────────────┐
│ [1/4] Read: src/index.ts           ✓ (45ms)      │
│ [2/4] Read: package.json           ✓ (32ms)      │
│ [3/4] Glob: src/**/*.ts            ✓ (78ms)      │
│ [4/4] Grep: "TODO"                 ✓ (156ms)     │
├───────────────────────────────────────────────────┤
│ Total: 4/4 succeeded in 156ms (parallel)          │
│ Sequential would have taken: 311ms                │
└───────────────────────────────────────────────────┘
```

### Mixed Results
```
┌─ Parallel Tool Execution (3 calls) ───────────────┐
│ [1/3] Read: src/config.ts          ✓ (38ms)      │
│ [2/3] Read: missing.ts             ✗ (12ms)      │
│       Error: File not found                       │
│ [3/3] Bash: npm --version          ✓ (234ms)     │
├───────────────────────────────────────────────────┤
│ Total: 2/3 succeeded, 1 failed                    │
└───────────────────────────────────────────────────┘
```

### Sequential Fallback
```
Agent: These operations depend on each other, so I'll run them sequentially.

[Read: package.json]
  → Found dependencies list

[Bash: npm install lodash]
  → Using package info from previous read
```

## Alternatives Considered

### Alternative 1: Always Sequential
Keep current sequential behavior.

**Pros**: Simpler, predictable
**Cons**: Slow, wastes time
**Decision**: Rejected - Performance is important

### Alternative 2: Unlimited Parallelism
No concurrency limits.

**Pros**: Maximum speed
**Cons**: Resource exhaustion, rate limiting
**Decision**: Rejected - Need semaphore control

### Alternative 3: Provider-Side Parallelism
Let LLM provider handle parallel calls.

**Pros**: Simpler client
**Cons**: Not all providers support it
**Decision**: Rejected - Need client-side control

## Security Considerations

1. **Resource Limits**: Cap concurrent operations
2. **Timeout Enforcement**: Per-tool and total timeouts
3. **Memory Management**: Stream large outputs
4. **Rate Limiting**: Respect external API limits
5. **Cancellation**: Support AbortSignal propagation

```typescript
const SAFETY_LIMITS = {
  maxConcurrent: 10,
  maxTotalCalls: 50,
  perToolTimeout: 30000,
  totalTimeout: 120000
};
```

## Testing Strategy

1. **Unit Tests**:
   - Semaphore behavior
   - Parallel vs sequential timing
   - Error handling
   - Timeout enforcement

2. **Integration Tests**:
   - Multiple tool types
   - Mixed success/failure
   - Resource cleanup

3. **Performance Tests**:
   - Speedup measurement
   - Memory usage
   - Concurrent limits

## Migration Path

1. **Phase 1**: ParallelToolExecutor with semaphore
2. **Phase 2**: Agent loop integration
3. **Phase 3**: CLI display updates
4. **Phase 4**: Configuration options
5. **Phase 5**: Performance telemetry

No breaking changes - sequential behavior preserved as default.

## References

- [Promise.all() - MDN](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Promise/all)
- [Semaphore Pattern](https://en.wikipedia.org/wiki/Semaphore_(programming))
- [Claude Code Tool Calling](https://code.claude.com/docs/en/tools)
