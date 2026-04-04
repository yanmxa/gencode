# Feature 19: TUI Rendering & Interaction

## Overview

The terminal UI is built with Bubble Tea. It handles keyboard input, real-time streaming output, and Markdown rendering.

**Components:**

| Component | Description |
|-----------|-------------|
| Input box | Multi-line textarea with message history (↑/↓) |
| Output area | Markdown with syntax highlighting |
| Status bar | Token counts, provider/model, permission mode |
| Progress spinner | Active during streaming |
| Task panel | `Ctrl+T` toggles a bottom task list |

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `Enter` | Submit message |
| `Shift+Enter` | Insert newline |
| `↑` / `↓` | Navigate input history |
| `Ctrl+T` | Toggle task panel |
| `Esc` | Cancel active stream |
| `Ctrl+C` | Exit |

**Markdown features:** fenced code blocks with syntax highlighting, bold/italic, ordered/unordered lists, inline code.

## How Streaming Works

1. LLM API sends tokens via SSE.
2. Each chunk becomes a `ChunkMsg` that triggers a Bubble Tea `Update` cycle.
3. `View()` re-renders after each chunk — the user sees tokens appear in real time.
4. On `Esc`, the stream context is cancelled; the partial response is preserved.

## Automated Tests

```bash
go test ./internal/app/render/... -v
go test ./internal/app/input/... -v
```

Covered:

```
TestRender_Markdown_CodeBlock
TestRender_Markdown_BoldItalic
TestRender_Markdown_List
TestMarkdown_SyntaxHighlight
TestInput_MultilineEntry
TestInput_HistoryNavigation
```

## Interactive Tests (tmux)

```bash
tmux new-session -d -s t_tui -x 220 -y 60
tmux send-keys -t t_tui 'gen' Enter
sleep 2

# Markdown rendering — code block
tmux send-keys -t t_tui 'show me a python hello world example' Enter
sleep 8
tmux capture-pane -t t_tui -p
# Expected: ```python block with syntax highlighting

# Multi-line input via Shift+Enter
tmux send-keys -t t_tui 'first line' S-Enter
sleep 0.3
tmux send-keys -t t_tui 'second line' Enter
sleep 6
tmux capture-pane -t t_tui -p
# Expected: both lines sent as a single message

# Input history — up arrow
tmux send-keys -t t_tui Up ''
sleep 0.3
tmux capture-pane -t t_tui -p
# Expected: previous input appears in the input box

# Task panel toggle
tmux send-keys -t t_tui C-t
sleep 0.3
tmux capture-pane -t t_tui -p
# Expected: task panel shown at bottom; Ctrl+T again hides it

# Interrupt streaming
tmux send-keys -t t_tui 'write a 1000-word essay about mountains' Enter
sleep 2
tmux send-keys -t t_tui Escape
sleep 1
tmux capture-pane -t t_tui -p
# Expected: streaming stops; partial response visible

# Status bar check
tmux capture-pane -t t_tui -p | tail -3
# Expected: provider name, model, and token counts visible

tmux kill-session -t t_tui
```
