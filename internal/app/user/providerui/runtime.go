package providerui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/util/log"
)

// Runtime defines the callbacks the providerui package needs from the parent app model.
type Runtime interface {
	AppendMessage(msg core.ChatMessage)
	CommitMessages() []tea.Cmd
	OnProviderChanged(p provider.Provider, model *provider.CurrentModelInfo)
}

// Update routes provider connection and selection messages.
func Update(rt Runtime, state *State, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case ConnectResultMsg:
		return state.Selector.HandleConnectResult(msg), true
	case SelectedMsg:
		return handleProviderSelected(rt, state, msg), true
	case ModelSelectedMsg:
		return handleModelSelected(rt, state, msg), true
	case ModelsLoadedMsg:
		state.Selector.HandleModelsLoaded(msg)
		return nil, true
	case StatusExpiredMsg:
		state.StatusMessage = ""
		return nil, true
	}
	return nil, false
}

func handleProviderSelected(rt Runtime, state *State, msg SelectedMsg) tea.Cmd {
	ctx := context.Background()
	result, err := state.Selector.ConnectProvider(ctx, msg.Provider, msg.AuthMethod)
	if err != nil {
		rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: "Error: " + err.Error()})
	} else {
		rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: result})
		refreshProviderConnection(rt, ctx, msg.Provider, msg.AuthMethod)
	}
	return tea.Batch(rt.CommitMessages()...)
}

func handleModelSelected(rt Runtime, state *State, msg ModelSelectedMsg) tea.Cmd {
	_, err := state.Selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
	if err != nil {
		rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: "Error: " + err.Error()})
		return tea.Batch(rt.CommitMessages()...)
	}

	model := &provider.CurrentModelInfo{
		ModelID:    msg.ModelID,
		Provider:   provider.Name(msg.ProviderName),
		AuthMethod: msg.AuthMethod,
	}
	ctx := context.Background()
	p := connectProvider(ctx, provider.Name(msg.ProviderName), msg.AuthMethod)
	rt.OnProviderChanged(p, model)

	// Show model name in status bar for 5 seconds
	state.StatusMessage = msg.ModelID
	return StatusTimer(5 * time.Second)
}

func refreshProviderConnection(rt Runtime, ctx context.Context, providerName provider.Name, authMethod provider.AuthMethod) {
	p := connectProvider(ctx, providerName, authMethod)
	if p != nil {
		rt.OnProviderChanged(p, nil)
	}
}

func connectProvider(ctx context.Context, providerName provider.Name, authMethod provider.AuthMethod) provider.Provider {
	p, err := provider.GetProvider(ctx, providerName, authMethod)
	if err != nil {
		log.Logger().Warn("failed to refresh provider connection",
			zap.String("provider", string(providerName)),
			zap.Error(err))
		return nil
	}
	return p
}
