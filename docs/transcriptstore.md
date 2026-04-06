# Transcript Store

## Overview

`gencode` now persists sessions only through `transcriptstore`.

A project session is stored under:

```text
~/.gen/projects/<encoded-cwd>/
├── transcripts/
│   └── <session-id>.jsonl
├── transcripts-index.json
└── blobs/
    ├── summary/
    └── tool-result/
        └── <session-id>/
            └── <tool-call-id>
```

This is the single source of truth for:

- interactive session save/load
- `gen --continue`
- `gen --resume`
- forked sessions
- compact summary restore
- persisted tool-result overflow restore
- subagent session persistence

## Data Model

Each transcript contains:

- append-only JSONL records in `transcripts/<session-id>.jsonl`
- projected metadata in `transcripts-index.json`
- large side blobs in `blobs/`

The projected session state includes:

- `title`
- `lastPrompt`
- `summary`
- `tag`
- `mode`
- `provider`
- `model`
- `cwd`
- `createdAt`
- `updatedAt`
- `messageCount`
- `parentSessionID`
- `tasks`

Large tool results are not kept inline when overflow persistence is enabled. The message stores a short marker:

```text
[Full output persisted to blobs/tool-result/<session-id>/<tool-call-id>]
```

At load time, transcript hydration replaces that marker with the full blob content.

## Recovery Path

Session restore works in this order:

1. Load transcript records from `transcripts/<session-id>.jsonl`.
2. Project transcript state into metadata, tasks, and message nodes.
3. Hydrate large tool results from `blobs/tool-result/<session-id>/`.
4. Convert transcript nodes into app session entries.
5. Restore compact summary from transcript state.

The TUI and CLI resume flows do not use any legacy session format anymore.

## Automated Tests

Run the transcript-focused suites with:

```bash
GOCACHE=/tmp/gocache go test ./internal/transcriptstore ./internal/app/session ./tests/integration/session/... ./tests/integration/cli/...
```

### Core Store

`./internal/transcriptstore`

- `TestFileStoreStartAppendListLoad`
- `TestFileStoreCompactAndFork`
- `TestFileStoreReplace`
- `TestHydrateToolResultNodes`
- `TestHydrateToolResultNodesIgnoresUnmatchedMarkers`
- `TestMetadataAndTaskViewHelpers`
- `TestTranscriptTypesCarryProjectedState`
- `TestProjectedTypesDoNotExposeJSONTags`

These cover:

- transcript creation
- append and load
- replace-based full rewrite
- compaction projection
- fork projection
- metadata/list-item views
- tool-result hydration

### App Session Projection

`./internal/app/session`

- `TestStoreSaveLoadRoundTrip`
- `TestStoreFork`
- `TestEntriesToNodesAppliesDefaults`
- `TestEntriesFromNodesRoundTrip`
- `TestNormalizeMetadataAppliesDefaults`
- `TestTranscriptFromSnapshotProjectsMetadataAndTasks`
- `TestEncodePath`
- `TestGenerateSessionIDFormat`
- `TestGetGitBranch`

These cover:

- transcript-backed app session save/load
- node-entry projection
- metadata normalization
- fork behavior
- path and session-id helpers

### End-to-End Session Behavior

`./tests/integration/session`

- `TestSession_SaveAndLoad`
- `TestSession_List`
- `TestSession_GetLatest`
- `TestSession_Delete`
- `TestSession_Cleanup`
- `TestSession_AppendBehavior`
- `TestSession_MetadataUpdatesOnNewMessage`
- `TestSession_EntryRoundtrip`
- `TestSession_MessageTypes_PersistRoundTrip`
- `TestSession_PersistToolResult`
- `TestSession_SaveAndLoadSessionMemory`
- `TestSession_LoadSessionMemory_NotFound`
- `TestSession_SaveSessionMemory_Overwrite`
- `TestSession_MemoryEndToEnd`
- `TestSession_JSONL_Integrity`
- `TestSession_ContinueRestoresMessages`

These cover:

- on-disk transcript location and integrity
- message ordering after resume
- metadata updates
- memory/compact restore
- large tool-result persistence and hydration

