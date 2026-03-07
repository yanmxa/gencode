package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	appsession "github.com/yanmxa/gencode/internal/app/session"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/tool"
)

// ensureSessionStore initializes the session store if not already initialized
func (m *model) ensureSessionStore() error {
	if m.session.Store != nil {
		return nil
	}
	store, err := session.NewStore(m.cwd)
	if err != nil {
		return err
	}
	m.session.Store = store
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
	entries := appsession.ConvertToEntries(m.conv.Messages)

	// Get provider and model info
	providerName := ""
	modelID := ""
	if m.provider.CurrentModel != nil {
		providerName = string(m.provider.CurrentModel.Provider)
		modelID = m.provider.CurrentModel.ModelID
	}

	// Build or update session
	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:       m.session.CurrentID,
			Provider: providerName,
			Model:    modelID,
			Cwd:      m.cwd,
		},
		Entries: entries,
		Tasks:   tool.DefaultTodoStore.Export(),
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

	// Reconfigure task tool with updated parent session ID so subsequent
	// subagent invocations link to this session.
	if m.provider.LLM != nil {
		appprovider.ConfigureTaskTool(m.provider.LLM, m.cwd, m.getModelID(), m.hookEngine, m.session.Store, m.session.CurrentID)
	}

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

	// Reset tasks if none in session (switching sessions at runtime)
	if len(sess.Tasks) == 0 {
		tool.DefaultTodoStore.Reset()
	}

	// Reset token usage (will be updated on next API call)
	m.provider.InputTokens = 0
	m.provider.OutputTokens = 0

	return nil
}

// restoreSessionData restores conversation state from a loaded session.
// Used by both loadSession (runtime) and newModel (initialization).
func (m *model) restoreSessionData(sess *session.Session) {
	m.conv.Messages = appsession.ConvertFromEntries(sess.Entries)
	m.session.CurrentID = sess.Metadata.ID

	// Load session memory (persisted compaction summary)
	if m.session.Store != nil {
		if mem, err := m.session.Store.LoadSessionMemory(sess.Metadata.ID); err == nil && mem != "" {
			m.session.Summary = mem
		}
	}

	// Restore tasks
	if len(sess.Tasks) > 0 {
		tool.DefaultTodoStore.Import(sess.Tasks)
	}
}

// updateSession routes session selection messages.
func (m *model) updateSession(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appsession.SelectedMsg:
		c := m.handleSessionSelected(msg)
		return c, true
	}
	return nil, false
}

// handleSessionSelected handles when a session is selected from the selector
func (m *model) handleSessionSelected(msg appsession.SelectedMsg) tea.Cmd {
	if err := m.loadSession(msg.SessionID); err != nil {
		m.conv.AddNotice("Failed to load session: " + err.Error())
	}

	// Commit restored messages to scrollback
	m.conv.CommittedCount = 0
	return tea.Batch(m.commitAllMessages()...)
}
