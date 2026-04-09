package session

import (
	"time"

	"github.com/yanmxa/gencode/internal/transcript"
)

var AppVersion string

const (
	EntryUser      = "user"
	EntryAssistant = "assistant"
)

type ContentBlock = transcript.ContentBlock
type ImageSource = transcript.ImageSource
type SessionMetadata = transcript.MetadataView

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
