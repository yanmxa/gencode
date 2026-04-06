package transcriptstore

import (
	"encoding/json"
	"time"
)

const (
	RecordStarted         = "transcript.started"
	RecordMessageAppended = "message.appended"
	RecordStatePatched    = "state.patched"
	RecordCompacted       = "transcript.compacted"
	RecordForked          = "transcript.forked"
)

const (
	PatchPathTitle      = "title"
	PatchPathLastPrompt = "lastPrompt"
	PatchPathTag        = "tag"
	PatchPathMode       = "mode"
	PatchPathSummary    = "summary"
	PatchPathTasks      = "tasks"
	PatchPathWorktree   = "worktree"
)

type Record struct {
	ID           string    `json:"id"`
	TranscriptID string    `json:"transcriptId"`
	Time         time.Time `json:"time"`
	Type         string    `json:"type"`

	ParentID    string `json:"parentId,omitempty"`
	IsSidechain bool   `json:"isSidechain,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	Version     string `json:"version,omitempty"`
	GitBranch   string `json:"gitBranch,omitempty"`
	AgentID     string `json:"agentId,omitempty"`

	Message *MessageRecord `json:"message,omitempty"`
	State   *StateRecord   `json:"state,omitempty"`
	System  *SystemRecord  `json:"system,omitempty"`
}

type MessageRecord struct {
	MessageID string         `json:"messageId"`
	Role      string         `json:"role"`
	Content   []ContentBlock `json:"content"`
}

type StateRecord struct {
	Ops []PatchOp `json:"ops"`
}

type PatchOp struct {
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

type SystemRecord struct {
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	ParentID      string `json:"parentId,omitempty"`
	SummaryBlobID string `json:"summaryBlobId,omitempty"`
	BoundaryID    string `json:"boundaryId,omitempty"`
}

type ContentBlock struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   []ContentBlock `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`

	Source *ImageSource `json:"source,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type WorktreeState struct {
	OriginalCwd    string `json:"originalCwd"`
	WorktreePath   string `json:"worktreePath"`
	WorktreeName   string `json:"worktreeName"`
	WorktreeBranch string `json:"worktreeBranch,omitempty"`
	OriginalBranch string `json:"originalBranch,omitempty"`
	Exited         bool   `json:"exited,omitempty"`
}
