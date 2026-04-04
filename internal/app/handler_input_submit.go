package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appinput "github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/ui/history"
)

func (m *model) resetInputField() {
	m.input.Textarea.Reset()
	m.input.Textarea.SetHeight(appinput.MinTextareaHeight())
}

func (m *model) blockPromptSubmission(reason string) tea.Cmd {
	m.conv.Append(message.ChatMessage{
		Role:    message.RoleNotice,
		Content: "Prompt blocked: " + reason,
	})
	m.resetInputField()
	return tea.Batch(m.commitMessages()...)
}

func (m *model) recordSubmittedInput(input string) {
	if input == "" {
		return
	}
	m.input.History = append(m.input.History, input)
	m.input.HistoryIdx = -1
	m.input.TempInput = ""
	history.Save(m.cwd, m.input.History)
}

func (m *model) handleCommandSubmit(input string) (tea.Cmd, bool) {
	result, cmd, isCmd := ExecuteCommand(context.Background(), m, input)
	if !isCmd {
		return nil, false
	}

	m.resetInputField()

	// For skill commands, the user message is appended inside handleSkillInvocation
	// before startStream, so skip it here. For regular commands, append now.
	if cmd == nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleUser, Content: input})
	}
	if result != "" {
		m.conv.AddNotice(result)
	}

	cmds := m.commitMessages()
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...), true
}

func (m *model) prepareSubmittedUserMessage(input string) (message.ChatMessage, tea.Cmd, bool) {
	content, fileImages, err := appinput.ProcessImageRefs(m.cwd, input)
	if err != nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "Image error: " + err.Error()})
		return message.ChatMessage{}, tea.Batch(m.commitMessages()...), true
	}

	allImages := append(m.input.Images.Pending, fileImages...)
	m.input.Images.Pending = nil

	return message.ChatMessage{
		Role:    message.RoleUser,
		Content: content,
		Images:  allImages,
	}, nil, false
}

func (m *model) startProviderTurn(content string) tea.Cmd {
	if m.provider.LLM == nil {
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleNotice,
			Content: "No provider connected. Use /provider to connect.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	m.detectThinkingKeywords(content)
	return m.startStream(nil, true)
}
