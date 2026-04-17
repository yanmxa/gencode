package app

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
)

func (m *model) saveSession() error {
	if err := m.runtime.EnsureSessionStore(m.cwd); err != nil {
		return err
	}

	if len(m.conv.Messages) == 0 {
		return nil
	}

	entries := session.ConvertToEntries(m.conv.Messages)

	providerName := ""
	modelID := ""
	if m.runtime.CurrentModel != nil {
		providerName = string(m.runtime.CurrentModel.Provider)
		modelID = m.runtime.CurrentModel.ModelID
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         m.runtime.SessionID,
			Provider:   providerName,
			Model:      modelID,
			Cwd:        m.cwd,
			LastPrompt: session.ExtractLastUserText(entries),
			Summary:    m.runtime.SessionSummary,
			Mode:       m.runtime.SessionMode(),
		},
		Entries: entries,
		Tasks:   tracker.DefaultStore.Export(),
	}

	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := m.runtime.SessionStore.Save(sess); err != nil {
		return err
	}

	m.runtime.SessionID = sess.Metadata.ID
	m.initTaskStorage()

	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.SetTranscriptPath(m.runtime.SessionStore.SessionPath(sess.Metadata.ID))
	}

	m.reconfigureAgentTool()

	return nil
}

func (m *model) loadSession(id string) error {
	if err := m.runtime.EnsureSessionStore(m.cwd); err != nil {
		return err
	}

	sess, err := m.runtime.SessionStore.Load(id)
	if err != nil {
		return err
	}

	tracker.DefaultStore.SetStorageDir("")
	m.restoreSessionData(sess)

	if len(sess.Tasks) == 0 {
		tracker.DefaultStore.Reset()
	}
	tool.ResetFetched()

	m.runtime.InputTokens = 0
	m.runtime.OutputTokens = 0

	return nil
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	m.conv.Messages = session.ConvertFromEntries(sess.Entries)
	m.runtime.SessionID = sess.Metadata.ID

	if sess.Metadata.Summary != "" {
		m.runtime.SessionSummary = sess.Metadata.Summary
	} else if m.runtime.SessionStore != nil {
		if mem, err := m.runtime.SessionStore.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			m.runtime.SessionSummary = mem
		}
	}

	m.initTaskStorage()

	if len(sess.Tasks) > 0 {
		tracker.DefaultStore.Import(sess.Tasks)
	}
}

func (m *model) initTaskStorage() {
	if tracker.DefaultStore.GetStorageDir() != "" {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Logger().Warn("failed to get home directory for task storage", zap.Error(err))
		return
	}

	taskListID := os.Getenv("GEN_TASK_LIST_ID")
	if taskListID != "" {
		dir := filepath.Join(homeDir, ".gen", "tasks", taskListID)
		tracker.DefaultStore.SetStorageDir(dir)
		_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
		return
	}

	if m.runtime.SessionID == "" {
		return
	}
	dir := filepath.Join(homeDir, ".gen", "tasks", m.runtime.SessionID)
	tracker.DefaultStore.SetStorageDir(dir)
	_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
}
