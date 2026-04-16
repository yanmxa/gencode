package transcript

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/tracker"
)

type fakeBlobReader struct {
	values map[string][]byte
	err    error
}

func (f fakeBlobReader) Get(kind, id string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.values[kind+":"+id], nil
}

func TestProjectStartAndAppendMessages(t *testing.T) {
	now := time.Date(2026, 4, 6, 14, 0, 0, 0, time.UTC)
	transcript, err := Project([]Record{
		{
			ID:           "tx-1:start",
			TranscriptID: "tx-1",
			Time:         now,
			Type:         RecordStarted,
			Cwd:          "/tmp/project",
			System:       &SystemRecord{Provider: "openai", Model: "gpt-test"},
		},
		{
			ID:           "rec-1",
			TranscriptID: "tx-1",
			Time:         now.Add(time.Second),
			Type:         RecordMessageAppended,
			Message:      &MessageRecord{MessageID: "msg-1", Role: "user", Content: []ContentBlock{{Type: "text", Text: "hello"}}},
		},
		{
			ID:           "rec-2",
			TranscriptID: "tx-1",
			Time:         now.Add(2 * time.Second),
			Type:         RecordMessageAppended,
			ParentID:     "msg-1",
			Message:      &MessageRecord{MessageID: "msg-2", Role: "assistant", Content: []ContentBlock{{Type: "text", Text: "world"}}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if transcript.ID != "tx-1" || transcript.Provider != "openai" || transcript.Model != "gpt-test" {
		t.Fatalf("unexpected transcript metadata: %+v", transcript)
	}
	if len(transcript.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(transcript.Messages))
	}
	if transcript.Messages[1].ParentID != "msg-1" {
		t.Fatalf("unexpected parent chain: %+v", transcript.Messages)
	}
}

func TestProjectStatePatchLastWins(t *testing.T) {
	transcript, err := Project([]Record{
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStarted},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStatePatched, State: &StateRecord{Ops: []PatchOp{PatchTitle("A"), patchMode("normal")}}},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStatePatched, State: &StateRecord{Ops: []PatchOp{PatchTitle("B"), patchMode("plan")}}},
	}, nil)
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if transcript.State.Title != "B" || transcript.State.Mode != "plan" {
		t.Fatalf("unexpected state: %+v", transcript.State)
	}
}

func TestProjectTasksAndWorktreePatches(t *testing.T) {
	taskTime := time.Date(2026, 4, 6, 14, 10, 0, 0, time.UTC)
	task := tracker.Task{
		ID:              "1",
		Subject:         "Refactor",
		Status:          tracker.StatusInProgress,
		CreatedAt:       taskTime,
		UpdatedAt:       taskTime,
		StatusChangedAt: taskTime,
	}
	wt := &WorktreeState{OriginalCwd: "/repo", WorktreePath: "/repo/.wt/1", WorktreeName: "fix-1"}
	transcript, err := Project([]Record{
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStarted},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStatePatched, State: &StateRecord{Ops: []PatchOp{PatchTasks([]tracker.Task{task}), patchWorktree(wt)}}},
	}, nil)
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if len(transcript.State.Tasks) != 1 || transcript.State.Tasks[0].Subject != "Refactor" {
		t.Fatalf("unexpected tasks: %+v", transcript.State.Tasks)
	}
	if transcript.State.Worktree == nil || transcript.State.Worktree.WorktreeName != "fix-1" {
		t.Fatalf("unexpected worktree: %+v", transcript.State.Worktree)
	}
}

func TestProjectWorktreeNullClears(t *testing.T) {
	transcript, err := Project([]Record{
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStarted},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStatePatched, State: &StateRecord{Ops: []PatchOp{patchWorktree(&WorktreeState{WorktreeName: "a"})}}},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStatePatched, State: &StateRecord{Ops: []PatchOp{patchWorktree(nil)}}},
	}, nil)
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if transcript.State.Worktree != nil {
		t.Fatalf("expected cleared worktree, got %+v", transcript.State.Worktree)
	}
}

