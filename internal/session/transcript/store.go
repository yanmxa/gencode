package transcript

import (
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/task/tracker"
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

func PatchTitle(title string) PatchOp {
	return mustPatch(PatchPathTitle, title)
}

func PatchLastPrompt(prompt string) PatchOp {
	return mustPatch(PatchPathLastPrompt, prompt)
}

func patchTag(tag string) PatchOp {
	return mustPatch(PatchPathTag, tag)
}

func patchMode(mode string) PatchOp {
	return mustPatch(PatchPathMode, mode)
}

func PatchTasks(tasks []tracker.Task) PatchOp {
	return mustPatch(PatchPathTasks, tasks)
}

func patchWorktree(worktree *WorktreeState) PatchOp {
	return mustPatch(PatchPathWorktree, worktree)
}

func mustPatch(path string, v any) PatchOp {
	data, err := json.Marshal(v)
	if err != nil {
		// Log instead of panicking — the marshal input is always controlled
		// (strings, simple structs), so this should never happen in practice.
		log.Logger().Error("transcript: mustPatch marshal failed", zap.String("path", path), zap.Error(err))
		return PatchOp{Path: path, Value: []byte("null")}
	}
	return PatchOp{
		Path:  path,
		Value: data,
	}
}
