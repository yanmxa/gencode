package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/llm"
)

func (m *model) GetCwd() string                 { return m.cwd }
func (m *model) ReloadPluginBackedState() error { return m.reloadPluginBackedState() }
func (m *model) ClearCachedInstructions()       { m.runtime.ClearCachedInstructions() }
func (m *model) RefreshMemoryContext(trigger string) {
	m.runtime.RefreshMemoryContext(m.cwd, trigger)
}
func (m *model) FireFileChanged(path, tool string) { m.fireFileChanged(path, tool) }
func (m *model) SetInputText(text string)          { m.userInput.Textarea.SetValue(text) }
func (m *model) SwitchProvider(p llm.Provider) {
	m.runtime.SwitchProvider(p)
	m.reconfigureAgentTool()
}
func (m *model) SetCurrentModel(cm *llm.CurrentModelInfo) { m.runtime.CurrentModel = cm }
func (m *model) LoadSession(id string) error              { return m.loadSession(id) }
func (m *model) ResetCommitIndex()                        { m.conv.CommittedCount = 0 }
func (m *model) CommitAllMessages() []tea.Cmd             { return m.commitAllMessages() }
func (m *model) SetProviderStatusMessage(msg string)      { m.userInput.Provider.SetStatusMessage(msg) }

func startExternalEditor(filePath string) tea.Cmd {
	return kit.StartExternalEditor(filePath, func(err error) tea.Msg {
		return input.MemoryEditorFinishedMsg{Err: err}
	})
}

func (m *model) handleStreamCancel() tea.Cmd {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	m.runtime.ClearThinkingOverride()
	m.cancelPendingToolCalls()
	m.conv.MarkLastInterrupted()

	cmds := m.commitMessages()
	if cmd := m.drainInputQueue(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m *model) cancelPendingToolCalls() {
	toolCalls := m.tool.DrainPendingCalls()
	if toolCalls == nil && len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		if lastMsg.Role == core.RoleAssistant {
			toolCalls = lastMsg.ToolCalls
		}
	}
	m.conv.AppendCancelledToolResults(toolCalls, pendingToolCancellationContent)
}

func (m *model) cancelRemainingToolCalls(startIdx int) {
	m.conv.AppendCancelledToolResults(m.tool.RemainingCalls(startIdx), func(core.ToolCall) string {
		return "Tool execution skipped."
	})
}

func pendingToolCancellationContent(tc core.ToolCall) string {
	switch tc.Name {
	case "TaskOutput":
		return "Stopped waiting for background task output because the user sent a new message. The background task may still be running."
	default:
		return "Tool execution interrupted because the user sent a new message."
	}
}

func (m *model) handleSkillInvocation() tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.AddNotice("No provider connected. Use /provider to connect.")
		m.userInput.Skill.ClearPending()
		return tea.Batch(m.commitMessages()...)
	}
	userMsg := m.userInput.Skill.ConsumeInvocation()
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})
	return m.sendToAgent(userMsg, nil)
}

func (m *model) pasteImageFromClipboard() (tea.Cmd, bool) {
	imgData, err := image.ReadImageToProviderData()
	if err != nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Image paste error: " + err.Error()})
		return tea.Batch(m.commitMessages()...), true
	}
	if imgData == nil {
		return nil, false
	}
	label := m.userInput.AddPendingImage(*imgData)
	m.userInput.Images.Selection = input.ImageSelection{}
	m.userInput.Textarea.InsertString(label)
	m.userInput.UpdateHeight()
	return nil, true
}

func (m *model) quitWithCancel() (tea.Cmd, bool) {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	if m.tool.Cancel != nil {
		m.tool.Cancel()
	}
	m.fireSessionEnd("prompt_input_exit")
	return tea.Quit, true
}
