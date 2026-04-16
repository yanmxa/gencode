package user

import (
	"time"

	"github.com/charmbracelet/bubbles/textarea"

	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/core"
)

// PastedChunk holds a collapsed multi-line paste block.
type PastedChunk struct {
	Text      string // the full pasted text
	LineCount int    // total line count
}

// Model holds all input-related state: textarea, history, suggestions, and images.
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
