# Feature 18: Cost / Token Tracking

## Overview

Token usage is tracked per turn and accumulated across the session. Cost is calculated based on the active model's pricing.

- **Per turn:** input tokens, output tokens
- **Session total:** cumulative across all turns
- **Display:** status bar shows running totals
- **Pricing:** model-aware; updates when the model changes

## UI Interactions

- **Status bar**: shows `in: N / out: N / $X.XX` after each turn.
- **`/tokenlimit`**: shows current usage and the model's context limit in a popup.
- **Auto-compact warning**: a notice appears when usage exceeds 80% of the limit.

## Automated Tests

```bash
go test ./internal/provider/... -v -run TestTokenUsage
go test ./internal/client/... -v -run TestCostTracking
```

Cases to add:

```go
func TestCost_PerTurnAccumulation(t *testing.T) {
    // Token counts must accumulate correctly across multiple turns
}

func TestCost_SessionTotal_MatchesSumOfTurns(t *testing.T) {
    // Session total must equal the sum of all per-turn counts
}

func TestCost_AnthropicPricing_CalculatedCorrectly(t *testing.T) {
    // Cost in USD must reflect current Anthropic model pricing
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cost -x 220 -y 60
tmux send-keys -t t_cost 'gen' Enter
sleep 2

# Send a message and inspect the status bar
tmux send-keys -t t_cost 'what is 2+2?' Enter
sleep 6
tmux capture-pane -t t_cost -p
# Expected: status bar updates with input/output token counts

# View token limit
tmux send-keys -t t_cost '/tokenlimit' Enter
sleep 2
tmux capture-pane -t t_cost -p
# Expected: current usage and context limit shown

# Accumulate across turns
for i in {1..3}; do
  tmux send-keys -t t_cost "question $i: give me a short fact" Enter
  sleep 6
done
tmux capture-pane -t t_cost -p
# Expected: token count in status bar increases after each turn

tmux kill-session -t t_cost
```
