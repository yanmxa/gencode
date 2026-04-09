package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appcommand "github.com/yanmxa/gencode/internal/app/command"
	appinput "github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/ui/history"
)

type submitRequest struct {
	Input string
}

func (m *model) resetInputField() {
	m.input.Textarea.Reset()
	m.input.Textarea.SetHeight(appinput.MinTextareaHeight())
	m.input.ClearPaste()
	m.input.ClearImages()
	m.queueSelectIdx = -1
	m.queueTempInput = ""
}

func (m *model) handleSubmit() tea.Cmd {
	m.promptSuggestion.Clear()

	input := strings.TrimSpace(m.input.FullValue())
	if input == "" && len(m.input.Images.Pending) == 0 {
		return nil
	}

	// If the LLM is busy (streaming or tools running), enqueue instead of blocking
	if m.isTurnActive() {
		m.enqueueCurrentInput(input)
		return nil
	}

	m.maxOutputRecoveryCount = 0
	m.conv.Compact.ClearResult()

	return m.executeSubmitRequest(submitRequest{Input: input})
}

// isTurnActive returns true when the LLM is streaming or tools are executing.
func (m *model) isTurnActive() bool {
	return m.conv.Stream.Active || m.hasPendingToolExecution()
}

// enqueueCurrentInput captures the current input field content into the queue.
func (m *model) enqueueCurrentInput(input string) {
	var images []message.ImageData
	for _, p := range m.input.Images.Pending {
		images = append(images, p.Data)
	}
	m.inputQueue.Enqueue(input, images)
	m.resetInputField()
}

// drainInputQueue dequeues the next pending input and starts a new turn.
// Returns nil if the queue is empty.
func (m *model) drainInputQueue() tea.Cmd {
	item, ok := m.inputQueue.Dequeue()
	if !ok {
		return nil
	}

	m.maxOutputRecoveryCount = 0
	m.conv.Compact.ClearResult()

	m.input.Textarea.SetValue(item.Content)
	m.input.Textarea.CursorEnd()
	m.input.UpdateHeight()
	m.input.Images.Pending = nil
	m.input.Images.Selection = appinput.ImageSelection{}

	req := submitRequest{Input: item.Content}
	// Restore images into the input model so prepareSubmittedUserMessage can process them
	maxID := 0
	for i, img := range item.Images {
		id := i + 1
		m.input.Images.Pending = append(m.input.Images.Pending, appinput.PendingImage{
			ID:   id,
			Data: img,
		})
		maxID = id
	}
	m.input.Images.NextID = maxID

	return m.executeSubmitRequest(req)
}

func (m *model) executeSubmitRequest(req submitRequest) tea.Cmd {
	if isExitRequest(req.Input) {
		cmd, _ := m.quitWithCancel()
		return cmd
	}

	if blocked, reason := m.checkPromptHook(req.Input); blocked {
		return m.blockPromptSubmission(reason)
	}

	// If the user submits a new turn while tools are still running, cancel the
	// unfinished tool calls first and append synthetic tool_result messages so
	// the next provider request does not contain orphaned tool_use blocks.
	if m.hasPendingToolExecution() {
		m.cancelPendingToolCalls()
	}

	m.recordSubmittedInput(req.Input)

	if cmd, handled := m.handleCommandSubmit(req.Input); handled {
		return cmd
	}

	m.skill.ActiveInvocation = ""
	plugin.ClearActivePluginRoot()

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
	preserve := shouldPreserveCommandInConversation(input, "", nil)
	preAppended := false
	if preserve && shouldPreserveBeforeCommandExecution(input) {
		m.conv.Append(message.ChatMessage{Role: message.RoleUser, Content: input})
		preAppended = true
	}

	insertAt := len(m.conv.Messages)
	result, cmd, isCmd := ExecuteCommand(context.Background(), m, input)
	if !isCmd {
		if preAppended && len(m.conv.Messages) > 0 {
			m.conv.Messages = m.conv.Messages[:len(m.conv.Messages)-1]
		}
		return nil, false
	}

	m.resetInputField()

	// Slash commands should remain visible in the conversation so the transcript
	// reflects the user's literal input and arguments. Skill commands are the
	// exception here because handleSkillInvocation appends the full slash
	// invocation itself before starting the provider turn.
	if preserve && !preAppended {
		m.insertConversationMessage(insertAt, message.ChatMessage{Role: message.RoleUser, Content: input})
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

func shouldPreserveBeforeCommandExecution(input string) bool {
	name, _, isCmd := appcommand.ParseCommand(input)
	if !isCmd {
		return false
	}
	return name == "loop"
}

func (m *model) insertConversationMessage(idx int, msg message.ChatMessage) {
	if idx < 0 || idx >= len(m.conv.Messages) {
		m.conv.Append(msg)
		return
	}

	m.conv.Messages = append(m.conv.Messages, message.ChatMessage{})
	copy(m.conv.Messages[idx+1:], m.conv.Messages[idx:])
	m.conv.Messages[idx] = msg
	if idx < m.conv.CommittedCount {
		m.conv.CommittedCount++
	}
}

func shouldPreserveCommandInConversation(input, result string, cmd tea.Cmd) bool {
	name, _, isCmd := appcommand.ParseCommand(input)
	if !isCmd {
		return false
	}
	switch name {
	case "clear", "exit":
		return false
	}

	if skill.DefaultRegistry != nil {
		if sk, ok := skill.DefaultRegistry.Get(name); ok && sk.IsEnabled() {
			return false
		}
	}

	if _, ok := appcommand.IsCustomCommand(name); ok {
		return false
	}

	return true
}

func (m *model) prepareSubmittedUserMessage(input string) (message.ChatMessage, tea.Cmd, bool) {
	content, fileImages, err := appinput.ProcessImageRefs(m.cwd, input)
	if err != nil {
		m.conv.Append(message.ChatMessage{Role: message.RoleNotice, Content: "Image error: " + err.Error()})
		return message.ChatMessage{}, tea.Batch(m.commitMessages()...), true
	}

	displayContent := content
	content, inlineImages := m.input.ExtractInlineImages(content)
	allImages := append(inlineImages, fileImages...)

	return message.ChatMessage{
		Role:           message.RoleUser,
		Content:        content,
		DisplayContent: displayContent,
		Images:         allImages,
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
