# Proposal: Cost Tracking

- **Proposal ID**: 0025
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement comprehensive cost tracking for API usage, showing real-time token counts, estimated costs, and cumulative spending. This provides users with visibility and control over their LLM API expenses.

## Motivation

Currently, mycode provides no cost visibility:

1. **Surprise bills**: Users don't know costs until invoice
2. **No optimization**: Can't identify expensive operations
3. **No budgets**: Can't set spending limits
4. **Hidden usage**: Token counts not visible
5. **No comparison**: Can't compare provider costs

Cost tracking enables informed usage decisions.

## Claude Code Reference

Claude Code displays cost information in the interface:

### Observed Features
- Token count display (input/output)
- Estimated cost per message
- Session totals
- Model-specific pricing

### Cost Visibility
```
Tokens: 1,234 in / 567 out (~$0.02)
Session total: 45,678 tokens (~$0.58)
```

## Detailed Design

### API Design

```typescript
// src/costs/types.ts
interface TokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
}

interface CostEstimate {
  inputCost: number;
  outputCost: number;
  totalCost: number;
  currency: string;  // 'USD'
}

interface MessageCost {
  messageId: string;
  timestamp: Date;
  model: string;
  provider: string;
  tokens: TokenUsage;
  cost: CostEstimate;
}

interface SessionCost {
  sessionId: string;
  messages: MessageCost[];
  totals: {
    tokens: TokenUsage;
    cost: CostEstimate;
  };
}

interface CostConfig {
  displayMode: 'always' | 'summary' | 'never';
  currency: string;
  budgets?: {
    perMessage?: number;
    perSession?: number;
    daily?: number;
    monthly?: number;
  };
  alerts?: {
    threshold: number;
    action: 'warn' | 'confirm' | 'block';
  };
}

// Provider pricing (per 1M tokens)
interface ProviderPricing {
  provider: string;
  model: string;
  inputPer1M: number;
  outputPer1M: number;
  effectiveDate: string;
}
```

### Cost Tracker

