# Feature 15: Compact / Conversation Compression

## Overview

Compaction summarises old messages to free up context window space. It can be triggered manually or fires automatically when the token count approaches the model's limit.

- **Manual trigger:** `/compact` slash command
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
TestCompact_ManualTrigger
TestCompact_ReducesMessageCount
TestCompact_PreservesRecentMessages
TestCompact_UpdatesSessionMetadata
TestCompact_PreCompact_Hook
TestCompact_PostCompact_Hook
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_compact -x 220 -y 60
tmux send-keys -t t_compact 'gen' Enter
sleep 2

# Build up conversation history
for i in {1..5}; do
  tmux send-keys -t t_compact "message $i: tell me an interesting fact" Enter
  sleep 6
done

# Manual compact
tmux send-keys -t t_compact '/compact' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: summary shown; message list replaced by a single summary entry

# Continue conversation after compact
tmux send-keys -t t_compact 'what were we talking about?' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: assistant references the summary context

tmux kill-session -t t_compact
```