### CLI Resume and Fork Paths

`./tests/integration/cli`

- `TestVersionCommand`
- `TestHelpCommand`
- `TestNonInteractivePrintMode`
- `TestSessionFork_IsIndependent`

These cover transcript-related CLI entry behavior:

- non-interactive startup path
- fork isolation
- resume-adjacent startup commands without a live provider

## Manual Non-Interactive Checks

These checks do not require the TUI.

```bash
# version/help should work without provider setup
gen version
gen help

# print mode without provider should fail clearly
env -u ANTHROPIC_API_KEY -u OPENAI_API_KEY -u GOOGLE_API_KEY -u MOONSHOT_API_KEY gen -p "hello"
```

Expected:

- `gen version` prints `gen version <value>`
- `gen help` prints flags including `--continue`, `--resume`, `--fork`
- `gen -p` without a configured provider exits non-zero and mentions provider configuration

## Manual Interactive Checks

These checks validate transcript persistence through the real TUI.

### Prerequisites

- `tmux` installed
- one working provider configured
- run from a stable project directory

### Session Lifecycle

```bash
tmux new-session -d -s t_tx -x 220 -y 60
tmux send-keys -t t_tx 'gen' Enter
sleep 2
tmux send-keys -t t_tx 'remember the number 42 and the word kiwi' Enter
sleep 8
tmux capture-pane -t t_tx -p
```

Expected:

- assistant response is visible
- a new transcript appears under `~/.gen/projects/<encoded-cwd>/transcripts/`

### Continue Latest

```bash
tmux send-keys -t t_tx C-c
tmux send-keys -t t_tx 'gen -c' Enter
sleep 2
tmux capture-pane -t t_tx -p
```

Expected:

- the latest transcript loads directly
- previous conversation is visible

### Resume Specific Session

```bash
PROJECT_DIR=~/.gen/projects/$(pwd | sed 's#/$##' | sed 's#/#-#g')
SESSION_ID=$(find "${PROJECT_DIR}/transcripts" -name '*.jsonl' | head -1 | xargs basename | sed 's/\.jsonl$//')
tmux send-keys -t t_tx C-c
tmux send-keys -t t_tx "gen -r ${SESSION_ID}" Enter
sleep 2
tmux capture-pane -t t_tx -p
```

Expected:

- the selected transcript loads without showing the picker

### Fork

```bash
tmux send-keys -t t_tx C-c
tmux send-keys -t t_tx "gen -r ${SESSION_ID} --fork" Enter
sleep 2
tmux capture-pane -t t_tx -p
```

Expected:

- a new session opens with copied history
- the source transcript is unchanged

### Compact and Resume

```bash
tmux send-keys -t t_tx '/compact remember the 42 example' Enter
sleep 6
tmux send-keys -t t_tx C-c
tmux send-keys -t t_tx 'gen -c' Enter
sleep 2
tmux send-keys -t t_tx 'what did we compact?' Enter
sleep 6
tmux capture-pane -t t_tx -p
```

Expected:

- compact summary survives resume
- answer reflects compacted context

### JSONL Integrity

```bash
SESSION_FILE=$(find "${PROJECT_DIR}/transcripts" -name '*.jsonl' | head -1)
export SESSION_FILE
python - <<'PY'
import json, os, pathlib
path = pathlib.Path(os.environ["SESSION_FILE"])
for line in path.read_text().splitlines():
    if line.strip():
        json.loads(line)
print("jsonl ok")
PY
```

Expected:

- script prints `jsonl ok`

### Tool Result Overflow

Use a prompt that produces a large tool result, then inspect the project store:

```bash
find "${PROJECT_DIR}/blobs/tool-result" -type f | sed -n '1,20p'
```

Expected:

- blob files exist under `blobs/tool-result/<session-id>/`
- resumed session shows the full tool result content instead of only the marker

### Cleanup

End the manual session:

```bash
tmux kill-session -t t_tx
```

## Related Docs

- `docs/features/1-cli-startup.md`
- `docs/features/2-session.md`
- `docs/features/15-compact.md`
- `docs/features/19-tui.md`
