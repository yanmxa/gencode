# Feature 16: Memory System

## Overview

gencode injects project and user instructions from Markdown files into the LLM's system prompt at startup.

**Files:**

| File | Scope |
|------|-------|
| `~/.gen/GEN.md` | User-level instructions |
| `./.gen/GEN.md` | Project-level instructions |
| `./.gen/GEN.local.md` | Local instructions (git-ignored) |
| `./.gen/rules/*.md` | Project rule files loaded alongside memory |

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
# Import resolution
TestResolveImports                    — basic @import resolution
TestResolveImportsCycle               — circular import detection
TestResolveImportsNotFound            — missing import file handled
TestResolveImportsMaxDepth            — max import depth enforced
TestResolveImportsNested              — nested imports resolved
TestResolveImportsRelativePath        — relative path imports

# Import chain (Cases to add → now covered)
TestMemory_ImportChain                — @import A, A imports B — both loaded
TestMemory_MissingFile_NoError        — missing GEN.md does not crash startup

# Memory file loading
TestLoadMemoryFilesWithImports        — memory files with imports loaded
TestFindMemoryFile                    — find memory file by path
TestGetAllMemoryPaths                 — all memory file paths collected
TestFormatFileSize                    — file size formatting
TestLoadRulesDirectory                — rules directory loading
TestLoadInstructions                  — instruction loading pipeline

# System prompt integration
TestPromptCaching                     — prompt caching works
TestPromptContainsInstructions        — instructions in system prompt
TestPromptDirectFields                — direct fields in prompt
TestPromptExtra                       — extra content in prompt
TestPromptNarrativeOrder              — narrative ordering
TestPromptPlanMode                    — plan mode prompt
TestPromptEmptyFieldsExcluded         — empty fields excluded
TestPromptInitCachedFiles             — cached files initialization

# Memory command
TestHandleMemoryList                  — /memory list formats output with sections
```

Cases to add:

```go
func TestMemory_ProjectOverridesUser_SameName(t *testing.T) {
    // Project GEN.md must take precedence over user GEN.md
}

func TestMemory_ThreeScopeMerge(t *testing.T) {
    // User + Project + Local GEN.md must merge with correct priority
}

func TestMemory_EditorLink_OpensFile(t *testing.T) {
    // Edit link in /memory must open file path with $EDITOR
}

func TestMemory_LocalFilePath_Resolved(t *testing.T) {
    // Local instructions must be loaded from .gen/GEN.local.md
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/mem_test/.gen

# Test 1: Instructions affect LLM behavior
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

# Test 2: /memory command shows loaded content
tmux send-keys -t t_mem '/memory' Enter
sleep 2
tmux capture-pane -t t_mem -p
# Expected: GEN.md content displayed

# Test 3: @import syntax
cat > /tmp/mem_test/.gen/rules.md << 'EOF'
- Never use exclamation marks.
EOF
cat > /tmp/mem_test/.gen/GEN.md << 'EOF'
# Project Instructions

@import rules.md

- Always respond in exactly one sentence.
EOF
tmux send-keys -t t_mem C-c
tmux send-keys -t t_mem 'cd /tmp/mem_test && gen' Enter
sleep 2
tmux send-keys -t t_mem '/memory' Enter
sleep 2
tmux capture-pane -t t_mem -p
# Expected: both GEN.md and imported rules.md content visible

# Test 4: Local GEN.md overrides project
cat > /tmp/mem_test/.gen/GEN.local.md << 'EOF'
# Local Override
- Always start your response with "LOCAL:"
EOF
tmux send-keys -t t_mem C-c
tmux send-keys -t t_mem 'cd /tmp/mem_test && gen' Enter
sleep 2
tmux send-keys -t t_mem 'hello' Enter
sleep 6
tmux capture-pane -t t_mem -p
# Expected: response starts with "LOCAL:" (local overrides project)

# Test 5: Rules directory appears in /memory
mkdir -p /tmp/mem_test/.gen/rules
cat > /tmp/mem_test/.gen/rules/style.md << 'EOF'
- Prefer short paragraphs.
EOF
tmux send-keys -t t_mem '/memory' Enter
sleep 2
tmux capture-pane -t t_mem -p
# Expected: rules/style.md is listed in the memory viewer

tmux kill-session -t t_mem
rm -rf /tmp/mem_test
```
