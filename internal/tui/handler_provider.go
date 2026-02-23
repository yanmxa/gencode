package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/tool"
)

func (m *model) handleProviderConnectResult(msg ProviderConnectResultMsg) (tea.Model, tea.Cmd) {
	m.selector.HandleConnectResult(msg)
	return m, nil
}

func (m *model) handleProviderSelected(msg ProviderSelectedMsg) (tea.Model, tea.Cmd) {
	ctx := context.Background()
	result, err := m.selector.ConnectProvider(ctx, msg.Provider, msg.AuthMethod)
	if err != nil {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "Error: " + err.Error()})
	} else {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: result})
		if p, err := provider.GetProvider(ctx, msg.Provider, msg.AuthMethod); err == nil {
			m.llmProvider = p
			modelID := ""
			if m.currentModel != nil {
				modelID = m.currentModel.ModelID
			}
			configureTaskTool(p, m.cwd, modelID, m.hookEngine)
		}
	}
	return m, tea.Batch(m.commitMessages()...)
}

func (m *model) handleModelSelected(msg ModelSelectedMsg) (tea.Model, tea.Cmd) {
	result, err := m.selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
	if err != nil {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "Error: " + err.Error()})
	} else {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: result})
		m.currentModel = &provider.CurrentModelInfo{
			ModelID:    msg.ModelID,
			Provider:   provider.Provider(msg.ProviderName),
			AuthMethod: msg.AuthMethod,
		}
		ctx := context.Background()
		if p, err := provider.GetProvider(ctx, provider.Provider(msg.ProviderName), msg.AuthMethod); err == nil {
			m.llmProvider = p
			configureTaskTool(p, m.cwd, msg.ModelID, m.hookEngine)
		}
	}
	return m, tea.Batch(m.commitMessages()...)
}

func configureTaskTool(llmProvider provider.LLMProvider, cwd string, modelID string, hookEngine *hooks.Engine) {
	if t, ok := tool.Get("Task"); ok {
		if taskTool, ok := t.(*tool.TaskTool); ok {
			executor := agent.NewExecutor(llmProvider, cwd, modelID, hookEngine)
			adapter := agent.NewExecutorAdapter(executor)
			taskTool.SetExecutor(adapter)
		}
	}
}
