// Shared constants: default dimensions, timeouts, and limits.
package tui

import (
	"time"

	"github.com/yanmxa/gencode/internal/options"
)

const (
	defaultMaxTokens   = options.DefaultMaxTokens
	doubleTapThreshold = 500 * time.Millisecond
	defaultWidth       = 80
	maxTextareaHeight  = 6
	minTextareaHeight  = 1
	minWrapWidth       = 40
)
