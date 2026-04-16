// Session management functions (save, load, restore, task storage).
// These are model-level lifecycle operations used by multiple subsystems.
// The session overlay dispatcher lives in update_overlays.go.
package app

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/util/log"
)

func (m *model) ensureSessionStore() error {
	if m.sessionStore == nil {
		store, err := session.NewStore(m.cwd)
		if err != nil {
			return err
		}
		m.sessionStore = store
	}
	return nil
}

func (m *model) saveSession() error {
	if err := m.ensureSessionStore(); err != nil {
		return err
	}

	if len(m.conv.Messages) == 0 {
		return nil
	}

	entries := session.ConvertToEntries(m.conv.Messages)

	providerName := ""
	modelID := ""
	if m.currentModel != nil {
		providerName = string(m.currentModel.Provider)
		modelID = m.currentModel.ModelID
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         m.sessionID,
			Provider:   providerName,
			Model:      modelID,
			Cwd:        m.cwd,
			LastPrompt: session.ExtractLastUserText(entries),
			Summary:    m.sessionSummary,
			Mode:       m.currentSessionMode(),
		},
		Entries: entries,
		Tasks:   tracker.DefaultStore.Export(),
	}

	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := m.sessionStore.Save(sess); err != nil {
		return err
	}

	m.sessionID = sess.Metadata.ID
	m.initTaskStorage()

	if m.hookEngine != nil {
		m.hookEngine.SetTranscriptPath(m.sessionStore.SessionPath(sess.Metadata.ID))
	}

	m.reconfigureAgentTool()

	return nil
}

func (m *model) loadSession(id string) error {
	if err := m.ensureSessionStore(); err != nil {
		return err
	}

	sess, err := m.sessionStore.Load(id)
	if err != nil {
		return err
	}

	tracker.DefaultStore.SetStorageDir("")
	m.restoreSessionData(sess)

	if len(sess.Tasks) == 0 {
		tracker.DefaultStore.Reset()
	}
	tool.ResetFetched()

	m.inputTokens = 0
	m.outputTokens = 0
	m.conv.TurnsSinceLastTaskTool = 0

	return nil
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	m.conv.Messages = session.ConvertFromEntries(sess.Entries)
	m.sessionID = sess.Metadata.ID

	if sess.Metadata.Summary != "" {
		m.sessionSummary = sess.Metadata.Summary
	} else if m.sessionStore != nil {
		if mem, err := m.sessionStore.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			m.sessionSummary = mem
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

	if m.sessionID == "" {
		return
	}
	dir := filepath.Join(homeDir, ".gen", "tasks", m.sessionID)
	tracker.DefaultStore.SetStorageDir(dir)
	_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
}

func (m *model) currentSessionMode() string {
	if m.mode.Enabled {
		return "plan"
	}
	switch m.operationMode {
	case config.ModeAutoAccept:
		return "auto-accept"
	default:
		return "normal"
	}
}
