// Session persistence and per-session task storage.
// Save/load conversations + plan snapshots to disk, wire the plan store's
// storage directory, fork a fresh session from the current one.
package app

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/genai-io/san/internal/app/conv"
	"github.com/genai-io/san/internal/confdir"
	"github.com/genai-io/san/internal/log"
	"github.com/genai-io/san/internal/session"
	"github.com/genai-io/san/internal/setting"
)

func (m *model) InitTaskStorage() {
	m.initTaskStorage(m.services.Session.ID())
}

func (m *model) PersistSession() error {
	if err := m.services.Session.EnsureStore(m.env.CWD); err != nil {
		return err
	}
	sess := m.buildSessionSnapshot()
	if sess == nil {
		return nil
	}

	if err := m.services.Session.Save(sess); err != nil {
		return err
	}

	m.services.Session.SetID(sess.Metadata.ID)
	m.initTaskStorage(m.services.Session.ID())

	m.services.Hook.SetTranscriptPath(m.services.Session.GetStore().SessionPath(sess.Metadata.ID))
	m.ReconfigureAgentTool()

	return nil
}

type persistSessionDoneMsg struct{ err error }

// Only safe when the session ID is already established (i.e. not the first save).
func (m *model) persistSessionCmd() tea.Cmd {
	if err := m.services.Session.EnsureStore(m.env.CWD); err != nil {
		log.Logger().Warn("failed to ensure session store for async persist", zap.Error(err))
		return nil
	}
	sess := m.buildSessionSnapshot()
	if sess == nil {
		return nil
	}

	store := m.services.Session.GetStore()
	return func() tea.Msg {
		if store == nil {
			return persistSessionDoneMsg{err: fmt.Errorf("no session store")}
		}
		return persistSessionDoneMsg{err: store.Save(sess)}
	}
}

// persistAfterTurn saves the session at the end of a turn, choosing the
// strategy by session state: the first save (no ID yet) runs synchronously so
// the session ID, task storage, and transcript wiring are established before
// the next turn; later saves go through the async command. Returns the async
// command to batch, or nil when the synchronous path ran (it logs its own
// error, matching how async failures surface via persistSessionDoneMsg).
func (m *model) persistAfterTurn() tea.Cmd {
	if m.services.Session.ID() != "" {
		return m.persistSessionCmd()
	}
	if err := m.PersistSession(); err != nil {
		log.Logger().Warn("failed to persist session at turn end", zap.Error(err))
	}
	return nil
}

// buildSessionSnapshot assembles the current conversation + task state into a
// session.Snapshot ready to Save, or nil when there is nothing to persist (no
// messages). Shared by the synchronous PersistSession and the async
// persistSessionCmd so the snapshot stays identical across both paths.
func (m *model) buildSessionSnapshot() *session.Snapshot {
	if len(m.conv.Messages) == 0 {
		return nil
	}

	entries := session.ConvertToEntries(m.conv.ConvertToProvider())

	var providerName, modelID string
	if m.env.CurrentModel != nil {
		providerName = string(m.env.CurrentModel.Provider)
		modelID = m.env.CurrentModel.ModelID
	}

	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         m.services.Session.ID(),
			Provider:   providerName,
			Model:      modelID,
			Cwd:        m.env.CWD,
			LastPrompt: session.ExtractLastUserText(entries),
			Mode:       m.env.SessionMode(),
		},
		Entries: entries,
		Tasks:   m.services.Plan.Export(),
	}

	if sess.Metadata.Title == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	return sess
}

func (m *model) loadSessionByID(id string) error {
	if err := m.services.Session.EnsureStore(m.env.CWD); err != nil {
		return err
	}

	sess, err := m.services.Session.Load(id)
	if err != nil {
		return err
	}

	m.services.Plan.SetStorageDir("")
	m.restoreSessionData(sess)

	if len(sess.Tasks) == 0 {
		m.services.Plan.Reset()
	}

	m.env.InputTokens = 0
	m.env.OutputTokens = 0

	return nil
}

func (m *model) restoreSessionData(sess *session.Snapshot) {
	m.conv.Messages = conv.ChatMessagesFromCore(session.ConvertFromEntries(sess.Entries))
	m.services.Session.SetID(sess.Metadata.ID)

	m.initTaskStorage(m.services.Session.ID())

	if len(sess.Tasks) > 0 {
		m.services.Plan.Import(sess.Tasks)
	}
}

func (m *model) initTaskStorage(sessionID string) {
	if dir := m.services.Plan.GetStorageDir(); dir != "" {
		m.configureBackgroundTaskOutput(dir)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Logger().Warn("failed to get home directory for task storage", zap.Error(err))
		return
	}

	taskListID := setting.Getenv("TASK_LIST_ID")
	if taskListID != "" {
		dir := filepath.Join(confdir.Dir(homeDir), "tasks", taskListID)
		m.configureTaskStorage(dir)
		return
	}

	if sessionID == "" {
		return
	}
	dir := filepath.Join(confdir.Dir(homeDir), "tasks", sessionID)
	m.configureTaskStorage(dir)
}

func (m *model) configureTaskStorage(dir string) {
	if err := m.services.Plan.SetStorageDir(dir); err != nil {
		log.Logger().Warn("failed to configure plan storage", zap.String("dir", dir), zap.Error(err))
		return
	}
	m.configureBackgroundTaskOutput(dir)
}

func (m *model) configureBackgroundTaskOutput(planDir string) {
	if err := m.services.BackgroundTasks.SetOutputDir(filepath.Join(planDir, "outputs")); err != nil {
		log.Logger().Warn("failed to configure background task output", zap.String("dir", planDir), zap.Error(err))
	}
}

func (m *model) forkSession() (string, error) {
	if m.services.Session.ID() == "" {
		return "", fmt.Errorf("no active session to fork")
	}
	forked, err := m.services.Session.Fork(m.services.Session.ID())
	if err != nil {
		return "", err
	}
	originalID := forked.Metadata.ParentSessionID
	m.services.Session.SetID(forked.Metadata.ID)
	if err := m.services.Plan.SetStorageDir(""); err != nil {
		return "", fmt.Errorf("detach plan storage: %w", err)
	}
	if err := m.services.BackgroundTasks.SetOutputDir(""); err != nil {
		return "", fmt.Errorf("detach background task output: %w", err)
	}
	m.initTaskStorage(forked.Metadata.ID)
	m.services.Plan.Import(forked.Tasks)
	return originalID, nil
}
