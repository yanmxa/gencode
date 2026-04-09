package transcript

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yanmxa/gencode/internal/tracker"
)

type StartCommand struct {
	TranscriptID string
	ProjectID    string
	Cwd          string
	Provider     string
	Model        string
	ParentID     string
	Time         time.Time
}

type AppendMessageCommand struct {
	TranscriptID string
	MessageID    string
	ParentID     string
	Time         time.Time
	Cwd          string
	GitBranch    string
	AgentID      string
	IsSidechain  bool
	Role         string
	Content      []ContentBlock
}

type PatchStateCommand struct {
	TranscriptID string
	Time         time.Time
	Ops          []PatchOp
}

type CompactCommand struct {
	TranscriptID string
	Time         time.Time
	BoundaryID   string
	Summary      string
}

type ForkCommand struct {
	SourceTranscriptID string
	NewTranscriptID    string
	Time               time.Time
}

type ReplaceCommand struct {
	Transcript Transcript
}

type ListOptions struct {
	Limit            int
	IncludeSidechain bool
}

type Store interface {
	Start(ctx context.Context, cmd StartCommand) error
	AppendMessage(ctx context.Context, cmd AppendMessageCommand) error
	PatchState(ctx context.Context, cmd PatchStateCommand) error
	Compact(ctx context.Context, cmd CompactCommand) error
	Fork(ctx context.Context, cmd ForkCommand) error
	Replace(ctx context.Context, cmd ReplaceCommand) error

	Load(ctx context.Context, transcriptID string) (*Transcript, error)
	LoadLatest(ctx context.Context, projectID string) (*Transcript, error)
	List(ctx context.Context, projectID string, opts ListOptions) ([]ListItem, error)
	RebuildIndex(ctx context.Context, projectID string) error
}

func PatchTitle(title string) PatchOp {
	return mustPatch(PatchPathTitle, title)
}

func PatchLastPrompt(prompt string) PatchOp {
	return mustPatch(PatchPathLastPrompt, prompt)
}

func PatchTag(tag string) PatchOp {
	return mustPatch(PatchPathTag, tag)
}

func PatchMode(mode string) PatchOp {
	return mustPatch(PatchPathMode, mode)
}

func PatchSummary(summary string) PatchOp {
	return mustPatch(PatchPathSummary, summary)
}

func PatchTasks(tasks []tracker.Task) PatchOp {
	return mustPatch(PatchPathTasks, tasks)
}

func PatchWorktree(worktree *WorktreeState) PatchOp {
	return mustPatch(PatchPathWorktree, worktree)
}

func mustPatch(path string, v any) PatchOp {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return PatchOp{
		Path:  path,
		Value: data,
	}
}
