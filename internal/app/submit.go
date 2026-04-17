package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/input"
	appcommand "github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/core"
)

func (m *model) handleSubmit() tea.Cmd {
	return input.HandleSubmit(m.submitDeps())
}

func (m *model) drainInputQueue() tea.Cmd {
	return input.DrainInputQueue(m.submitDeps())
}

func (m *model) executeSubmitRequest(req submitRequest) tea.Cmd {
	return input.ExecuteSubmitRequest(m.submitDeps(), req)
}

func (m *model) blockPromptSubmission(reason string) tea.Cmd {
	return input.BlockPromptSubmission(m.submitDeps(), reason)
}

func (m *model) prepareSubmittedUserMessage(rawInput string) (core.ChatMessage, tea.Cmd, bool) {
	return input.PrepareSubmittedUserMessage(m.submitDeps(), rawInput)
}

func (m *model) submitDeps() input.SubmitDeps {
	return input.SubmitDeps{
		Input:          &m.userInput,
		Conversation:   &m.conv,
		Runtime:        &m.runtime,
		Cwd:            m.cwd,
		CommitMessages: m.commitMessages,
		HandleCommand: func(text string) (tea.Cmd, bool) {
			return m.commands().handleSubmit(text)
		},
		QuitWithCancel:    m.quitWithCancel,
		StartProviderTurn: m.startProviderTurn,
	}
}

// startProviderTurn starts an LLM turn by sending the user message to the agent.
func (m *model) startProviderTurn(content string) tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "No provider connected. Use /provider to connect.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	if err := m.ensureAgentSession(); err != nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "Failed to start agent: " + err.Error(),
		})
		return tea.Batch(m.commitMessages()...)
	}

	m.runtime.DetectThinkingKeywords(content)

	var images []core.Image
	if len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		images = lastMsg.Images
	}

	return m.sendToAgent(content, images)
}

func shouldPreserveCommandInConversation(text string) bool {
	name, _, isCmd := appcommand.ParseCommand(text)
	if !isCmd {
		return false
	}
	switch name {
	case "clear", "exit":
		return false
	}
	if _, ok := input.LookupSkillCommand(name); ok {
		return false
	}
	if _, ok := appcommand.IsCustomCommand(name); ok {
		return false
	}
	return true
}

func shouldPreserveBeforeCommandExecution(text string) bool {
	name, _, isCmd := appcommand.ParseCommand(text)
	if !isCmd {
		return false
	}
	return name == "loop"
}

func isExitRequest(input string) bool {
	return strings.EqualFold(input, "exit")
}
