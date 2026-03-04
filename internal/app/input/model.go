package input

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/ui/suggest"
)

// Model holds all input-related state: textarea, history, suggestions, and images.
type Model struct {
	Textarea    textarea.Model
	History     []string
	HistoryIdx  int
	TempInput   string
	Suggestions suggest.State
	LastCtrlO   time.Time
	Images      ImageState
}

// ImageState holds state for pending image attachments.
type ImageState struct {
	Pending     []message.ImageData
	SelectMode  bool
	SelectedIdx int
}

// RemoveAt removes the image at the given index and adjusts selection state.
func (img *ImageState) RemoveAt(idx int) {
	if idx < 0 || idx >= len(img.Pending) {
		return
	}
	img.Pending = append(img.Pending[:idx], img.Pending[idx+1:]...)
	if img.SelectedIdx >= len(img.Pending) && img.SelectedIdx > 0 {
		img.SelectedIdx--
	}
	if len(img.Pending) == 0 {
		img.SelectMode = false
	}
}
