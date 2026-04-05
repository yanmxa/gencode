package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appinput "github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/ui/history"
)

type submitRequest struct {
	Input string
}

func (m *model) resetInputField() {
	m.input.Textarea.Reset()
	m.input.Textarea.SetHeight(appinput.MinTextareaHeight())
}

func (m *model) handleSubmit() tea.Cmd {
	m.promptSuggestion.Clear()
	m.maxOutputRecoveryCount = 0

	req, ok := m.readSubmitRequest()
	if !ok {
		return nil
	}

	return m.executeSubmitRequest(req)
}

func (m *model) readSubmitRequest() (submitRequest, bool) {
	if m.conv.Stream.Active {
		return submitRequest{}, false
	}

	input := strings.TrimSpace(m.input.Textarea.Value())
	if input == "" && len(m.input.Images.Pending) == 0 {
		return submitRequest{}, false
	}

	return submitRequest{
		Input: input,
	}, true
}

func (m *model) executeSubmitRequest(req submitRequest) tea.Cmd {
	if isExitRequest(req.Input) {
		cmd, _ := m.quitWithCancel()
		return cmd
	}

	if blocked, reason := m.checkPromptHook(req.Input); blocked {
		return m.blockPromptSubmission(reason)
	}

	m.recordSubmittedInput(req.Input)

	if cmd, handled := m.handleCommandSubmit(req.Input); handled {
		return cmd
	}

	m.skill.ActiveInvocation = ""

	userMsg, cmd, handled := m.prepareSubmittedUserMessage(req.Input)
	if handled {
		return cmd
	}
	m.conv.Append(userMsg)
	m.resetInputField()
	return m.startProviderTurn(userMsg.Content)
}

func isExitRequest(input string) bool {
	return strings.EqualFold(input, "exit")
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
	// before startLLMStream, so skip it here. For regular commands, append now.
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
	return m.startLLMStream(nil)
}
