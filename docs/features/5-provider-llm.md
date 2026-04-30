# Feature 5: Model & LLM System

## Overview

Supports multiple LLM providers. The active provider and model are shown in the status bar and can be changed at runtime.

| Provider | Notes |
|----------|-------|
| Anthropic | API Key, Vertex AI, Amazon Bedrock |
| OpenAI | API Key |
| Google | API Key |
| MiniMax | API Key |
| Moonshot | API Key |
| Alibaba | API Key |

**Thinking efforts**:

Thinking/reasoning is configured as a provider-native effort string, not a global model-specific enum. The active provider determines which effort values are supported; implementations may accept the model ID for future refinement, but the default behavior assumes the provider's latest supported model family.

Effort definitions should follow the actual API compatibility layer, not the provider brand. For example, Moonshot uses OpenAI-compatible APIs and should reuse OpenAI reasoning efforts; MiniMax uses the Anthropic-compatible client and should reuse Anthropic thinking efforts.

| Provider | Efforts | Default | Notes |
|----------|---------|---------|-------|
| Anthropic | `off`, `think`, `think+`, `ultrathink` | `off` | Maps to Anthropic thinking budget tokens. |
| OpenAI | `none`, `low`, `medium`, `high`, `xhigh` | `medium` | Maps directly to OpenAI reasoning effort. |
| Moonshot | `none`, `low`, `medium`, `high`, `xhigh` | `medium` | Reuses OpenAI-compatible reasoning effort. |
| MiniMax | `off`, `think`, `think+`, `ultrathink` | `off` | Reuses Anthropic-compatible thinking effort. |
| Google | provider-defined effort strings | provider default | Maps to Google thinking/reasoning API parameters. |
| Alibaba | provider-defined effort strings | provider default | Maps to Alibaba reasoning API parameters. |

Anthropic-compatible budget mapping:

| Effort | Trigger | Budget tokens |
|--------|---------|---------------|
| `off` | — | 0 |
| `think` | `think` in prompt | 5,000 |
| `think+` | `think+` in prompt | 32,000 |
| `ultrathink` | `ultrathink` in prompt | 128,000 |

Provider reasoning options:

```go
type ReasoningOptions struct {
    Efforts []string
    Default string
}

type ReasoningProvider interface {
    ReasoningOptions(model string) ReasoningOptions
}
```

Define these in `internal/llm/provider.go`. Each provider implements `ReasoningOptions(model)` in its own `thinking.go`; keep `catalog.go` focused on model catalog/default-model metadata only.

The `model` argument is available for provider implementations that need it, but callers should treat efforts as provider-owned options. Current effort selection is session/UI state, not provider state, so provider clients should not expose get/set methods.

Runtime/session state:

```go
GetReasoningEffort(provider, model string) string
SetReasoningEffort(provider, model, effort string) error
CycleReasoningEffort(provider, model string, direction int) error
```

Implement session state on the runtime/TUI model that owns the active provider/model selection. `/think` and keyboard shortcuts use the same cycle/set logic and validate the selected value against the current provider's returned efforts.

UI behavior:

- `/think`: open or cycle the current reasoning effort using the provider's ordered effort list.
- `ctrl+t`: cycle to the next reasoning effort without opening a command.
- `ctrl+shift+t` or `alt+t`: cycle to the previous reasoning effort if supported by the terminal/key handling layer.
- If the provider returns no efforts, `/think` and shortcuts should no-op and show a short message such as `reasoning is not supported by this provider`.
- When switching provider or model, if the current effort is unsupported by the new provider, reset to that provider's default effort.
- Status bar should show the active reasoning effort when supported:
  - OpenAI-compatible providers: append to model name, for example `gpt-5.5 (medium)`.
  - Anthropic-compatible providers: show a compact thinking marker, for example `claude-sonnet-4 ✦ think+`.
  - Providers with no reasoning support: show no reasoning marker.
- After `/think` or a shortcut changes the effort, show transient feedback such as `reasoning: high` or `thinking: ultrathink`.
- Prompt keyword detection should use the provider's ordered effort list instead of hard-coded levels: `think` selects the first non-off effort, `think+` selects a high effort, and `ultrathink` selects the highest effort when available.

## UI Interactions

- **`/model`**: opens a tabbed picker overlay with Models and Providers tabs; arrow keys to navigate, Tab to switch, Enter to select.
- **`/search`**: opens a picker to select the search engine for web search.
- **`/think`**: cycles or selects reasoning/thinking effort; validates against the active provider's supported efforts.
- **Thinking shortcut**: `ctrl+t` cycles to the next reasoning effort without opening a command; `ctrl+shift+t` or `alt+t` cycles to the previous effort if supported by the terminal/key handling layer.
- **Status bar reasoning display**: shows the active effort when supported, for example `gpt-5.5 (medium)` for OpenAI-compatible providers or `claude-sonnet-4 ✦ think+` for Anthropic-compatible providers.
- **Streaming**: tokens appear in real time; a spinner indicates active streaming.
- **Thinking blocks**: `<thinking>` content is rendered in a collapsible block above the answer.

## Automated Tests

