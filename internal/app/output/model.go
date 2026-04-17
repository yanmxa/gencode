package output

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/output/render"
)

// Model holds all output-related state: spinner, markdown renderer, and task progress.
type Model struct {
	Spinner      spinner.Model
	MDRenderer   *render.MDRenderer
	TaskProgress map[int][]string
	ProgressHub  *ProgressHub
}

// New creates a fully initialized output Model.
// hub may be nil to disable progress transport for tests or non-interactive use.
func New(width int, hub *ProgressHub) Model {
	return Model{
		Spinner:     newSpinner(),
		MDRenderer:  render.NewMDRenderer(width),
		ProgressHub: hub,
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
