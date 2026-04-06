# Feature 15: Compact / Conversation Compression

## Overview

Compaction summarises old messages to free up context window space. It can be triggered manually or fires automatically when the token count approaches the model's limit.

- **Manual trigger:** `/compact` slash command
- **Focus hint:** `/compact <focus>` biases the generated summary
- **Auto trigger:** when context usage exceeds the threshold
- **Effect:** old messages are replaced by a summary; recent turns are preserved
- **Hooks:** `PreCompact` and `PostCompact` fire around each compaction

## UI Interactions

- **`/compact`**: immediately triggers compaction; a summary is shown as a notice message in the conversation.
- **Auto-compact notice**: a system notice appears when auto-compaction fires, showing how many messages were compressed.
- **After compact**: conversation continues normally; the LLM receives the summary as context.

## Automated Tests

```bash
go test ./tests/integration/compact/... -v
```

Covered:

```
# Integration tests
TestCompact_SummarizesConversation    — compaction produces summary
TestCompact_WithFocus                 — compaction with focus topic
TestCompact_EmptyConversation         — empty conversation handled gracefully
TestCompact_WithSessionMemory         — session memory preserved across compact

# Compaction threshold
TestNeedsCompaction                   — threshold detection for auto-compact

# Request building
TestBuildCompactRequest               — compact request construction
```

Cases to add:

```go
func TestCompact_ReducesMessageCount(t *testing.T) {
    // Message count must decrease after compaction
}

func TestCompact_PreservesRecentMessages(t *testing.T) {
    // Recent turns must survive compaction
}

func TestCompact_UpdatesSessionMetadata(t *testing.T) {
    // Session metadata must update after compaction
}

func TestCompact_PreCompact_Hook(t *testing.T) {
    // PreCompact hook must fire before compaction runs
}

func TestCompact_PostCompact_Hook(t *testing.T) {
    // PostCompact hook must fire after compaction completes
}

func TestCompact_AutoTrigger_ExceedsThreshold(t *testing.T) {
    // Auto-compact must fire when token count exceeds threshold
}

func TestCompact_MultipleTimes(t *testing.T) {
    // Multiple consecutive compactions must each reduce message count
}

func TestCompact_SummaryPersistsAcrossResume(t *testing.T) {
    // Compaction summary must be saved as session memory and restored on resume
}
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_compact -x 220 -y 60
tmux send-keys -t t_compact 'gen' Enter
sleep 2

# Test 1: Build up conversation history
for i in {1..5}; do
  tmux send-keys -t t_compact "message $i: tell me an interesting fact" Enter
  sleep 6
done

# Test 2: Manual compact
tmux send-keys -t t_compact '/compact' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: summary shown; message list replaced by a single summary entry

# Test 3: Continue conversation after compact
tmux send-keys -t t_compact 'what were we talking about?' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: assistant references the summary context

# Test 4: Compact with focus topic
tmux send-keys -t t_compact '/compact focus on facts about animals' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: summary emphasizes animal-related facts

# Test 5: Notice message visible
tmux capture-pane -t t_compact -p | grep -i "compact\|summary\|compressed"
# Expected: notice about compression visible in conversation

# Test 6: Resume after compact keeps summary context
tmux send-keys -t t_compact C-c
tmux send-keys -t t_compact 'gen -c' Enter
sleep 2
tmux send-keys -t t_compact 'what summary do you have from earlier?' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: resumed session still has the compacted summary context

tmux kill-session -t t_compact
```
