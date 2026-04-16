package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/providerui"
	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
)

// updateProvider routes provider connection and selection messages.
func (m *model) updateProvider(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case providerui.ConnectResultMsg:
		c := m.handleProviderConnectResult(msg)
		return c, true
	case providerui.SelectedMsg:
		c := m.handleProviderSelected(msg)
		return c, true
	case providerui.ModelSelectedMsg:
		c := m.handleModelSelected(msg)
		return c, true
	case providerui.ModelsLoadedMsg:
		m.provider.Selector.HandleModelsLoaded(msg)
		return nil, true
	case providerui.StatusExpiredMsg:
		m.provider.StatusMessage = ""
		return nil, true
	}
	return nil, false
}

func (m *model) handleProviderConnectResult(msg providerui.ConnectResultMsg) tea.Cmd {
	return m.provider.Selector.HandleConnectResult(msg)
}

func (m *model) handleProviderSelected(msg providerui.SelectedMsg) tea.Cmd {
	ctx := context.Background()
	result, err := m.provider.Selector.ConnectProvider(ctx, msg.Provider, msg.AuthMethod)
	if err != nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Error: " + err.Error()})
	} else {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: result})
		m.refreshProviderConnection(ctx, msg.Provider, msg.AuthMethod)
	}
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleModelSelected(msg providerui.ModelSelectedMsg) tea.Cmd {
	_, err := m.provider.Selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
	if err != nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Error: " + err.Error()})
		return tea.Batch(m.commitMessages()...)
	}

	m.provider.CurrentModel = &llm.CurrentModelInfo{
		ModelID:    msg.ModelID,
		Provider:   llm.Name(msg.ProviderName),
		AuthMethod: msg.AuthMethod,
	}
	if m.hookEngine != nil {
		m.hookEngine.SetLLMCompleter(buildLLMCompleter(m.provider.LLM), msg.ModelID)
	}
	ctx := context.Background()
	m.refreshProviderConnection(ctx, llm.Name(msg.ProviderName), msg.AuthMethod)

	// Show model name in status bar for 5 seconds
	m.provider.StatusMessage = msg.ModelID
	return providerui.StatusTimer(5 * time.Second)
}

func (m *model) refreshProviderConnection(ctx context.Context, providerName llm.Name, authMethod llm.AuthMethod) {
	p, err := llm.GetProvider(ctx, providerName, authMethod)
	if err != nil {
		log.Logger().Warn("failed to refresh provider connection",
			zap.String("provider", string(providerName)),
			zap.Error(err))
		return
	}
	m.provider.LLM = p
	if m.hookEngine != nil {
		m.hookEngine.SetLLMCompleter(buildLLMCompleter(p), m.getModelID())
	}
	m.reconfigureAgentTool()
}
