// Bubble Tea Update: top-level message dispatch to specialized handlers.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tui/progress"
)

func (m *model) updateTextareaHeight() {
	content := m.textarea.Value()
	lines := strings.Count(content, "\n") + 1

	newHeight := lines
	if newHeight < minTextareaHeight {
		newHeight = minTextareaHeight
	}
	if newHeight > maxTextareaHeight {
		newHeight = maxTextareaHeight
	}

	m.textarea.SetHeight(newHeight)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ProviderConnectResultMsg:
		return m.handleProviderConnectResult(msg)

	case ProviderSelectedMsg:
		return m.handleProviderSelected(msg)

	case SelectorCancelledMsg,
		ToolToggleMsg, ToolSelectorCancelledMsg,
		SkillCycleMsg, SkillSelectorCancelledMsg,
		AgentToggleMsg, AgentSelectorCancelledMsg:
		return m, nil

	case MCPConnectMsg:
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetDisabled(msg.ServerName, false)
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return m, StartMCPConnect(msg.ServerName)

	case MCPConnectResultMsg:
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, false)
			if !msg.Success && msg.Error != nil {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, msg.Error.Error())
			} else {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, "")
			}
		}
		m.mcpSelector.HandleConnectResult(msg)
		if !m.mcpSelector.IsActive() && !msg.Success {
			content := fmt.Sprintf("Failed to connect to '%s': %v", msg.ServerName, msg.Error)
			m.messages = append(m.messages, chatMessage{role: roleNotice, content: content})
			return m, tea.Batch(m.commitMessages()...)
		}
		return m, nil

	case MCPDisconnectMsg:
		m.mcpSelector.HandleDisconnect(msg.ServerName)
		return m, nil

	case MCPReconnectMsg:
		m.mcpSelector.HandleReconnect(msg.ServerName)
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return m, StartMCPConnect(msg.ServerName)

	case MCPRemoveMsg:
		m.mcpSelector.HandleRemove(msg.ServerName)
		return m, nil

	case MCPAddRequestMsg:
		m.textarea.SetValue("/mcp add ")
		return m, nil

	case PluginEnableMsg:
		m.pluginSelector.HandleEnable(msg.PluginName)
		return m, nil

	case PluginDisableMsg:
		m.pluginSelector.HandleDisable(msg.PluginName)
		return m, nil

	case PluginUninstallMsg:
		m.pluginSelector.HandleUninstall(msg.PluginName)
		return m, nil

	case PluginInstallMsg:
		return m, m.installPlugin(msg)

	case PluginInstallResultMsg:
		m.pluginSelector.HandleInstallResult(msg)
		if msg.Success {
			agent.Init(m.cwd)
		}
		return m, nil

	case MarketplaceRemoveMsg:
		m.pluginSelector.HandleMarketplaceRemove(msg.ID)
		return m, nil

	case MarketplaceSyncResultMsg:
		m.pluginSelector.HandleMarketplaceSync(msg)
		return m, nil

	case MCPSelectorCancelledMsg, SessionSelectorCancelledMsg, MemorySelectorCancelledMsg, PluginSelectorCancelledMsg:
		return m, nil

	case SessionSelectedMsg:
		return m.handleSessionSelected(msg)

	case MemorySelectedMsg:
		return m.handleMemorySelected(msg)

	case SkillInvokeMsg:
		if sk, ok := skill.DefaultRegistry.Get(msg.SkillName); ok {
			executeSkillCommand(&m, sk, "")
			return m.handleSkillInvocation()
		}
		return m, nil

	case ModelSelectedMsg:
		return m.handleModelSelected(msg)

	case PermissionRequestMsg:
		return m.handlePermissionRequest(msg)

	case PermissionResponseMsg:
		return m.handlePermissionResponse(msg)

	case ResultMsg:
		return m.handleToolResult(msg)

	case progress.Msg:
		return m.handleTaskProgress(msg)

	case progress.TickMsg:
		return m.handleTaskProgressTick()

	case StartMsg:
		return m.handleStartToolExecution(msg)

	case CompletedMsg:
		return m.handleAllToolsCompleted()

	case QuestionRequestMsg:
		return m.handleQuestionRequest(msg)

	case QuestionResponseMsg:
		return m.handleQuestionResponse(msg)

	case PlanRequestMsg:
		return m.handlePlanRequest(msg)

	case PlanResponseMsg:
		return m.handlePlanResponse(msg)

	case EnterPlanRequestMsg:
		return m.handleEnterPlanRequest(msg)

	case EnterPlanResponseMsg:
		return m.handleEnterPlanResponse(msg)

	case TokenLimitResultMsg:
		return m.handleTokenLimitResult(msg)

	case CompactResultMsg:
		return m.handleCompactResult(msg)

	case EditorFinishedMsg:
		return m.handleEditorFinished(msg)

	case tea.KeyMsg:
		result, cmd := m.handleKeypress(msg)
		if result != nil {
			return result, cmd
		}
		if cmd != nil {
			return m, cmd
		}

	case tea.WindowSizeMsg:
		return m.handleWindowResize(msg)

	case streamChunkMsg:
		return m.handleStreamChunk(msg)

	case streamContinueMsg:
		return m.handleStreamContinue(msg)

	case streamDoneMsg:
		m.streaming = false
		m.streamChan = nil
		m.cancelFunc = nil
		cmds = append(cmds, m.commitMessages()...)

	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)
	}

	var cmd tea.Cmd
	prevValue := m.textarea.Value()
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	if m.textarea.Value() != prevValue {
		m.updateTextareaHeight()
		m.suggestions.UpdateSuggestions(m.textarea.Value())
	}

	if m.streaming || m.fetchingTokenLimits || m.compacting {
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}
