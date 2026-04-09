// handler_command_session.go contains session-related command handlers:
// /clear, /fork, and /resume.
package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tracker"
)

func handleClearCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	m.conv.Clear()
	m.provider.InputTokens = 0
	m.provider.OutputTokens = 0
	tracker.DefaultStore.Reset()
	tool.ResetFetched()
	m.cronQueue = nil
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		_, _ = tty.WriteString("\033[2J\033[3J\033[H")
		_ = tty.Close()
	}
	if os.Getenv("TMUX") != "" {
		_ = exec.Command("tmux", "clear-history").Run()
	}
	return "", tea.ClearScreen, nil
}

func handleForkCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if len(m.conv.Messages) == 0 {
		return "Nothing to fork — no messages in current session.", nil, nil
	}

	// Save current session first so all messages are persisted.
	if err := m.saveSession(); err != nil {
		return "", nil, fmt.Errorf("failed to save session before fork: %w", err)
	}

	if m.session.CurrentID == "" {
		return "No active session to fork.", nil, nil
	}

	forked, err := m.session.Store.Fork(m.session.CurrentID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fork session: %w", err)
	}

	// Switch to the forked session.
	m.session.CurrentID = forked.Metadata.ID
	m.session.Summary = ""
	tracker.DefaultStore.SetStorageDir("")
	m.initTaskStorage()

	m.reconfigureAgentTool()

	originalID := forked.Metadata.ParentSessionID
	return fmt.Sprintf("Forked conversation. You are now in the fork.\nTo resume the original: gen -r %s", originalID), nil, nil
}

func handleResumeCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if err := m.ensureSessionStore(); err != nil {
		return "", nil, fmt.Errorf("failed to initialize session store: %w", err)
	}
	if err := m.session.Selector.EnterSelect(m.width, m.height, m.session.Store, m.cwd); err != nil {
		return "", nil, fmt.Errorf("failed to open session selector: %w", err)
	}
	return "", nil, nil
}
