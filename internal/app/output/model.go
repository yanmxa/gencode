package output

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"

	"github.com/yanmxa/gencode/internal/app/render"
)

// Model holds all output-related state: spinner, markdown renderer, and task progress.
type Model struct {
	Spinner      spinner.Model
	MDRenderer   *render.MDRenderer
	TaskProgress map[int][]string
}

// New creates a fully initialized output Model.
func New(width int) Model {
	return Model{
		Spinner:    newSpinner(),
		MDRenderer: render.NewMDRenderer(width),
	}
}

func newSpinner() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    80 * time.Millisecond,
	}
	sp.Style = render.ThinkingStyle
	return sp
}