func TestProjectCompactBoundaryTruncatesActiveChain(t *testing.T) {
	now := time.Date(2026, 4, 6, 14, 20, 0, 0, time.UTC)
	transcript, err := Project([]Record{
		{TranscriptID: "tx-1", Time: now, Type: RecordStarted},
		{TranscriptID: "tx-1", Time: now.Add(time.Second), Type: RecordMessageAppended, Message: &MessageRecord{MessageID: "m1", Role: "user"}},
		{TranscriptID: "tx-1", Time: now.Add(2 * time.Second), Type: RecordMessageAppended, ParentID: "m1", Message: &MessageRecord{MessageID: "m2", Role: "assistant"}},
		{TranscriptID: "tx-1", Time: now.Add(3 * time.Second), Type: RecordMessageAppended, ParentID: "m2", Message: &MessageRecord{MessageID: "m3", Role: "user"}},
		{TranscriptID: "tx-1", Time: now.Add(4 * time.Second), Type: RecordCompacted, System: &SystemRecord{BoundaryID: "m2"}},
	}, nil)
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if len(transcript.Messages) != 2 || transcript.Messages[0].ID != "m2" || transcript.Messages[1].ID != "m3" {
		t.Fatalf("unexpected active chain: %+v", transcript.Messages)
	}
}

func TestProjectCompactLoadsSummaryFromBlob(t *testing.T) {
	transcript, err := Project([]Record{
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStarted},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordCompacted, System: &SystemRecord{SummaryBlobID: "blob-1"}},
	}, fakeBlobReader{
		values: map[string][]byte{"summary:blob-1": []byte("summary text")},
	})
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if transcript.State.Summary != "summary text" {
		t.Fatalf("unexpected summary: %q", transcript.State.Summary)
	}
}

func TestProjectUnknownPatchPathReturnsError(t *testing.T) {
	_, err := Project([]Record{
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStarted},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStatePatched, State: &StateRecord{Ops: []PatchOp{{Path: "bad.path", Value: json.RawMessage(`"x"`)}}}},
	}, nil)
	if err == nil {
		t.Fatal("expected error for unknown patch path")
	}
}

func TestProjectLatestLeafWins(t *testing.T) {
	now := time.Date(2026, 4, 6, 14, 30, 0, 0, time.UTC)
	transcript, err := Project([]Record{
		{TranscriptID: "tx-1", Time: now, Type: RecordStarted},
		{TranscriptID: "tx-1", Time: now.Add(time.Second), Type: RecordMessageAppended, Message: &MessageRecord{MessageID: "m1", Role: "user"}},
		{TranscriptID: "tx-1", Time: now.Add(2 * time.Second), Type: RecordMessageAppended, ParentID: "m1", Message: &MessageRecord{MessageID: "m2", Role: "assistant"}},
		{TranscriptID: "tx-1", Time: now.Add(3 * time.Second), Type: RecordMessageAppended, ParentID: "m1", Message: &MessageRecord{MessageID: "m3", Role: "assistant"}},
	}, nil)
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if len(transcript.Messages) != 2 || transcript.Messages[1].ID != "m3" {
		t.Fatalf("expected latest leaf m3, got %+v", transcript.Messages)
	}
}

func TestProjectIgnoresBlobReadErrors(t *testing.T) {
	transcript, err := Project([]Record{
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordStarted},
		{TranscriptID: "tx-1", Time: time.Now(), Type: RecordCompacted, System: &SystemRecord{SummaryBlobID: "blob-1"}},
	}, fakeBlobReader{err: errors.New("boom")})
	if err != nil {
		t.Fatalf("Project(): %v", err)
	}
	if transcript.State.Summary != "" {
		t.Fatalf("expected empty summary on blob read error, got %q", transcript.State.Summary)
	}
}
