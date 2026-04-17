package input

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/kit/history"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/core"
)

// PastedChunk holds a collapsed multi-line paste block.
type PastedChunk struct {
	Text      string // the full pasted text
	LineCount int    // total line count
}

// Model holds all input-related state: textarea, history, suggestions, images, and selectors.
type Model struct {
	Textarea       textarea.Model
	History        []string
	HistoryIdx     int
	TempInput      string
	Suggestions    suggest.State
	LastCtrlO      time.Time
	Images         ImageState
	TerminalHeight int
	PastedChunks   []PastedChunk
	QueueSelectIdx int    // -1 = no selection, 0+ = selected queue item index
	QueueTempInput string // stashed input when navigating into queue
	Queue          Queue

	// Selectors / overlays
	Approval ApprovalModel
	Agent    AgentSelector
	Search   SearchSelector
	Skill    SkillState
	Session  SessionState
	Memory   MemoryState
	MCP      MCPState
	Plugin   PluginSelector
	Provider ProviderState
	Tool     ToolSelector
}

// PendingImage holds an inline image token and its provider payload.
type PendingImage struct {
	ID   int
	Data core.Image
}

// ImageSelection tracks the currently selected inline image token.
type ImageSelection struct {
	Active       bool
	PendingIdx   int
	CursorAbsPos int
}

// ImageState holds state for pending inline image tokens.
type ImageState struct {
	Pending   []PendingImage
	NextID    int
	Selection ImageSelection
}

// RemoveAt removes the image at the given index and adjusts selection state.
func (img *ImageState) RemoveAt(idx int) {
	if idx < 0 || idx >= len(img.Pending) {
		return
	}
	img.Pending = append(img.Pending[:idx], img.Pending[idx+1:]...)
	if len(img.Pending) == 0 {
		img.Selection = ImageSelection{}
		return
	}
	if img.Selection.PendingIdx == idx {
		img.Selection = ImageSelection{}
		return
	}
	if img.Selection.PendingIdx > idx {
		img.Selection.PendingIdx--
	}
}

// New creates a fully initialized input Model.
func New(cwd string, width int, matchFunc suggest.Matcher) Model {
	suggestions := suggest.NewState(matchFunc)
	suggestions.SetCwd(cwd)
	return Model{
		Textarea:       newTextarea(width),
		History:        history.Load(cwd),
		HistoryIdx:     -1,
		Suggestions:    suggestions,
		QueueSelectIdx: -1,
	}
}

// newTextarea creates a configured textarea with sensible defaults.
func newTextarea(width int) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.Focus()
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetWidth(width)
	ta.SetHeight(minTextareaHeight)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	ta.KeyMap.InsertNewline.SetEnabled(true)
	return ta
}
