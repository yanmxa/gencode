package app

import (
	"fmt"

	"github.com/yanmxa/gencode/internal/message"
)

// persistToolResultOverflow checks if a tool result exceeds the overflow threshold
// and, if so, persists the full content to disk and replaces it with a truncated preview.
func (m *model) persistToolResultOverflow(result *message.ToolResult) {
	if len(result.Content) <= ToolResultOverflowThreshold {
		return
	}

	if err := m.ensureSessionStore(); err != nil {
		return
	}
	if m.session.CurrentID == "" {
		return
	}
	if err := m.session.Store.PersistToolResult(m.session.CurrentID, result.ToolCallID, result.Content); err != nil {
		return
	}

	preview := result.Content[:toolResultPreviewSize]
	result.Content = fmt.Sprintf("%s\n\n[Full output persisted to blobs/tool-result/%s/%s]", preview, m.session.CurrentID, result.ToolCallID)
}
