# Feature 5: Provider / LLM System

## Overview

Supports multiple LLM providers. The active provider and model are shown in the status bar and can be changed at runtime.

| Provider | Notes |
|----------|-------|
| Anthropic | API Key, Vertex AI, Amazon Bedrock |
| OpenAI | API Key |
| Google | API Key |
| Moonshot | API Key |
| Alibaba | API Key |

**Thinking levels** (Anthropic only):

| Level | Trigger | Budget tokens |
|-------|---------|---------------|
| Off | — | 0 |
| Normal | `think` in prompt | 5,000 |
| High | `think+` in prompt | 32,000 |
| Ultra | `ultrathink` in prompt | 128,000 |

## UI Interactions

- **`/provider`**: opens a picker overlay; arrow keys to navigate, Enter to select.
- **`/model`**: shows models for the current provider; selection takes effect immediately.
- **`/think`**: cycles thinking level; status bar updates with the current level.
- **Streaming**: tokens appear in real time; a spinner indicates active streaming.
- **Thinking blocks**: `<thinking>` content is rendered in a collapsible block above the answer.

## Automated Tests

```bash
go test ./internal/provider/anthropic/... -v
go test ./internal/provider/moonshot/... -v
go test ./internal/provider/streamutil/... -v
go test ./internal/core/... -v
go test ./internal/client/... -v
```

Covered:

```
internal/provider/anthropic/client_test.go  — streaming response parsing
internal/core/core_test.go                  — LLM loop: request → tool call → continue
internal/client/client_test.go              — client wrapper behavior
```

Cases to add:

```go
func TestProvider_ModelListing(t *testing.T) {
    // ListModels must return a non-empty list for configured providers
}

func TestProvider_ThinkingBudget_SetCorrectly(t *testing.T) {
    // budget_tokens in the request must match the selected thinking level
}

func TestProvider_StreamChunk_OrderPreserved(t *testing.T) {
    // Chunks must arrive in order
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_prov -x 220 -y 60
tmux send-keys -t t_prov 'gen' Enter
sleep 2

# Switch provider
tmux send-keys -t t_prov '/provider' Enter
sleep 1
tmux capture-pane -t t_prov -p
# Expected: provider list; select one with arrow keys + Enter

# Switch model
tmux send-keys -t t_prov '/model' Enter
sleep 1
tmux capture-pane -t t_prov -p
# Expected: model list for current provider

# Enable thinking
tmux send-keys -t t_prov '/think' Enter
sleep 1
# Select "normal"
tmux capture-pane -t t_prov -p
# Expected: status bar shows thinking is on

tmux send-keys -t t_prov 'what is the sum of the first 100 prime numbers?' Enter
sleep 20
tmux capture-pane -t t_prov -p
# Expected: <thinking> block visible before the answer

tmux kill-session -t t_prov
```