```typescript
// src/costs/tracker.ts
class CostTracker {
  private config: CostConfig;
  private pricing: Map<string, ProviderPricing> = new Map();
  private sessionCosts: Map<string, SessionCost> = new Map();
  private dailyTotal: number = 0;
  private monthlyTotal: number = 0;

  constructor(config?: Partial<CostConfig>) {
    this.config = {
      displayMode: 'always',
      currency: 'USD',
      ...config
    };
    this.loadPricing();
    this.loadHistory();
  }

  private loadPricing(): void {
    // Current pricing as of 2025
    const defaultPricing: ProviderPricing[] = [
      // Anthropic
      { provider: 'anthropic', model: 'claude-opus-4-5', inputPer1M: 15, outputPer1M: 75, effectiveDate: '2025-01-01' },
      { provider: 'anthropic', model: 'claude-sonnet-4', inputPer1M: 3, outputPer1M: 15, effectiveDate: '2025-01-01' },
      { provider: 'anthropic', model: 'claude-haiku-3-5', inputPer1M: 0.25, outputPer1M: 1.25, effectiveDate: '2025-01-01' },

      // OpenAI
      { provider: 'openai', model: 'gpt-4o', inputPer1M: 2.5, outputPer1M: 10, effectiveDate: '2025-01-01' },
      { provider: 'openai', model: 'gpt-4-turbo', inputPer1M: 10, outputPer1M: 30, effectiveDate: '2025-01-01' },
      { provider: 'openai', model: 'o1', inputPer1M: 15, outputPer1M: 60, effectiveDate: '2025-01-01' },

      // Google
      { provider: 'gemini', model: 'gemini-2.0-flash', inputPer1M: 0.075, outputPer1M: 0.30, effectiveDate: '2025-01-01' },
      { provider: 'gemini', model: 'gemini-1.5-pro', inputPer1M: 1.25, outputPer1M: 5, effectiveDate: '2025-01-01' },
    ];

    for (const pricing of defaultPricing) {
      this.pricing.set(`${pricing.provider}:${pricing.model}`, pricing);
    }
  }

  recordUsage(
    sessionId: string,
    model: string,
    provider: string,
    tokens: TokenUsage
  ): MessageCost {
    const cost = this.calculateCost(provider, model, tokens);

    const messageCost: MessageCost = {
      messageId: generateId(),
      timestamp: new Date(),
      model,
      provider,
      tokens,
      cost
    };

    // Update session costs
    const session = this.sessionCosts.get(sessionId) || {
      sessionId,
      messages: [],
      totals: { tokens: { inputTokens: 0, outputTokens: 0, totalTokens: 0 }, cost: { inputCost: 0, outputCost: 0, totalCost: 0, currency: 'USD' } }
    };

    session.messages.push(messageCost);
    session.totals.tokens.inputTokens += tokens.inputTokens;
    session.totals.tokens.outputTokens += tokens.outputTokens;
    session.totals.tokens.totalTokens += tokens.totalTokens;
    session.totals.cost.inputCost += cost.inputCost;
    session.totals.cost.outputCost += cost.outputCost;
    session.totals.cost.totalCost += cost.totalCost;

    this.sessionCosts.set(sessionId, session);

    // Update daily/monthly
    this.dailyTotal += cost.totalCost;
    this.monthlyTotal += cost.totalCost;

    // Check budgets
    this.checkBudgets(messageCost);

    return messageCost;
  }

  private calculateCost(
    provider: string,
    model: string,
    tokens: TokenUsage
  ): CostEstimate {
    const pricing = this.pricing.get(`${provider}:${model}`);

    if (!pricing) {
      // Default/unknown pricing
      return {
        inputCost: 0,
        outputCost: 0,
        totalCost: 0,
        currency: 'USD'
      };
    }

    const inputCost = (tokens.inputTokens / 1_000_000) * pricing.inputPer1M;
    const outputCost = (tokens.outputTokens / 1_000_000) * pricing.outputPer1M;

    return {
      inputCost,
      outputCost,
      totalCost: inputCost + outputCost,
      currency: 'USD'
    };
  }

  private checkBudgets(cost: MessageCost): void {
    const budgets = this.config.budgets;
    if (!budgets) return;

    const alerts = this.config.alerts;

    if (budgets.perMessage && cost.cost.totalCost > budgets.perMessage) {
      this.handleBudgetAlert('perMessage', cost.cost.totalCost, budgets.perMessage);
    }

    if (budgets.daily && this.dailyTotal > budgets.daily) {
      this.handleBudgetAlert('daily', this.dailyTotal, budgets.daily);
    }

    if (budgets.monthly && this.monthlyTotal > budgets.monthly) {
      this.handleBudgetAlert('monthly', this.monthlyTotal, budgets.monthly);
    }
  }

  getSessionCost(sessionId: string): SessionCost | undefined {
    return this.sessionCosts.get(sessionId);
  }

  formatCost(cost: CostEstimate): string {
    if (cost.totalCost < 0.01) {
      return `<$0.01`;
    }
    return `$${cost.totalCost.toFixed(2)}`;
  }

  formatTokens(tokens: TokenUsage): string {
    return `${this.formatNumber(tokens.inputTokens)} in / ${this.formatNumber(tokens.outputTokens)} out`;
  }

  private formatNumber(n: number): string {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
    return n.toString();
  }

  getReport(period: 'session' | 'day' | 'month'): CostReport {
    // Generate detailed cost report
    return {
      period,
      totalCost: period === 'session' ? this.getCurrentSessionCost() : this.getPeriodCost(period),
      breakdown: this.getBreakdown(period),
      topOperations: this.getTopOperations(period)
    };
  }
}

export const costTracker = new CostTracker();
```

### Provider Integration

```typescript
// Update to src/providers/types.ts
interface CompletionResponse {
  content: ContentBlock[];
  stopReason: string;
  usage?: {
    inputTokens: number;
    outputTokens: number;
  };
}

// Update agent to track costs
class Agent {
  async *run(messages: Message[]): AsyncGenerator<AgentEvent> {
    // ... existing code ...

    const response = await this.provider.complete({ messages, tools });

    // Track cost if usage available
    if (response.usage) {
      const cost = costTracker.recordUsage(
        this.sessionId,
        this.model,
        this.provider.name,
        {
          inputTokens: response.usage.inputTokens,
          outputTokens: response.usage.outputTokens,
          totalTokens: response.usage.inputTokens + response.usage.outputTokens
        }
      );

      yield { type: 'cost', cost };
    }
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/costs/types.ts` | Create | Type definitions |
| `src/costs/tracker.ts` | Create | Cost tracking logic |
| `src/costs/pricing.ts` | Create | Provider pricing data |
| `src/costs/reports.ts` | Create | Cost reporting |
| `src/costs/index.ts` | Create | Module exports |
| `src/agent/agent.ts` | Modify | Track costs |
| `src/providers/types.ts` | Modify | Add usage to response |
| `src/cli/ui.ts` | Modify | Display costs |

