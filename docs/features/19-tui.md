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
| `Alt+Enter` | Insert newline |
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
go test ./internal/app/... -v
```

Covered:

```
# Markdown rendering
TestMDRenderer_Heading                  — heading levels rendered
TestMDRenderer_Emphasis                 — bold/italic rendering
TestMDRenderer_CodeSpan                 — inline code rendering
TestMDRenderer_FencedCodeBlock          — fenced code block with language
TestMDRenderer_UnorderedList            — bullet list rendering
TestMDRenderer_OrderedList              — numbered list rendering
TestMDRenderer_Link                     — link rendering
TestMDRenderer_ThematicBreak            — horizontal rule
TestMDRenderer_Blockquote               — blockquote rendering
TestMDRenderer_Paragraph                — paragraph text
TestMDRenderer_WordWrap                 — text wrapping at terminal width
TestMDRenderer_MixedContent             — mixed markdown types
TestMDRenderer_Table                    — table rendering
TestMDRenderer_TableWithLinks           — tables with embedded links
TestMDRenderer_NoLeadingBlankLine       — no leading blank line in output
TestMDRenderer_NoConsecutiveBlankLines  — no consecutive blank lines
TestRenderMarkdownContent               — full content rendering
TestRenderInlineMarkdown_Link           — inline link rendering
TestRender_Markdown_NestedList          — nested list rendering
TestRender_EmptyMessage_NoOutput        — empty input produces no output

# Line normalization
TestNormalizeLineBreaks                 — 9 sub-tests for line normalization

# Image handling
TestImageRefPattern                     — image reference pattern matching

# Input
TestReadSubmitRequest                   — submit request parsing
TestIsExitRequest                       — exit request detection

# App UI
TestOverlaySelectorsOrder               — overlay selector ordering
TestStartPromptSuggestionUsesRuntimeInterface — prompt suggestion
TestStartLLMStreamUsesRuntimeInterface  — LLM stream startup
TestBuildStreamRequestExcludesAssistantPlaceholder — stream request building
TestRenderActiveModalPriority           — modal priority rendering
TestCancelClearsTransientState          — cancel clears state
TestHandleKeypressEscClearsModelSearchBeforeDismiss — Esc clears search
TestHandleKeypressEscDismissesAfterSearchCleared    — Esc dismisses overlay

# Provider/plugin selectors
TestGoBackResetsInlineConnectState      — go back resets state
TestHandleKeypressTabSwitchClearsInlineResult — tab switch clears
TestSelectModelReturnsSelectionMessage  — model selection message
TestCancelClearsTransientPluginSelectorState — plugin selector cancel
TestHandleListEscClearsSearchBeforeDismiss   — plugin list Esc
TestHandleListEscDismissesSelector           — plugin list dismiss
TestSwitchTabResetsDetailStateAndSearch      — tab switch resets
TestToggleSelectedPluginReturnsDisableMsg    — plugin toggle
```

Cases to add:

```go
func TestInput_HistoryNavigation_UpDown(t *testing.T) {
    // Up/Down arrow must navigate through input history
}

func TestInput_MultilineEntry_AltEnter(t *testing.T) {
    // Alt+Enter must insert newline without submitting
}

func TestStream_ChunkRendering(t *testing.T) {
    // ChunkMsg must trigger View re-render with new tokens
}

func TestStream_EscCancels_PreservesPartial(t *testing.T) {
    // Esc during streaming must cancel and preserve partial response
}

func TestStatusBar_TokenCounts(t *testing.T) {
    // Status bar must display provider, model, and token counts
}

func TestTaskPanel_Toggle(t *testing.T) {
    // Ctrl+T must toggle task panel visibility
}
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

# Multi-line input via Alt+Enter
tmux send-keys -t t_tui 'first line' M-Enter
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
# Expected: task panel shown; Ctrl+T again hides it

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
