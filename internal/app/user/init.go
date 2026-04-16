package user

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit/history"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/app/kit"
)

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
		ShowTasks:      true,
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