## User Experience

### Real-time Display
```
┌─ mycode ──────────────────────────────────────────────────┐
│ Agent: Here's the implementation for the auth module...  │
│                                                          │
│ [Edit: src/auth.ts]                                      │
│ [Write: src/auth.test.ts]                                │
│                                                          │
├──────────────────────────────────────────────────────────┤
│ Tokens: 2.3K in / 1.8K out (~$0.04)  Session: $0.23     │
└──────────────────────────────────────────────────────────┘
```

### Cost Report
```
User: /costs

Cost Report - Current Session:
┌────────────────────────────────────────────────────────────┐
│ Provider    Model             Messages  Tokens    Cost    │
├────────────────────────────────────────────────────────────┤
│ anthropic   claude-sonnet-4   12        45.2K     $0.23   │
│ anthropic   claude-haiku      3         8.1K      $0.01   │
├────────────────────────────────────────────────────────────┤
│ Total                         15        53.3K     $0.24   │
└────────────────────────────────────────────────────────────┘

Daily: $1.45 / $10.00 budget (14.5%)
Monthly: $23.67 / $100.00 budget (23.7%)
```

### Budget Warning
```
⚠️ Budget Alert
Daily spending ($9.50) approaching limit ($10.00).
Continue? [y/N]
```

### Compare Providers
```
User: /costs compare

Cost Comparison for Current Session (53.3K tokens):
┌────────────────────────────────────────────────────────────┐
│ Provider     Model                 Input    Output   Total│
├────────────────────────────────────────────────────────────┤
│ gemini       gemini-2.0-flash      $0.00    $0.01   $0.01 │
│ anthropic    claude-haiku          $0.01    $0.05   $0.06 │
│ openai       gpt-4o                $0.10    $0.40   $0.50 │
│ anthropic    claude-sonnet         $0.13    $0.68   $0.81 │
│ anthropic    claude-opus           $0.67    $3.38   $4.05 │
└────────────────────────────────────────────────────────────┘

Current selection: claude-sonnet ($0.24 actual)
```

## Alternatives Considered

### Alternative 1: Provider Dashboard Only
Rely on provider's usage dashboards.

**Pros**: Accurate, authoritative
**Cons**: Not real-time, not in-context
**Decision**: Rejected - Need inline visibility

### Alternative 2: Token Count Only
Show tokens without cost estimates.

**Pros**: Simpler, always accurate
**Cons**: Users must calculate costs
**Decision**: Rejected - Cost is more actionable

### Alternative 3: Post-hoc Reports Only
Only show costs after session ends.

**Pros**: Simpler implementation
**Cons**: No real-time awareness
**Decision**: Rejected - Real-time is essential

## Security Considerations

1. **No API Keys in Reports**: Don't expose credentials
2. **Local Storage**: Cost data stored locally
3. **Pricing Updates**: Verify pricing source
4. **Budget Enforcement**: Optional hard limits
5. **Export Encryption**: Protect exported reports

## Testing Strategy

1. **Unit Tests**:
   - Cost calculation accuracy
   - Token counting
   - Budget checking
   - Report generation

2. **Integration Tests**:
   - Provider usage extraction
   - Real-time updates
   - Multiple sessions

3. **Manual Testing**:
   - Various providers
   - Display modes
   - Budget scenarios

## Migration Path

1. **Phase 1**: Basic token/cost display
2. **Phase 2**: Session totals
3. **Phase 3**: Budget system
4. **Phase 4**: Detailed reports
5. **Phase 5**: Cost optimization suggestions

No breaking changes.

## References

- [Anthropic Pricing](https://www.anthropic.com/pricing)
- [OpenAI Pricing](https://openai.com/pricing)
- [Google AI Pricing](https://ai.google.dev/pricing)
- [Token Counting Libraries](https://github.com/openai/tiktoken)
