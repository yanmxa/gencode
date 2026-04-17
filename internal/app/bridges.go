// Runtime interface implementations: bridges between the app model and
// sub-package handlers (user overlays, agent notifications, system events).
package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/app/kit"
	appsystem "github.com/yanmxa/gencode/internal/app/system"
	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/app/user/mcpui"
	"github.com/yanmxa/gencode/internal/app/user/pluginui"
	"github.com/yanmxa/gencode/internal/app/user/providerui"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/llm"
)

// --- User overlay dispatchers (Source 1) ---

func (m *model) updateMCP(msg tea.Msg) (tea.Cmd, bool) {
	return mcpui.Update(m, &m.mcp, msg)
}
func (m *model) updateMemory(msg tea.Msg) (tea.Cmd, bool) {
	return appuser.UpdateMemory(m, &m.userInput.Memory, msg)
}
func (m *model) updatePlugin(msg tea.Msg) (tea.Cmd, bool) {
	return pluginui.Update(m, &m.plugin, msg)
}
func (m *model) updateProvider(msg tea.Msg) (tea.Cmd, bool) {
	return providerui.Update(m, &m.provider, msg)
}
func (m *model) updateSearch(msg tea.Msg) (tea.Cmd, bool) {
	return appuser.UpdateSearch(m, &m.userInput.Search, msg)
}
func (m *model) updateSession(msg tea.Msg) (tea.Cmd, bool) {
	return appuser.UpdateSession(m, &m.userInput.Session, msg)
}

// --- User overlay Runtime interface implementations ---

// pluginui.Runtime
func (m *model) GetCwd() string                { return m.cwd }
func (m *model) ReloadPluginBackedState() error { return m.reloadPluginBackedState() }

// memory.Runtime
func (m *model) ClearCachedInstructions() {
	m.cachedUserInstructions = ""
	m.cachedProjectInstructions = ""
}
func (m *model) RefreshMemoryContext(trigger string) { m.refreshMemoryContext(trigger) }
func (m *model) FireFileChanged(path, tool string)   { m.fireFileChanged(path, tool) }

// mcpui.Runtime
func (m *model) SetInputText(text string) { m.userInput.Textarea.SetValue(text) }

// providerui.Runtime
func (m *model) SwitchProvider(p llm.Provider) {
	m.llmProvider = p
	if m.hookEngine != nil {
		m.hookEngine.SetLLMProvider(m.llmProvider, m.getModelID())
	}
	m.reconfigureAgentTool()
}
func (m *model) SetCurrentModel(cm *llm.CurrentModelInfo) { m.currentModel = cm }

// user.SessionRuntime
func (m *model) EnsureSessionStore() error { return m.ensureSessionStore() }
func (m *model) ForkSession(id string) (string, error) {
	forked, err := m.sessionStore.Fork(id)
	if err != nil {
		return "", err
	}
	return forked.Metadata.ID, nil
}
func (m *model) LoadSession(id string) error { return m.loadSession(id) }
func (m *model) ResetCommitIndex()           { m.conv.CommittedCount = 0 }
func (m *model) CommitAllMessages() []tea.Cmd { return m.commitAllMessages() }

// searchui.Runtime
func (m *model) SetProviderStatusMessage(msg string) { m.provider.SetStatusMessage(msg) }

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
	m.thinkingOverride = llm.ThinkingOff
	m.cancelPendingToolCalls()
	m.conv.MarkLastInterrupted()

	cmds := m.commitMessages()
	if cmd := m.drainInputQueue(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m *model) cancelPendingToolCalls() {
	var toolCalls []core.ToolCall

	if m.tool.Cancel != nil {
		m.tool.Cancel()
	}

	if m.tool.PendingCalls != nil && m.tool.CurrentIdx < len(m.tool.PendingCalls) {
		toolCalls = m.tool.PendingCalls[m.tool.CurrentIdx:]
		m.tool.Reset()
	} else if len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		if lastMsg.Role == core.RoleAssistant {
			toolCalls = lastMsg.ToolCalls
		}
	}

	for _, tc := range toolCalls {
		m.conv.Append(core.ChatMessage{
			Role:     core.RoleUser,
			ToolName: tc.Name,
			ToolResult: &core.ToolResult{
				ToolCallID: tc.ID,
				Content:    pendingToolCancellationContent(tc),
				IsError:    true,
			},
		})
	}
}

func (m *model) cancelRemainingToolCalls(startIdx int) {
	if m.tool.PendingCalls == nil || startIdx >= len(m.tool.PendingCalls) {
		return
	}
	for _, tc := range m.tool.PendingCalls[startIdx:] {
		m.conv.Append(core.ChatMessage{
			Role:     core.RoleUser,
			ToolName: tc.Name,
			ToolResult: &core.ToolResult{
				ToolCallID: tc.ID,
				Content:    "Tool execution skipped.",
				IsError:    true,
			},
		})
	}
}

func pendingToolCancellationContent(tc core.ToolCall) string {
	switch tc.Name {
	case "TaskOutput":
		return "Stopped waiting for background task output because the user sent a new message. The background task may still be running."
	default:
		return "Tool execution interrupted because the user sent a new message."
	}
}

func (m *model) detectThinkingKeywords(input string) {
	lower := strings.ToLower(input)

	if strings.Contains(lower, "ultrathink") ||
		strings.Contains(lower, "think really hard") ||
		strings.Contains(lower, "think super hard") ||
		strings.Contains(lower, "maximum thinking") {
		m.thinkingOverride = llm.ThinkingUltra
		return
	}

	if strings.Contains(lower, "think harder") ||
		strings.Contains(lower, "think hard") ||
		strings.Contains(lower, "think deeply") ||
		strings.Contains(lower, "think carefully") {
		m.thinkingOverride = llm.ThinkingHigh
		return
	}
}

func (m *model) handleSkillInvocation() tea.Cmd {
	if m.llmProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "No provider connected. Use /provider to connect."})
		m.userInput.Skill.PendingInstructions = ""
		m.userInput.Skill.PendingArgs = ""
		return tea.Batch(m.commitMessages()...)
	}

	userMsg := m.userInput.Skill.PendingArgs
	if userMsg == "" {
		userMsg = "Execute the skill."
	}
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})

	if m.userInput.Skill.PendingInstructions != "" {
		m.userInput.Skill.ActiveInvocation = m.userInput.Skill.PendingInstructions
		m.userInput.Skill.PendingInstructions = ""
	}
	m.userInput.Skill.PendingArgs = ""

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
	if m.llmProvider == nil {
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
	return appsystem.Update(systemRuntime{m: m}, &m.systemInput, m.hookEngine, msg)
}

func (m *model) isInputIdle() bool {
	return !m.conv.Stream.Active && !m.isToolPhaseActive()
}

func (m *model) handleAsyncHookTick() tea.Cmd {
	cmd, _ := appsystem.Update(systemRuntime{m: m}, &m.systemInput, m.hookEngine, appsystem.AsyncHookTickMsg{})
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
	if m.llmProvider == nil {
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
	if m.llmProvider == nil {
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
