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

**Thinking levels** (Anthropic only):

| Level | Trigger | Budget tokens |
|-------|---------|---------------|
| Off | — | 0 |
| Normal | `think` in prompt | 5,000 |
| High | `think+` in prompt | 32,000 |
| Ultra | `ultrathink` in prompt | 128,000 |

## UI Interactions

- **`/model`**: opens a tabbed picker overlay with Models and Providers tabs; arrow keys to navigate, Tab to switch, Enter to select.
- **`/search`**: opens a picker to select the search engine for web search.
- **`/think`**: cycles thinking level; status bar updates with the current level.
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
