package app

import (
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/sessionui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tracker"
)

// ensureSessionStore initializes the session store if not already initialized
func (m *model) ensureSessionStore() error {
	if m.session.Store == nil {
		store, err := session.NewStore(m.cwd)
		if err != nil {
			return err
		}
		m.session.Store = store
	}
	return nil
}

// saveSession saves the current session to disk
func (m *model) saveSession() error {
	if err := m.ensureSessionStore(); err != nil {
		return err
	}

	// Skip if no messages
	if len(m.conv.Messages) == 0 {
		return nil
	}

	// Convert messages to entry format
	entries := session.ConvertToEntries(m.conv.Messages)

	// Get provider and model info
	providerName := ""
	modelID := ""
	if m.provider.CurrentModel != nil {
		providerName = string(m.provider.CurrentModel.Provider)
		modelID = m.provider.CurrentModel.ModelID
	}

	// Build or update session
	sess := &session.Snapshot{
		Metadata: session.SessionMetadata{
			ID:         m.session.CurrentID,
			Provider:   providerName,
			Model:      modelID,
			Cwd:        m.cwd,
			LastPrompt: session.ExtractLastUserText(entries),
			Summary:    m.session.Summary,
			Mode:       m.currentSessionMode(),
		},
		Entries: entries,
		Tasks:   tracker.DefaultStore.Export(),
	}

	// Generate title from first user message if new session
	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(sess.Entries)
	}

	if err := m.session.Store.Save(sess); err != nil {
		return err
	}

	// Update current session ID
	m.session.CurrentID = sess.Metadata.ID
	m.initTaskStorage()

	// Set transcript path on hook engine so all subsequent hook events include it
	if m.hookEngine != nil {
		m.hookEngine.SetTranscriptPath(m.session.Store.SessionPath(sess.Metadata.ID))
	}

	m.reconfigureAgentTool()

	return nil
}

// loadSession loads a session from disk and restores it
func (m *model) loadSession(id string) error {
	if err := m.ensureSessionStore(); err != nil {
		return err
	}

	sess, err := m.session.Store.Load(id)
	if err != nil {
		return err
	}

	m.restoreSessionData(sess)

	// Reset tasks if none in session (switching sessions at runtime).
	if len(sess.Tasks) == 0 {
		tracker.DefaultStore.Reset()
	}
	// Reset deferred tool state for new session context
	tool.ResetFetched()

	// Reset token usage (will be updated on next API call)
	m.provider.InputTokens = 0
	m.provider.OutputTokens = 0

	// Reset task reminder counter so new session starts fresh
	m.conv.TurnsSinceLastTaskTool = 0

	return nil
}

// restoreSessionData restores conversation state from a loaded session.
// Used by both loadSession (runtime) and newModel (initialization).
func (m *model) restoreSessionData(sess *session.Snapshot) {
	m.conv.Messages = session.ConvertFromEntries(sess.Entries)
	m.session.CurrentID = sess.Metadata.ID

	// Load session memory (persisted compaction summary)
	if sess.Metadata.Summary != "" {
		m.session.Summary = sess.Metadata.Summary
	} else if m.session.Store != nil {
		if mem, err := m.session.Store.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			m.session.Summary = mem
		}
	}

	// Init task storage for this session
	m.initTaskStorage()

	// Restore tasks
	if len(sess.Tasks) > 0 {
		tracker.DefaultStore.Import(sess.Tasks)
	}
}

// initTaskStorage sets up disk-based task persistence for the current session.
// If GEN_TASK_LIST_ID is set, uses that as the storage directory name instead of
// the session ID, enabling cross-session task sharing.
func (m *model) initTaskStorage() {
	// Already initialized for this session
	if tracker.DefaultStore.GetStorageDir() != "" {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// GEN_TASK_LIST_ID allows sharing tasks across concurrent sessions
	taskListID := os.Getenv("GEN_TASK_LIST_ID")
	if taskListID != "" {
		dir := filepath.Join(homeDir, ".gen", "tasks", taskListID)
		tracker.DefaultStore.SetStorageDir(dir)
		_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
		return
	}

	if m.session.CurrentID == "" {
		return
	}
	dir := filepath.Join(homeDir, ".gen", "tasks", m.session.CurrentID)
	tracker.DefaultStore.SetStorageDir(dir)
	_ = task.SetOutputDir(filepath.Join(dir, "outputs"))
}

func (m *model) currentSessionMode() string {
	if m.mode.Enabled {
		return "plan"
	}
	switch m.mode.Operation {
	case config.ModeAutoAccept:
		return "auto-accept"
	default:
		return "normal"
	}
}

// updateSession routes session selection messages.
func (m *model) updateSession(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case sessionui.SelectedMsg:
		c := m.handleSessionSelected(msg)
		return c, true
	}
	return nil, false
}

// handleSessionSelected handles when a session is selected from the selector
func (m *model) handleSessionSelected(msg sessionui.SelectedMsg) tea.Cmd {
	sessionID := msg.SessionID

	// If fork is pending, fork the selected session instead of resuming it directly.
	if m.session.PendingFork {
		m.session.PendingFork = false
		if err := m.ensureSessionStore(); err != nil {
			m.conv.AddNotice("Failed to fork session: " + err.Error())
			return nil
		}
		forked, err := m.session.Store.Fork(sessionID)
		if err != nil {
			m.conv.AddNotice("Failed to fork session: " + err.Error())
			return nil
		}
		sessionID = forked.Metadata.ID
	}

	if err := m.loadSession(sessionID); err != nil {
		m.conv.AddNotice("Failed to load session: " + err.Error())
	}

	// Commit restored messages to scrollback
	m.conv.CommittedCount = 0
	return tea.Batch(m.commitAllMessages()...)
}
