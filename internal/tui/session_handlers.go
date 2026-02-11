package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/session"
)

// ensureSessionStore initializes the session store if not already initialized
func (m *model) ensureSessionStore() error {
	if m.sessionStore != nil {
		return nil
	}
	store, err := session.NewStore()
	if err != nil {
		return err
	}
	m.sessionStore = store
	return nil
}

// saveSession saves the current session to disk
func (m *model) saveSession() error {
	if err := m.ensureSessionStore(); err != nil {
		return err
	}

	// Skip if no messages
	if len(m.messages) == 0 {
		return nil
	}

	// Convert messages to stored format
	storedMessages := convertToStoredMessages(m.messages)

	// Get provider and model info
	providerName := ""
	modelID := ""
	if m.currentModel != nil {
		providerName = string(m.currentModel.Provider)
		modelID = m.currentModel.ModelID
	}

	// Build or update session
	sess := &session.Session{
		Metadata: session.SessionMetadata{
			ID:       m.currentSessionID,
			Provider: providerName,
			Model:    modelID,
			Cwd:      m.cwd,
		},
		Messages: storedMessages,
	}

	// Generate title from first user message if new session
	if sess.Metadata.Title == "" || sess.Metadata.ID == "" {
		sess.Metadata.Title = session.GenerateTitle(storedMessages)
	}

	if err := m.sessionStore.Save(sess); err != nil {
		return err
	}

	// Update current session ID
	m.currentSessionID = sess.Metadata.ID
	return nil
}

// loadSession loads a session from disk and restores it
func (m *model) loadSession(id string) error {
	if err := m.ensureSessionStore(); err != nil {
		return err
	}

	sess, err := m.sessionStore.Load(id)
	if err != nil {
		return err
	}

	// Restore messages
	m.messages = convertFromStoredMessages(sess.Messages)
	m.currentSessionID = sess.Metadata.ID

	// Reset token usage (will be updated on next API call)
	m.lastInputTokens = 0
	m.lastOutputTokens = 0

	return nil
}

// convertToStoredMessages converts chatMessages to StoredMessages
func convertToStoredMessages(messages []chatMessage) []session.StoredMessage {
	stored := make([]session.StoredMessage, 0, len(messages))
	for _, msg := range messages {
		stored = append(stored, session.StoredMessage{
			Role:       msg.role,
			Content:    msg.content,
			Thinking:   msg.thinking,
			Images:     msg.images,
			ToolCalls:  msg.toolCalls,
			ToolResult: msg.toolResult,
			ToolName:   msg.toolName,
			IsSummary:  msg.isSummary,
		})
	}
	return stored
}

// convertFromStoredMessages converts StoredMessages to chatMessages
func convertFromStoredMessages(stored []session.StoredMessage) []chatMessage {
	messages := make([]chatMessage, 0, len(stored))
	for _, sm := range stored {
		messages = append(messages, chatMessage{
			role:       sm.Role,
			content:    sm.Content,
			thinking:   sm.Thinking,
			images:     sm.Images,
			toolCalls:  sm.ToolCalls,
			toolResult: sm.ToolResult,
			toolName:   sm.ToolName,
			isSummary:  sm.IsSummary,
		})
	}
	return messages
}

// handleSessionSelected handles when a session is selected from the selector
func (m model) handleSessionSelected(msg SessionSelectedMsg) (tea.Model, tea.Cmd) {
	if err := m.loadSession(msg.SessionID); err != nil {
		// Add error message
		m.messages = append(m.messages, chatMessage{
			role:    roleNotice,
			content: "Failed to load session: " + err.Error(),
		})
	}

	// Commit restored messages to scrollback
	m.committedCount = 0
	return m, tea.Batch(m.commitAllMessages()...)
}
