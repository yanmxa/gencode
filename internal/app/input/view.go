package input

import (
	"github.com/yanmxa/gencode/internal/app/render"
)

// RenderPendingImages renders indicator for clipboard images waiting to be sent.
func (m *Model) RenderPendingImages() string {
	return render.RenderPendingImages(render.PendingImagesParams{
		Pending:     m.Images.Pending,
		SelectMode:  m.Images.SelectMode,
		SelectedIdx: m.Images.SelectedIdx,
	})
}
