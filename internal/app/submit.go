package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/core"
	appcommand "github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/plugin"
)

type submitRequest struct {
	Input string
}

func (m *model) resetInputField() {
	m.userInput.Reset()
}

func (m *model) handleSubmit() tea.Cmd {
	m.promptSuggestion.Clear()

	input := strings.TrimSpace(m.userInput.FullValue())
	if input == "" && len(m.userInput.Images.Pending) == 0 {
		return nil
	}

	if m.isTurnActive() {
		m.enqueueCurrentInput(input)
		return nil
	}

	m.conv.Compact.ClearResult()

	return m.executeSubmitRequest(submitRequest{Input: input})
}

// isTurnActive returns true when the LLM is streaming or tools are executing.
func (m *model) isTurnActive() bool {
	return m.conv.Stream.Active
}

// enqueueCurrentInput captures the current input field content into the queue.
func (m *model) enqueueCurrentInput(input string) {
	if m.userInput.Queue.Enqueue(input, m.userInput.PendingImages()) < 0 {
		m.conv.AddNotice("Input queue is full. Please wait for the current turn to complete.")
		return
	}
	m.resetInputField()
}

// drainInputQueue dequeues the next pending input and starts a new turn.
// Returns nil if the queue is empty.
func (m *model) drainInputQueue() tea.Cmd {
	item, ok := m.userInput.Queue.Dequeue()
	if !ok {
		return nil
	}

	m.conv.Compact.ClearResult()
	m.userInput.RestoreImages(item.Images)

	req := submitRequest{Input: item.Content}

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

	m.userInput.RecordSubmission(m.cwd, req.Input)

	if cmd, handled := m.commands().handleSubmit(req.Input); handled {
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

func (m *model) blockPromptSubmission(reason string) tea.Cmd {
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: "Prompt blocked: " + reason,
	})
	m.resetInputField()
	return tea.Batch(m.commitMessages()...)
}


func (m *model) prepareSubmittedUserMessage(input string) (core.ChatMessage, tea.Cmd, bool) {
	content, fileImages, err := appuser.ProcessImageRefs(m.cwd, input)
	if err != nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Image error: " + err.Error()})
		return core.ChatMessage{}, tea.Batch(m.commitMessages()...), true
	}

	displayContent := content
	content, inlineImages := m.userInput.ExtractInlineImages(content)
	allImages := make([]core.Image, 0, len(inlineImages)+len(fileImages))
	allImages = append(allImages, inlineImages...)
	allImages = append(allImages, fileImages...)

	return core.ChatMessage{
		Role:           core.RoleUser,
		Content:        content,
		DisplayContent: displayContent,
		Images:         allImages,
	}, nil, false
}

// startProviderTurn starts an LLM turn by sending the user message to the agent.
func (m *model) startProviderTurn(content string) tea.Cmd {
	if m.llmProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "No provider connected. Use /provider to connect.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	// Ensure agent session exists (lazy initialization)
	if err := m.ensureAgentSession(); err != nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "Failed to start agent: " + err.Error(),
		})
		return tea.Batch(m.commitMessages()...)
	}

	// Detect thinking keywords for per-turn override
	m.detectThinkingKeywords(content)

	// Get images from the last appended user message
	var images []core.Image
	if len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		images = lastMsg.Images
	}

	return m.sendToAgent(content, images)
}

func isExitRequest(input string) bool {
	return strings.EqualFold(input, "exit")
}

func shouldPreserveCommandInConversation(input string) bool {
	name, _, isCmd := appcommand.ParseCommand(input)
	if !isCmd {
		return false
	}
	switch name {
	case "clear", "exit":
		return false
	}
	if _, ok := lookupSkillCommand(name); ok {
		return false
	}
	if _, ok := appcommand.IsCustomCommand(name); ok {
		return false
	}
	return true
}

func shouldPreserveBeforeCommandExecution(input string) bool {
	name, _, isCmd := appcommand.ParseCommand(input)
	if !isCmd {
		return false
	}
	return name == "loop"
}
