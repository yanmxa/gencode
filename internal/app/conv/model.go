package conv

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

type OutputModel struct {
	Spinner      spinner.Model
	MDRenderer   *MDRenderer
	TaskProgress map[int][]string
	ProgressHub  *ProgressHub
	ShowTasks    bool
}

type Model struct {
	ConversationModel
	OutputModel
}

func NewModel(width int) Model {
	hub := NewProgressHub(100)
	return Model{
		ConversationModel: NewConversation(),
		OutputModel: OutputModel{
			Spinner:     newSpinner(),
			MDRenderer:  NewMDRenderer(width),
			ProgressHub: hub,
			ShowTasks:   true,
		},
	}
}

func (m *OutputModel) ResizeMDRenderer(width int) {
	m.MDRenderer = NewMDRenderer(width)
}

func newSpinner() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"◐", "◓", "◑", "◒"},
		FPS:    80 * time.Millisecond,
	}
	sp.Style = lipgloss.NewStyle()
	return sp
}
