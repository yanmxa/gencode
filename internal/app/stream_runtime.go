package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/message"
)

func (m *model) buildStreamRequest(extra []string) streamRequest {
	m.ensureMemoryContextLoaded()
	c := m.buildLoopClient()
	return streamRequest{
		Client:   c,
		Messages: m.conv.ConvertToProvider(),
		Tools:    m.buildLoopToolSet().Tools(),
		System:   m.buildLoopSystem(extra, c).Prompt(),
	}
}

func (m *model) buildInternalContinuationRequest(extra []string, prompt string) streamRequest {
	req := m.buildStreamRequest(extra)
	if prompt != "" {
		req.Messages = append(req.Messages, message.UserMessage(prompt, nil))
	}
	return req
}

func (m *model) startConversationStream(req streamRequest) tea.Cmd {
	started := m.asyncOps.StartStream(req)
	m.conv.Stream.Cancel = started.Cancel
	m.conv.Stream.Active = true
	m.conv.Stream.Ch = started.Ch

	commitCmds := m.commitMessages()
	m.conv.Append(message.ChatMessage{Role: message.RoleAssistant, Content: ""})

	allCmds := append(commitCmds, m.waitForChunk(), m.output.Spinner.Tick)
	return tea.Batch(allCmds...)
}
