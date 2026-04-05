package input

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/message"
)

const (
	minTextareaHeight = 1
	maxTextareaHeight = 6
)

// ImageRefPattern matches @path/to/image.ext references (case-insensitive extension).
var ImageRefPattern = regexp.MustCompile(`(?i)@([^\s]+\.(png|jpg|jpeg|gif|webp))`)

// UpdateHeight adjusts textarea height based on content line count.
func (m *Model) UpdateHeight() {
	content := m.Textarea.Value()
	lines := strings.Count(content, "\n") + 1

	newHeight := lines
	if newHeight < minTextareaHeight {
		newHeight = minTextareaHeight
	}
	if newHeight > maxTextareaHeight {
		newHeight = maxTextareaHeight
	}

	m.Textarea.SetHeight(newHeight)
}

// HistoryUp navigates to the previous history entry.
func (m *Model) HistoryUp() {
	if len(m.History) == 0 {
		return
	}
	if m.HistoryIdx == -1 {
		m.TempInput = m.Textarea.Value()
		m.HistoryIdx = len(m.History) - 1
	} else if m.HistoryIdx > 0 {
		m.HistoryIdx--
	}
	m.Textarea.SetValue(m.History[m.HistoryIdx])
	m.Textarea.CursorEnd()
	m.UpdateHeight()
}

// HistoryDown navigates to the next history entry.
func (m *Model) HistoryDown() {
	if m.HistoryIdx == -1 {
		return
	}
	if m.HistoryIdx < len(m.History)-1 {
		m.HistoryIdx++
		m.Textarea.SetValue(m.History[m.HistoryIdx])
	} else {
		m.HistoryIdx = -1
		m.Textarea.SetValue(m.TempInput)
	}
	m.Textarea.CursorEnd()
	m.UpdateHeight()
}

// ProcessImageRefs extracts @image.png references from input.
// Returns the cleaned text content and any loaded images.
// Only processes references where the file actually exists on disk;
// non-existent file references are left in the text as-is.
func ProcessImageRefs(cwd, input string) (string, []message.ImageData, error) {
	matches := ImageRefPattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input, nil, nil
	}

	var images []message.ImageData
	var loadedRefs []string // track which @references were successfully loaded
	for _, match := range matches {
		path := match[1]
		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(cwd, absPath)
		}

		// Skip references to files that don't exist
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			continue
		}

		imgInfo, err := image.Load(absPath)
		if err != nil {
			return "", nil, fmt.Errorf("loading image %s: %w", absPath, err)
		}
		images = append(images, imgInfo.ToProviderData())
		loadedRefs = append(loadedRefs, match[0]) // full match including @
	}

	// Only remove references that were successfully loaded
	content := input
	for _, ref := range loadedRefs {
		content = strings.ReplaceAll(content, ref, "")
	}
	content = strings.TrimSpace(content)

	return content, images, nil
}

// MinTextareaHeight returns the minimum textarea height constant.
func MinTextareaHeight() int {
	return minTextareaHeight
}
