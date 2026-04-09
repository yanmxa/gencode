package session

import (
	"time"

	"github.com/yanmxa/gencode/internal/tracker"
	"github.com/yanmxa/gencode/internal/transcript"
)

func NormalizeMetadata(meta *SessionMetadata, entries []Entry, defaultCwd string, now time.Time) {
	for i := range entries {
		if entries[i].Type == "" && entries[i].Message != nil {
			entries[i].Type = entryTypeForRole(entries[i].Message.Role)
		}
	}
	if meta.ID == "" {
		meta.ID = generateSessionID()
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}
	meta.UpdatedAt = now
	meta.MessageCount = len(entries)
	if meta.Cwd == "" {
		meta.Cwd = defaultCwd
	}
	if meta.LastPrompt == "" {
		meta.LastPrompt = ExtractLastUserText(entries)
	}
	if meta.Title == "" {
		meta.Title = GenerateTitle(entries)
	}
}

func TranscriptFromSnapshot(sess *Snapshot, nodes []transcript.Node, tasks []tracker.Task) transcript.Transcript {
	return transcript.Transcript{
		ID:        sess.Metadata.ID,
		ParentID:  sess.Metadata.ParentSessionID,
		Cwd:       sess.Metadata.Cwd,
		CreatedAt: sess.Metadata.CreatedAt,
		UpdatedAt: sess.Metadata.UpdatedAt,
		Provider:  sess.Metadata.Provider,
		Model:     sess.Metadata.Model,
		Messages:  nodes,
		State: transcript.State{
			Title:      sess.Metadata.Title,
			LastPrompt: sess.Metadata.LastPrompt,
			Tag:        sess.Metadata.Tag,
			Mode:       sess.Metadata.Mode,
			Summary:    sess.Metadata.Summary,
			Tasks:      transcript.TrackerTaskViewsFromTasks(tasks),
		},
	}
}