```bash
go test ./internal/llm/anthropic/... -v
go test ./internal/llm/moonshot/... -v
go test ./internal/llm/stream/... -v
go test ./internal/core/... -v
go test ./internal/llm/... -v
```

Covered:

```
# Streaming & response parsing
TestStateEmitsAndAccumulatesChunks         — stream chunk emission and accumulation
TestStateAddsToolCallsInStableOrder        — tool calls in stable order
TestStateEnsureToolUseStopReason           — stop reason for tool use
TestStateFailAndFinishEmitTerminalChunks   — terminal chunk emission
TestLoop_StreamChunks                      — stream chunk delivery in loop

# Tool ID sanitization (Anthropic)
TestToolIDSanitizer_ValidIDPassthrough     — valid IDs pass through
TestToolIDSanitizer_InvalidIDReplaced      — invalid IDs replaced
TestToolIDSanitizer_StableMapping          — same ID maps consistently
TestToolIDSanitizer_UniqueReplacements     — different IDs get unique replacements
TestToolIDSanitizer_ConsistentAcrossToolUseAndResult — tool_use and tool_result IDs match
TestToolIDSanitizer_NoAllocationForValidIDs — no wasteful allocation

# Message merging
TestMergeConsecutiveMessages_ToolResults   — multiple tool results merged
TestMergeConsecutiveMessages_NoConsecutive — non-consecutive pass through
TestMergeConsecutiveMessages_Empty         — empty input handled
TestMergeConsecutiveMessages_Single        — single message handled

# Moonshot
TestMoonshotAssistantMessagesIncludeReasoningContent — reasoning content included

# Client wrapper
TestClientSend                             — send request
TestClientStream                           — stream request
TestClientComplete                         — completion request
TestClientNameAndModelID                   — name and model ID
TestResolveMaxTokens_CustomOverride        — custom max token override
TestResolveMaxTokens_FromProvider          — max tokens from provider
TestResolveMaxTokens_Fallback              — fallback max tokens

# LLM loop
TestLoopInit                               — loop initialization
TestAddUser                                — add user message
TestAddResponse                            — add response
TestAddToolResult                          — add tool result
TestRunTransitions                         — loop state transitions
TestRunEndTurn                             — end turn handling
TestRunMaxTurns                            — max turns enforcement
TestRunCancelled                           — cancellation handling
TestLoop_TokenAccumulation                 — token counts accumulate across turns

# Thinking keyword detection
TestDetectThinkingKeywords                 — think/think+/ultrathink detection
```

Cases to add:

```go
func TestProvider_ModelListing(t *testing.T) {
    // ListModels must return a non-empty list for configured providers
}

func TestProvider_ThinkingBudget_SetCorrectly(t *testing.T) {
    // budget_tokens in the request must match the selected thinking level (5K/32K/128K)
}

func TestProvider_StreamChunk_OrderPreserved(t *testing.T) {
    // Chunks must arrive in order during streaming
}

func TestProvider_SwitchMidConversation(t *testing.T) {
    // Switching provider mid-conversation via /model must use new provider for next turn
}

func TestProvider_ModelSwitch_TakesEffectImmediately(t *testing.T) {
    // Model switch via /model must apply to the next LLM call
}

func TestProvider_ThinkingLevel_Persistence(t *testing.T) {
    // Thinking level must persist across turns within a session
}

func TestProvider_NonAnthropicThinking_Fallback(t *testing.T) {
    // Non-Anthropic providers must ignore thinking level gracefully
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_prov -x 220 -y 60
tmux send-keys -t t_prov 'gen' Enter
sleep 2

# Test 1: Switch model (Models tab)
tmux send-keys -t t_prov '/model' Enter
sleep 1
tmux capture-pane -t t_prov -p
# Expected: tabbed picker with Models tab showing available models

# Test 2: Switch provider (Providers tab)
# Press Tab to switch to Providers tab within /model overlay
tmux send-keys -t t_prov Tab
sleep 1
tmux capture-pane -t t_prov -p
# Expected: Providers tab showing available providers

# Test 3: Enable thinking
tmux send-keys -t t_prov '/think' Enter
sleep 1
# Select "normal"
tmux capture-pane -t t_prov -p
# Expected: thinking level cycles away from off for subsequent turns

# Test 4: Thinking block visible
tmux send-keys -t t_prov 'what is the sum of the first 100 prime numbers?' Enter
sleep 20
tmux capture-pane -t t_prov -p
# Expected: <thinking> block visible before the answer

# Test 5: Status bar shows provider and model
tmux capture-pane -t t_prov -p | tail -3
# Expected: current provider and model are visible in the footer/status area

# Test 6: Streaming tokens appear in real time
tmux send-keys -t t_prov 'write a short poem about the ocean' Enter
sleep 3
tmux capture-pane -t t_prov -p
# Expected: tokens streaming progressively; spinner visible during streaming

# Test 7: Model switch takes effect via /model
tmux send-keys -t t_prov '/model' Enter
sleep 1
# Select a different model from the Models tab
tmux send-keys -t t_prov Enter
sleep 1
tmux capture-pane -t t_prov -p | tail -3
# Expected: footer/status area updates to show the new model name

tmux kill-session -t t_prov
```
