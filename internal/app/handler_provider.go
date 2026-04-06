package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appprovider "github.com/yanmxa/gencode/internal/app/provider"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// updateProvider routes provider connection and selection messages.
func (m *model) updateProvider(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appprovider.ConnectResultMsg:
		c := m.handleProviderConnectResult(msg)
		return c, true
	case appprovider.SelectedMsg:
		c := m.handleProviderSelected(msg)
		return c, true
	case appprovider.ModelSelectedMsg:
		c := m.handleModelSelected(msg)
		return c, true
	case appprovider.StatusExpiredMsg:
		m.provider.StatusMessage = ""
		return nil, true
	}
	return nil, false
}

func (m *model) handleProviderConnectResult(msg appprovider.ConnectResultMsg) tea.Cmd {
	m.provider.Selector.HandleConnectResult(msg)
	return nil
}

func (m *model) handleProviderSelected(msg appprovider.SelectedMsg) tea.Cmd {
	ctx := context.Background()
	result, err := m.provider.Selector.ConnectProvider(ctx, msg.Provider, msg.AuthMethod)
	if err != nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "Error: " + err.Error()})
	} else {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: result})
		m.refreshProviderConnection(ctx, msg.Provider, msg.AuthMethod)
	}
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleModelSelected(msg appprovider.ModelSelectedMsg) tea.Cmd {
	_, err := m.provider.Selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
	if err != nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "Error: " + err.Error()})
		return tea.Batch(m.commitMessages()...)
	}

	m.provider.CurrentModel = &provider.CurrentModelInfo{
		ModelID:    msg.ModelID,
		Provider:   provider.Provider(msg.ProviderName),
		AuthMethod: msg.AuthMethod,
	}
	if m.hookEngine != nil {
		m.hookEngine.SetLLMProvider(m.provider.LLM, msg.ModelID)
	}
	ctx := context.Background()
	m.refreshProviderConnection(ctx, provider.Provider(msg.ProviderName), msg.AuthMethod)

	// Show model name in status bar for 5 seconds
	m.provider.StatusMessage = msg.ModelID
	return appprovider.StatusTimer(5 * time.Second)
}

func (m *model) refreshProviderConnection(ctx context.Context, providerName provider.Provider, authMethod provider.AuthMethod) {
	p, err := provider.GetProvider(ctx, providerName, authMethod)
	if err != nil {
		return
	}
	m.provider.LLM = p
	if m.hookEngine != nil {
		m.hookEngine.SetLLMProvider(p, m.getModelID())
	}
	m.reconfigureAgentTool()
}
