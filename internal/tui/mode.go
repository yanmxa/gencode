// Operation mode management: cycling between normal, auto-accept, and plan modes.
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/plugin"
)

func (m *model) cycleOperationMode() {
	m.operationMode = m.operationMode.Next()
	m.applyOperationModePermissions()
	m.planMode = m.operationMode == modePlan

	if m.hookEngine != nil {
		m.hookEngine.SetPermissionMode(m.operationModeName())
	}
}

// applyOperationModePermissions configures session permissions based on the current mode.
func (m *model) applyOperationModePermissions() {
	// Reset all permissions first
	m.sessionPermissions.AllowAllEdits = false
	m.sessionPermissions.AllowAllWrites = false
	m.sessionPermissions.AllowAllBash = false
	m.sessionPermissions.AllowAllSkills = false

	// Enable auto-accept permissions
	if m.operationMode == modeAutoAccept {
		m.sessionPermissions.AllowAllEdits = true
		m.sessionPermissions.AllowAllWrites = true
		for _, pattern := range config.CommonAllowPatterns {
			m.sessionPermissions.AllowPattern(pattern)
		}
	}
}

// operationModeName returns the string name of the current operation mode.
func (m *model) operationModeName() string {
	switch m.operationMode {
	case modeAutoAccept:
		return "auto"
	case modePlan:
		return "plan"
	default:
		return "normal"
	}
}

func (m model) installPlugin(msg PluginInstallMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		installer := plugin.NewInstaller(plugin.DefaultRegistry, m.cwd)
		if err := installer.LoadMarketplaces(); err != nil {
			return PluginInstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		pluginRef := msg.PluginName
		if msg.Marketplace != "" {
			pluginRef = msg.PluginName + "@" + msg.Marketplace
		}

		if err := installer.Install(ctx, pluginRef, msg.Scope); err != nil {
			return PluginInstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		return PluginInstallResultMsg{PluginName: msg.PluginName, Success: true}
	}
}
