// Thin dispatchers for overlay feature messages (MCP, memory, plugin, provider, session, search).
// Each overlay sub-package defines its own Update(rt, state, msg) function.
// This file implements the Runtime interfaces each sub-package requires.
package app

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	appmemory "github.com/yanmxa/gencode/internal/app/user/memory"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/app/user/providerui"
	"github.com/yanmxa/gencode/internal/app/user/searchui"
	"github.com/yanmxa/gencode/internal/app/user/sessionui"
	"github.com/yanmxa/gencode/internal/provider"
)

// --- Dispatchers ---

func (m *model) updateMCP(msg tea.Msg) (tea.Cmd, bool) {
	return mcpui.Update(m, &m.mcp, msg)
}

func (m *model) updateMemory(msg tea.Msg) (tea.Cmd, bool) {
	return appmemory.Update(m, &m.memory, msg)
}

func (m *model) updatePlugin(msg tea.Msg) (tea.Cmd, bool) {
	return pluginui.Update(m, &m.plugin, msg)
}

func (m *model) updateProvider(msg tea.Msg) (tea.Cmd, bool) {
	return providerui.Update(m, &m.provider, msg)
}

func (m *model) updateSearch(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case searchui.SelectedMsg:
		m.search.Cancel()
		m.provider.StatusMessage = fmt.Sprintf("Search engine: %s", msg.Provider)
		return providerui.StatusTimer(3 * time.Second), true
	}
	return nil, false
}

func (m *model) updateSession(msg tea.Msg) (tea.Cmd, bool) {
	return sessionui.Update(m, &m.session, msg)
}

// --- Runtime interface implementations ---
// Each overlay sub-package defines a Runtime interface. The *model
// satisfies all of them through the methods below. Shared methods
// (AppendMessage, CommitMessages, etc.) are defined once and satisfy
// multiple interfaces.

// pluginui.Runtime
func (m *model) GetCwd() string                { return m.cwd }
func (m *model) ReloadPluginBackedState() error { return m.reloadPluginBackedState() }

// memory.Runtime
func (m *model) RefreshMemoryContext(trigger string) { m.refreshMemoryContext(trigger) }
func (m *model) FireFileChanged(path, tool string)   { m.fireFileChanged(path, tool) }

// mcpui.Runtime
func (m *model) SetInputText(text string) { m.userInput.Textarea.SetValue(text) }

// providerui.Runtime
func (m *model) SetHookLLMCompleter(p provider.Provider, modelID string) {
	if m.hookEngine != nil {
		m.hookEngine.SetLLMCompleter(buildLLMCompleter(p), modelID)
	}
}
func (m *model) ReconfigureAgentTool() { m.reconfigureAgentTool() }
func (m *model) GetModelID() string    { return m.getModelID() }

// sessionui.Runtime
func (m *model) EnsureSessionStore() error { return m.ensureSessionStore() }
func (m *model) ForkSession(id string) (string, error) {
	forked, err := m.session.Store.Fork(id)
	if err != nil {
		return "", err
	}
	return forked.Metadata.ID, nil
}
func (m *model) LoadSession(id string) error { return m.loadSession(id) }
func (m *model) ResetCommitIndex()           { m.conv.CommittedCount = 0 }
func (m *model) CommitAllMessages() []tea.Cmd { return m.commitAllMessages() }

// startExternalEditor is a thin wrapper kept for command handler reuse.
func startExternalEditor(filePath string) tea.Cmd {
	return appmemory.StartExternalEditor(filePath, func(err error) tea.Msg {
		return appmemory.EditorFinishedMsg{Err: err}
	})
}
