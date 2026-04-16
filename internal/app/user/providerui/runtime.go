package providerui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/user/searchui"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/util/log"
)

// Runtime defines the callbacks the providerui package needs from the parent app model.
type Runtime interface {
	AppendMessage(msg core.ChatMessage)
	CommitMessages() []tea.Cmd
	SetHookLLMCompleter(p provider.Provider, modelID string)
	ReconfigureAgentTool()
	GetModelID() string
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

// UpdateSearch routes search provider selection messages.
func UpdateSearch(state *State, search *searchui.State, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case searchui.SelectedMsg:
		search.Selector.Cancel()
		state.StatusMessage = fmt.Sprintf("Search engine: %s", msg.Provider)
		return StatusTimer(3 * time.Second), true
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
		refreshProviderConnection(rt, state, ctx, msg.Provider, msg.AuthMethod)
	}
	return tea.Batch(rt.CommitMessages()...)
}

func handleModelSelected(rt Runtime, state *State, msg ModelSelectedMsg) tea.Cmd {
	_, err := state.Selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
	if err != nil {
		rt.AppendMessage(core.ChatMessage{Role: core.RoleNotice, Content: "Error: " + err.Error()})
		return tea.Batch(rt.CommitMessages()...)
	}

	state.CurrentModel = &provider.CurrentModelInfo{
		ModelID:    msg.ModelID,
		Provider:   provider.Name(msg.ProviderName),
		AuthMethod: msg.AuthMethod,
	}
	rt.SetHookLLMCompleter(state.LLM, msg.ModelID)
	ctx := context.Background()
	refreshProviderConnection(rt, state, ctx, provider.Name(msg.ProviderName), msg.AuthMethod)

	// Show model name in status bar for 5 seconds
	state.StatusMessage = msg.ModelID
	return StatusTimer(5 * time.Second)
}

func refreshProviderConnection(rt Runtime, state *State, ctx context.Context, providerName provider.Name, authMethod provider.AuthMethod) {
	p, err := provider.GetProvider(ctx, providerName, authMethod)
	if err != nil {
		log.Logger().Warn("failed to refresh provider connection",
			zap.String("provider", string(providerName)),
			zap.Error(err))
		return
	}
	state.LLM = p
	rt.SetHookLLMCompleter(p, rt.GetModelID())
	rt.ReconfigureAgentTool()
}
