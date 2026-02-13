package session

import (
	"time"

	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
)

// SessionMetadata contains metadata about a session
type SessionMetadata struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	Cwd          string    `json:"cwd"`
	MessageCount int       `json:"messageCount"`
}

// StoredMessage represents a message stored in a session
type StoredMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content,omitempty"`
	Thinking   string               `json:"thinking,omitempty"`
	Images     []provider.ImageData `json:"images,omitempty"`
	ToolCalls  []provider.ToolCall  `json:"toolCalls,omitempty"`
	ToolResult *provider.ToolResult `json:"toolResult,omitempty"`
	ToolName   string               `json:"toolName,omitempty"`
	IsSummary  bool                 `json:"isSummary,omitempty"`
}

// Session represents a complete session with metadata and messages
type Session struct {
	Metadata SessionMetadata `json:"metadata"`
	Messages []StoredMessage `json:"messages"`
	Tasks    []tool.TodoTask `json:"tasks,omitempty"`
}
