package session

import (
	"time"

	"github.com/yanmxa/gencode/internal/transcriptstore"
)

// AppVersion is set by the main package at startup.
var AppVersion string

const (
	EntryUser      = "user"
	EntryAssistant = "assistant"
)

type ContentBlock = transcriptstore.ContentBlock
type ImageSource = transcriptstore.ImageSource
type SessionMetadata = transcriptstore.MetadataView

type EntryMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

type Entry struct {
	Type        string        `json:"type"`
	ParentUuid  *string       `json:"parentUuid,omitempty"`
	IsSidechain bool          `json:"isSidechain,omitempty"`
	Cwd         string        `json:"cwd,omitempty"`
	SessionID   string        `json:"sessionId,omitempty"`
	Version     string        `json:"version,omitempty"`
	GitBranch   string        `json:"gitBranch,omitempty"`
	AgentID     string        `json:"agentId,omitempty"`
	Message     *EntryMessage `json:"message,omitempty"`
	UUID        string        `json:"uuid,omitempty"`
	Timestamp   time.Time     `json:"timestamp,omitempty"`
}
