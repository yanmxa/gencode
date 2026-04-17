// Runtime interface implementations: bridges between the app model and
// sub-package handlers (user overlays, agent notifications, system events).
package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/app/kit"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/llm"
)

// --- User overlay dispatcher (Source 1) ---

func (m *model) updateUserOverlays(msg tea.Msg) (tea.Cmd, bool) {
	return appuser.Update(m, &m.userInput, msg)
}

// --- User overlay Runtime interface implementations ---

// user.PluginRuntime
func (m *model) GetCwd() string                { return m.cwd }
func (m *model) ReloadPluginBackedState() error { return m.reloadPluginBackedState() }

// memory.Runtime
func (m *model) ClearCachedInstructions()            { m.runtime.ClearCachedInstructions() }
func (m *model) RefreshMemoryContext(trigger string) { m.runtime.RefreshMemoryContext(m.cwd, trigger) }
func (m *model) FireFileChanged(path, tool string)   { m.fireFileChanged(path, tool) }

// user.MCPRuntime
func (m *model) SetInputText(text string) { m.userInput.Textarea.SetValue(text) }

// appuser.ProviderRuntime
func (m *model) SwitchProvider(p llm.Provider) {
	m.runtime.SwitchProvider(p)
	m.reconfigureAgentTool()
}
func (m *model) SetCurrentModel(cm *llm.CurrentModelInfo) { m.runtime.CurrentModel = cm }

// user.SessionRuntime
func (m *model) LoadSession(id string) error { return m.loadSession(id) }
func (m *model) ResetCommitIndex()           { m.conv.CommittedCount = 0 }
func (m *model) CommitAllMessages() []tea.Cmd { return m.commitAllMessages() }

// searchui.Runtime
func (m *model) SetProviderStatusMessage(msg string) { m.userInput.Provider.SetStatusMessage(msg) }

func startExternalEditor(filePath string) tea.Cmd {
	return kit.StartExternalEditor(filePath, func(err error) tea.Msg {
		return appuser.MemoryEditorFinishedMsg{Err: err}
	})
}

// --- User input helpers ---

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
	m.userInput.Images.Selection = appuser.ImageSelection{}
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

// --- Agent input bridge (Source 2) ---

type agentRuntime struct {
	m *model
}

func (m *model) updateAgentInput(msg tea.Msg) (tea.Cmd, bool) {
	return appagent.Update(agentRuntime{m: m}, &m.agentInput, msg)
}

func (m *model) handleTaskNotificationTick() tea.Cmd {
	cmd, _ := appagent.Update(agentRuntime{m: m}, &m.agentInput, appagent.TickMsg{})
	return cmd
}

func (rt agentRuntime) IsInputIdle() bool    { return rt.m.isInputIdle() }
func (rt agentRuntime) StreamActive() bool   { return rt.m.conv.Stream.Active }

func (rt agentRuntime) InjectTaskNotificationContinuation(item appagent.Notification) tea.Cmd {
	return rt.m.injectTaskNotificationContinuation(item)
}

func (m *model) injectTaskNotificationContinuation(item appagent.Notification) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if m.runtime.LLMProvider == nil {
		if item.Notice == "" {
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleNotice,
				Content: "A background task completed, but no provider is connected.",
			})
		}
		return tea.Batch(m.commitMessages()...)
	}
	if item.ContinuationPrompt == "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "A background task completed, but no task notification payload was available.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	for _, ctx := range appagent.ContinuationContext(item) {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(appagent.BuildContinuationPrompt(item), nil)
}

// --- System input bridge (Source 3) ---

type systemRuntime struct {
	m *model
}

func (m *model) updateSystemInput(msg tea.Msg) (tea.Cmd, bool) {
	return appsystem.Update(systemRuntime{m: m}, &m.systemInput, msg)
}

func (m *model) isInputIdle() bool {
	return !m.conv.Stream.Active && !m.isToolPhaseActive()
}

func (m *model) handleAsyncHookTick() tea.Cmd {
	cmd, _ := appsystem.Update(systemRuntime{m: m}, &m.systemInput, appsystem.AsyncHookTickMsg{})
	return cmd
}

func (rt systemRuntime) IsInputIdle() bool { return rt.m.isInputIdle() }

func (rt systemRuntime) InjectAsyncHookContinuation(item appsystem.AsyncHookRewake) tea.Cmd {
	return rt.m.injectAsyncHookContinuation(item)
}

func (rt systemRuntime) InjectCronPrompt(prompt string) tea.Cmd {
	return rt.m.injectCronPrompt(prompt)
}

func (rt systemRuntime) AppendNotice(text string) {
	if text == "" {
		return
	}
	rt.m.conv.Append(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: text,
	})
}

func (m *model) injectAsyncHookContinuation(item appsystem.AsyncHookRewake) tea.Cmd {
	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if len(item.Context) == 0 {
		return tea.Batch(m.commitMessages()...)
	}
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "Async hook requested a follow-up, but no provider is connected.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	for _, ctx := range item.Context {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: ctx,
		})
	}
	return m.sendToAgent(item.ContinuationPrompt, nil)
}

func (m *model) injectCronPrompt(prompt string) tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: fmt.Sprintf("Cron fired but no provider connected: %s", prompt),
		})
		return tea.Batch(m.commitMessages()...)
	}

	m.conv.Append(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: "Scheduled task fired",
	})
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: prompt,
	})

	return m.sendToAgent(prompt, nil)
}
