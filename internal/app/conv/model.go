package conv

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

// OutputModel holds all output-related state: spinner, markdown renderer, task progress,
// and display toggles.
type OutputModel struct {
	Spinner      spinner.Model
	MDRenderer   *MDRenderer
	TaskProgress map[int][]string
	ProgressHub  *ProgressHub
	ShowTasks    bool
}

// New creates a fully initialized output OutputModel.
// hub may be nil to disable progress transport for tests or non-interactive use.
func New(width int, hub *ProgressHub) OutputModel {
	return OutputModel{
		Spinner:     newSpinner(),
		MDRenderer:  NewMDRenderer(width),
		ProgressHub: hub,
		ShowTasks:   true,
	}
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
