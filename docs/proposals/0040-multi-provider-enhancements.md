# Proposal: Multi-Provider Enhancements

- **Proposal ID**: 0040
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Enhance the multi-provider system with unified tool calling, provider fallbacks, automatic model selection, and cross-provider consistency for seamless switching between LLM providers.

## Motivation

Current multi-provider support is basic:

1. **Inconsistent tool calling**: Different formats per provider
2. **No fallbacks**: Single point of failure
3. **Manual selection**: User must choose provider
4. **Feature gaps**: Not all features on all providers
5. **No cost optimization**: Can't auto-select cheapest

Enhanced multi-provider enables robust, flexible LLM usage.

## Detailed Design

### API Design

```typescript
// src/providers/types.ts
interface ProviderConfig {
  name: string;
  enabled: boolean;
  priority: number;           // Lower = higher priority
  models: ModelConfig[];
  rateLimits?: RateLimitConfig;
  fallbackTo?: string;        // Fallback provider
}

interface ModelConfig {
  id: string;
  aliases: string[];
  capabilities: ModelCapability[];
  contextWindow: number;
  maxOutputTokens: number;
  inputPricePer1M: number;
  outputPricePer1M: number;
}

type ModelCapability =
  | 'text'
  | 'vision'
  | 'tools'
  | 'streaming'
  | 'thinking'
  | 'json_mode';

interface ProviderSelector {
  strategy: 'priority' | 'round_robin' | 'cost_optimized' | 'latency_optimized';
  requirements?: {
    capabilities?: ModelCapability[];
    minContextWindow?: number;
    maxCostPer1K?: number;
  };
}

interface ProviderResult<T> {
  provider: string;
  model: string;
  result: T;
  usage?: TokenUsage;
  latency: number;
  fallbackUsed: boolean;
}
```

### Unified Provider Manager

```typescript
// src/providers/manager.ts
class ProviderManager {
  private providers: Map<string, LLMProvider> = new Map();
  private configs: Map<string, ProviderConfig> = new Map();
  private healthStatus: Map<string, ProviderHealth> = new Map();

  constructor() {
    this.loadProviders();
    this.startHealthChecks();
  }

  async complete(
    request: CompletionRequest,
    selector?: ProviderSelector
  ): Promise<ProviderResult<CompletionResponse>> {
    const provider = await this.selectProvider(request, selector);

    const start = Date.now();
    let fallbackUsed = false;

    try {
      const result = await provider.complete(request);
      return {
        provider: provider.name,
        model: request.model,
        result,
        usage: result.usage,
        latency: Date.now() - start,
        fallbackUsed
      };
    } catch (error) {
      // Try fallback
      const fallback = await this.getFallback(provider.name, request);
      if (fallback) {
        fallbackUsed = true;
        const result = await fallback.complete(request);
        return {
          provider: fallback.name,
          model: request.model,
          result,
          usage: result.usage,
          latency: Date.now() - start,
          fallbackUsed
        };
      }
      throw error;
    }
  }

  private async selectProvider(
    request: CompletionRequest,
    selector?: ProviderSelector
  ): Promise<LLMProvider> {
    const strategy = selector?.strategy || 'priority';
    const requirements = selector?.requirements || {};

    // Get eligible providers
    const eligible = Array.from(this.providers.entries())
      .filter(([name, provider]) => {
        const config = this.configs.get(name);
        if (!config?.enabled) return false;

        // Check health
        const health = this.healthStatus.get(name);
        if (health?.status === 'unhealthy') return false;

        // Check capabilities
        if (requirements.capabilities) {
          const model = this.findModel(config, request.model);
          if (!model) return false;
          const hasAll = requirements.capabilities.every(c =>
            model.capabilities.includes(c)
          );
          if (!hasAll) return false;
        }

        return true;
      });

    if (eligible.length === 0) {
      throw new Error('No eligible providers available');
    }

    // Select based on strategy
    switch (strategy) {
      case 'priority':
        eligible.sort((a, b) => {
          const priorityA = this.configs.get(a[0])?.priority || 999;
          const priorityB = this.configs.get(b[0])?.priority || 999;
          return priorityA - priorityB;
        });
        return eligible[0][1];

      case 'cost_optimized':
        return this.selectCheapest(eligible, request);

      case 'latency_optimized':
        return this.selectFastest(eligible);

      case 'round_robin':
        return this.selectRoundRobin(eligible);

      default:
        return eligible[0][1];
    }
  }

  private selectCheapest(
    providers: [string, LLMProvider][],
    request: CompletionRequest
  ): LLMProvider {
    const estimatedTokens = this.estimateTokens(request);

    let cheapest: LLMProvider | null = null;
    let lowestCost = Infinity;

    for (const [name, provider] of providers) {
      const config = this.configs.get(name);
      const model = this.findModel(config!, request.model);
      if (!model) continue;

      const cost = (estimatedTokens.input * model.inputPricePer1M / 1_000_000) +
                   (estimatedTokens.output * model.outputPricePer1M / 1_000_000);

      if (cost < lowestCost) {
        lowestCost = cost;
        cheapest = provider;
      }
    }

    return cheapest || providers[0][1];
  }

  // Tool calling normalization
  normalizeToolCall(call: ProviderToolCall): UnifiedToolCall {
    // Convert provider-specific format to unified format
    return {
      id: call.id || generateId(),
      name: call.name || call.function?.name,
      input: call.input || JSON.parse(call.function?.arguments || '{}')
    };
  }

  normalizeToolResult(result: ToolResult, provider: string): ProviderToolResult {
    // Convert unified result to provider-specific format
    switch (provider) {
      case 'anthropic':
        return {
          type: 'tool_result',
          tool_use_id: result.id,
          content: result.success ? result.output : `Error: ${result.error}`
        };

      case 'openai':
        return {
          role: 'tool',
          tool_call_id: result.id,
          content: result.success ? result.output : `Error: ${result.error}`
        };

      case 'gemini':
        return {
          functionResponse: {
            name: result.name,
            response: { result: result.success ? result.output : result.error }
          }
        };

      default:
        throw new Error(`Unknown provider: ${provider}`);
    }
  }

  // Health monitoring
  private startHealthChecks(): void {
    setInterval(async () => {
      for (const [name, provider] of this.providers) {
        try {
          const start = Date.now();
          await provider.complete({
            messages: [{ role: 'user', content: 'ping' }],
            maxTokens: 5
          });
          this.healthStatus.set(name, {
            status: 'healthy',
            latency: Date.now() - start,
            lastCheck: new Date()
          });
        } catch (error) {
          const current = this.healthStatus.get(name);
          this.healthStatus.set(name, {
            status: current?.status === 'degraded' ? 'unhealthy' : 'degraded',
            error: error instanceof Error ? error.message : 'Unknown error',
            lastCheck: new Date()
          });
        }
      }
    }, 60000);  // Check every minute
  }
}

export const providerManager = new ProviderManager();
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/providers/types.ts` | Modify | Enhanced types |
| `src/providers/manager.ts` | Create | Provider management |
| `src/providers/selector.ts` | Create | Selection strategies |
| `src/providers/normalizer.ts` | Create | Format normalization |
| `src/providers/health.ts` | Create | Health monitoring |

