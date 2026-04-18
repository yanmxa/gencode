package input

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/kit/history"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/core"
	coremcp "github.com/yanmxa/gencode/internal/mcp"
	coreplugin "github.com/yanmxa/gencode/internal/plugin"
	coreskill "github.com/yanmxa/gencode/internal/skill"
)

// PastedChunk holds a collapsed multi-line paste block.
type PastedChunk struct {
	Text      string // the full pasted text
	LineCount int    // total line count
}

// HistoryNav tracks command history navigation state.
type HistoryNav struct {
	Items   []string
	Index   int    // -1 = not navigating
	Stashed string // stashed textarea input while navigating
}

// Model holds all input-related state: textarea, history, suggestions, images, and selectors.
type Model struct {
	Textarea         textarea.Model
	History          HistoryNav
	PromptSuggestion PromptSuggestionState
	Suggestions      suggest.State
	LastCtrlO        time.Time
	Images           ImageState
	TerminalHeight   int
	PastedChunks     []PastedChunk
	Queue            Queue

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

// SelectorDeps holds the external dependencies needed to build all input selectors.
// Passing this to New() lets the root model avoid manually wiring each selector.
type SelectorDeps struct {
	AgentRegistry  AgentRegistry
	SkillRegistry  *coreskill.Registry
	MCPRegistry    *coremcp.Registry
	PluginRegistry *coreplugin.Registry
	LoadDisabled   func(userLevel bool) map[string]bool
	UpdateDisabled func(disabled map[string]bool, userLevel bool) error
}

// New creates a fully initialized input Model with all selectors wired up.
func New(cwd string, width int, matchFunc suggest.Matcher, deps SelectorDeps) Model {
	suggestions := suggest.NewState(matchFunc)
	suggestions.SetCwd(cwd)
	return Model{
		Textarea:    newTextarea(width),
		History:     HistoryNav{Items: history.Load(cwd), Index: -1},
		Suggestions: suggestions,
		Queue:       NewQueue(),

		Approval: NewApproval(),
		Agent:    NewAgentSelector(deps.AgentRegistry),
		Search:   NewSearchSelector(),
		Skill:    SkillState{Selector: NewSkillSelector(deps.SkillRegistry)},
		Session:  SessionState{Selector: NewSessionSelector()},
		Memory:   MemoryState{Selector: NewMemorySelector()},
		MCP:      MCPState{Selector: NewMCPSelector(deps.MCPRegistry)},
		Plugin:   NewPluginSelector(deps.PluginRegistry),
		Provider: ProviderState{Selector: NewProviderSelector()},
		Tool:     NewToolSelector(deps.LoadDisabled, deps.UpdateDisabled),
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
