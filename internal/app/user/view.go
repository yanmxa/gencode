package user

import (
	"strings"

	"github.com/yanmxa/gencode/internal/app/ui/render"
)

// RenderTextarea renders the textarea with styled inline image tokens.
func (m *Model) RenderTextarea() string {
	view := strings.TrimRight(m.Textarea.View(), " ")
	if len(m.Images.Pending) == 0 {
		return view
	}

	selectedPendingIdx := -1
	if match, ok := m.SelectedImageMatch(); ok {
		selectedPendingIdx = match.PendingIdx
	}

	for _, match := range m.PendingImageMatches() {
		style := render.PendingImageStyle
		if match.PendingIdx == selectedPendingIdx {
			style = render.SelectedImageStyle
		}
		view = strings.Replace(view, match.Label, style.Render(match.Label), 1)
	}

	return view
}