## User Experience

### Provider Status
```
User: /providers

Provider Status:
┌────────────────────────────────────────────────────────────┐
│ Provider   Status    Latency   Models            Priority │
├────────────────────────────────────────────────────────────┤
│ anthropic  ● Healthy  120ms    claude-*               1   │
│ openai     ● Healthy  95ms     gpt-4*, o1             2   │
│ gemini     ○ Degraded 450ms    gemini-*               3   │
└────────────────────────────────────────────────────────────┘

Active: anthropic (claude-sonnet-4)
Fallback: openai (gpt-4o)
```

### Automatic Fallback
```
Agent: Attempting to connect to anthropic...
⚠️ anthropic unavailable, falling back to openai

Using: openai/gpt-4o
```

### Cost-Optimized Selection
```
User: /settings provider strategy cost_optimized

Provider strategy set to: Cost Optimized

For your typical usage:
• Small queries → gemini-2.0-flash ($0.0001/1K)
• Complex tasks → claude-sonnet ($0.003/1K)
• Vision tasks → gpt-4o ($0.005/1K)
```

### Model Comparison
```
User: /compare-models "Explain async/await"

Model Comparison:
┌────────────────────────────────────────────────────────────┐
│ Model                  Tokens   Cost     Latency  Quality │
├────────────────────────────────────────────────────────────┤
│ claude-opus-4-5       1,234    $0.093   2.3s     ★★★★★   │
│ claude-sonnet-4         892    $0.027   1.1s     ★★★★☆   │
│ gpt-4o                  756    $0.019   0.9s     ★★★★☆   │
│ gemini-2.0-flash        645    $0.002   0.4s     ★★★☆☆   │
└────────────────────────────────────────────────────────────┘
```

## Security Considerations

1. API key isolation per provider
2. Secure credential storage
3. No cross-provider data leakage
4. Rate limit enforcement
5. Audit logging per provider

## Migration Path

1. **Phase 1**: Unified tool calling
2. **Phase 2**: Provider fallbacks
3. **Phase 3**: Selection strategies
4. **Phase 4**: Health monitoring
5. **Phase 5**: Cost optimization

## References

- [Anthropic API](https://docs.anthropic.com/claude/reference)
- [OpenAI API](https://platform.openai.com/docs/api-reference)
- [Google AI API](https://ai.google.dev/docs)
- [LiteLLM](https://github.com/BerriAI/litellm) - Multi-provider abstraction
