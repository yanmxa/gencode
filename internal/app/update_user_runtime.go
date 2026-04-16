package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appuser "github.com/yanmxa/gencode/internal/app/user"
	"github.com/yanmxa/gencode/internal/util/image"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/provider"
)

func (m *model) handleStreamCancel() tea.Cmd {
	// Stop core.Agent if active — cancel its context to interrupt the loop
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	// Reset per-turn thinking override so it doesn't leak into subsequent turns
	m.thinkingOverride = provider.ThinkingOff
	m.cancelPendingToolCalls()
	m.conv.MarkLastInterrupted()

	cmds := m.commitMessages()

	// Drain queued user inputs after cancellation
	if cmd := m.drainInputQueue(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return tea.Batch(cmds...)
}

// cancelPendingToolCalls adds cancellation messages for pending tool calls.
func (m *model) cancelPendingToolCalls() {
	var toolCalls []core.ToolCall

	if m.toolExec.Cancel != nil {
		m.toolExec.Cancel()
	}

	if m.toolExec.PendingCalls != nil && m.toolExec.CurrentIdx < len(m.toolExec.PendingCalls) {
		toolCalls = m.toolExec.PendingCalls[m.toolExec.CurrentIdx:]
		m.toolExec.Reset()
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

// cancelRemainingToolCalls adds cancellation tool_result messages for pending
// tool calls starting at startIdx. This ensures every tool_use block in the
// assistant message has a corresponding tool_result so the API doesn't reject
// the request with "tool_use ids were found without tool_result blocks".
func (m *model) cancelRemainingToolCalls(startIdx int) {
	if m.toolExec.PendingCalls == nil || startIdx >= len(m.toolExec.PendingCalls) {
		return
	}
	for _, tc := range m.toolExec.PendingCalls[startIdx:] {
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
		return "Stopped waiting for background task output because the user sent a new core. The background task may still be running."
	default:
		return "Tool execution interrupted because the user sent a new core."
	}
}

// detectThinkingKeywords scans the user's message for explicit thinking-level keywords
// and sets a per-turn override (not persistent). The override resets after the turn completes.
func (m *model) detectThinkingKeywords(input string) {
	lower := strings.ToLower(input)

	if strings.Contains(lower, "ultrathink") ||
		strings.Contains(lower, "think really hard") ||
		strings.Contains(lower, "think super hard") ||
		strings.Contains(lower, "maximum thinking") {
		m.thinkingOverride = provider.ThinkingUltra
		return
	}

	if strings.Contains(lower, "think harder") ||
		strings.Contains(lower, "think hard") ||
		strings.Contains(lower, "think deeply") ||
		strings.Contains(lower, "think carefully") {
		m.thinkingOverride = provider.ThinkingHigh
		return
	}
}

// handleSkillInvocation handles skill command invocation by sending the skill
// instructions and args to the LLM.
func (m *model) handleSkillInvocation() tea.Cmd {
	if m.llmProvider == nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "No provider connected. Use /provider to connect."})
		m.skill.PendingInstructions = ""
		m.skill.PendingArgs = ""
		return tea.Batch(m.commitMessages()...)
	}

	userMsg := m.skill.PendingArgs
	if userMsg == "" {
		userMsg = "Execute the skill."
	}
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})

	if m.skill.PendingInstructions != "" {
		m.skill.ActiveInvocation = m.skill.PendingInstructions
		m.skill.PendingInstructions = ""
	}
	m.skill.PendingArgs = ""

	return m.sendToAgent(userMsg, nil)
}

// pasteImageFromClipboard handles pasting image from clipboard.
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

// quitWithCancel cancels any active stream and tool execution before quitting.
// Use this as the single exit point for all quit paths (Ctrl+C, Ctrl+D, "exit").
func (m *model) quitWithCancel() (tea.Cmd, bool) {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	if m.toolExec.Cancel != nil {
		m.toolExec.Cancel()
	}
	m.fireSessionEnd("prompt_input_exit")
	return tea.Quit, true
}
