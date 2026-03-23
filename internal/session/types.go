package session

import (
	"encoding/json"
	"time"

	"github.com/yanmxa/gencode/internal/tool"
)

// AppVersion is set by the main package at startup.
var AppVersion string

// Entry type constants identify the kind of JSONL entry.
const (
	EntryMetadata  = "metadata"
	EntryUser      = "user"
	EntryAssistant = "assistant"
)

// ContentBlock represents a content block in the Anthropic API format.
// It is a union type — only fields relevant to the block's Type are populated.
type ContentBlock struct {
	Type string `json:"type"`

	// text block
	Text string `json:"text,omitempty"`

	// thinking block
	Thinking string `json:"thinking,omitempty"`

	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   []ContentBlock `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`

	// image block
	Source *ImageSource `json:"source,omitempty"`
}

// ImageSource represents image data source for content blocks.
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// EntryMessage holds the role and content blocks for a message entry.
type EntryMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// Entry represents a single JSONL line in a session file.
type Entry struct {
	Type        string          `json:"type"`
	ParentUuid  *string         `json:"parentUuid,omitempty"`
	IsSidechain bool            `json:"isSidechain,omitempty"`
	Cwd         string          `json:"cwd,omitempty"`
	SessionID   string          `json:"sessionId,omitempty"`
	Version     string          `json:"version,omitempty"`
	GitBranch   string          `json:"gitBranch,omitempty"`
	AgentID     string          `json:"agentId,omitempty"`
	Message     *EntryMessage   `json:"message,omitempty"`
	UUID        string          `json:"uuid,omitempty"`
	Timestamp   time.Time       `json:"timestamp,omitempty"`

	// metadata type fields
	Metadata *EntryMetadata_ `json:"metadata,omitempty"`
}

// EntryMetadata_ holds session-level metadata written as a JSONL entry.
// The trailing underscore avoids collision with the Entry.Metadata field name.
type EntryMetadata_ struct {
	Title           string          `json:"title"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	Cwd             string          `json:"cwd"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	MessageCount    int             `json:"messageCount"`
	ParentSessionID string          `json:"parentSessionId,omitempty"`
	Tasks           []tool.TodoTask `json:"tasks,omitempty"`
}

// SessionMetadata contains metadata about a session
type SessionMetadata struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	Cwd             string    `json:"cwd"`
	MessageCount    int       `json:"messageCount"`
	ParentSessionID string    `json:"parentSessionId,omitempty"`
}

// Session represents a complete session with metadata and entries
type Session struct {
	Metadata SessionMetadata `json:"metadata"`
	Entries  []Entry         `json:"entries"`
	Tasks    []tool.TodoTask `json:"tasks,omitempty"`
}

// SessionIndex holds the per-project session index for fast listing.
type SessionIndex struct {
	Version      int          `json:"version"`
	OriginalPath string       `json:"originalPath"`
	Entries      []IndexEntry `json:"entries"`
}

// IndexEntry is one entry in the session index.
type IndexEntry struct {
	SessionID    string    `json:"sessionId"`
	FullPath     string    `json:"fullPath"`
	FileMtime    int64     `json:"fileMtime"`
	FirstPrompt  string    `json:"firstPrompt"`
	Summary      string    `json:"summary"`
	MessageCount int       `json:"messageCount"`
	Created      time.Time `json:"created"`
	Modified     time.Time `json:"modified"`
	GitBranch    string    `json:"gitBranch,omitempty"`
	IsSidechain  bool      `json:"isSidechain,omitempty"`
}
