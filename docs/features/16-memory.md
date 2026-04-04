# Feature 16: Memory System

## Overview

gencode injects project and user instructions from Markdown files into the LLM's system prompt at startup.

**Files:**

| File | Scope |
|------|-------|
| `~/.gen/GEN.md` | User-level instructions |
| `./.gen/GEN.md` | Project-level instructions |
| `./.gen/local/GEN.md` | Local instructions (git-ignored) |

**Import syntax:** `@import other.md` — inline another file's content.

**Load order** (lowest → highest priority): User → Project → Local

**`/memory` command:** view and edit all loaded memory files in the TUI.

## UI Interactions

- **`/memory`**: opens a viewer showing all loaded files and their contents; edit links open the file in `$EDITOR`.
- **Instructions effect**: the LLM silently follows instructions from GEN.md without the user needing to repeat them.

## Automated Tests

```bash
go test ./internal/app/memory/... -v
go test ./internal/system/... -v
```

Covered:

```
TestMemory_LoadGENmd
TestMemory_ImportSyntax
TestMemory_ScopeMerge
TestMemory_Caching
```

Cases to add:

```go
func TestMemory_ProjectOverridesUser_SameName(t *testing.T) {
    // Project GEN.md must take precedence over user GEN.md
}

func TestMemory_ImportChain(t *testing.T) {
    // @import A, A imports B — both must be loaded into system prompt
}

func TestMemory_MissingFile_NoError(t *testing.T) {
    // Missing GEN.md must not crash startup
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/mem_test/.gen
cat > /tmp/mem_test/.gen/GEN.md << 'EOF'
# Project Instructions

- Always respond in exactly one sentence.
- End every response with "(end)".
EOF

tmux new-session -d -s t_mem -x 220 -y 60
tmux send-keys -t t_mem 'cd /tmp/mem_test && gen' Enter
sleep 2
tmux send-keys -t t_mem 'hello' Enter
sleep 6
tmux capture-pane -t t_mem -p
# Expected: response is one sentence ending with "(end)"

# /memory command
tmux send-keys -t t_mem '/memory' Enter
sleep 2
tmux capture-pane -t t_mem -p
# Expected: GEN.md content displayed

tmux kill-session -t t_mem
rm -rf /tmp/mem_test
```
