// Source 1 overlay message routing.
//
// Key dispatch and submit handling remain in root app/ (keypress.go, submit.go)
// because they are cross-cutting — touching conv, mode, agent session, and runtime.
// This file routes overlay-specific messages (provider, MCP, plugin, session, etc.)
// that only mutate user.Model state through injected Runtime callbacks.
package input

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Update routes Source 1 overlay messages to the appropriate handler.
func Update(rt Runtime, state *Model, msg tea.Msg) (tea.Cmd, bool) {
	if cmd, ok := UpdateProvider(rt, &state.Provider, msg); ok {
		return cmd, true
	}
	if cmd, ok := UpdateMCP(rt, &state.MCP, msg); ok {
		return cmd, true
	}
	if cmd, ok := UpdatePlugin(rt, &state.Plugin, msg); ok {
		return cmd, true
	}
	if cmd, ok := UpdateSession(rt, &state.Session, msg); ok {
		return cmd, true
	}
	if cmd, ok := UpdateMemory(rt, &state.Memory, msg); ok {
		return cmd, true
	}
	if cmd, ok := UpdateSearch(rt, &state.Search, msg); ok {
		return cmd, true
	}
	return nil, false
}
