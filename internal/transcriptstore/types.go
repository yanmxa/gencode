package transcriptstore

import "time"

type Transcript struct {
	ID        string
	ParentID  string
	Cwd       string
	CreatedAt time.Time
	UpdatedAt time.Time

	Provider string
	Model    string

	Messages []Node
	State    State
}

type Node struct {
	ID          string
	ParentID    string
	Role        string
	Time        time.Time
	Cwd         string
	GitBranch   string
	AgentID     string
	IsSidechain bool
	Content     []ContentBlock
}

type State struct {
	Title      string
	LastPrompt string
	Tag        string
	Mode       string
	Summary    string

	Tasks    []TodoTaskView
	Worktree *WorktreeState
}

type TodoTaskView struct {
	ID              string
	Subject         string
	Description     string
	ActiveForm      string
	Status          string
	Owner           string
	Blocks          []string
	BlockedBy       []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StatusChangedAt time.Time
}

type ListItem struct {
	TranscriptID string
	FullPath     string
	CreatedAt    time.Time
	UpdatedAt    time.Time

	Title        string
	LastPrompt   string
	MessageCount int
	GitBranch    string

	HasSummary  bool
	IsSidechain bool
}
