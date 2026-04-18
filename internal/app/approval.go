package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/core"
)

func (m *model) approvalDeps() input.ApprovalFlowDeps {
	return input.ApprovalFlowDeps{
		Input:                &m.userInput,
		Runtime:              &m.runtime,
		Tool:                 &m.tool,
		Width:                m.width,
		Height:               m.height,
		Cwd:                  m.cwd,
		ProgressHub:          m.agentOutput.ProgressHub,
		ContinueOutbox:       m.continueOutbox,
		AbortToolWithError:   m.abortToolWithError,
		ReloadProjectContext: m.reloadProjectContext,
	}
}

func (m *model) handlePermissionRequest(msg input.ApprovalRequestMsg) tea.Cmd {
	return input.HandlePermissionRequest(m.approvalDeps(), msg)
}

func (m *model) handleHookPermissionResult(msg input.HookPermissionResultMsg) tea.Cmd {
	return input.HandleHookPermissionResult(m.approvalDeps(), msg)
}

func (m *model) handlePermissionResponse(msg input.ApprovalResponseMsg) tea.Cmd {
	return m.handlePermBridgeDecision(permissionDecision{Approved: msg.Approved, AllowAll: msg.AllowAll, Request: msg.Request})
}

func (m *model) togglePermissionPreview() {
	input.TogglePermissionPreview(&m.userInput)
}

func (m *model) applyUpdatedToolInput(updated map[string]any) {
	input.ApplyUpdatedToolInput(&m.tool, updated)
}

func (m *model) abortToolWithError(errorMsg string, retry bool) tea.Cmd {
	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		m.tool.Reset()
		m.conv.Stream.Stop()
		return tea.Batch(m.commitMessages()...)
	}
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, ToolName: tc.Name, ToolResult: &core.ToolResult{ToolCallID: tc.ID, Content: errorMsg, IsError: true}})
	m.cancelRemainingToolCalls(m.tool.CurrentIdx + 1)
	m.tool.Reset()
	m.conv.Stream.Stop()
	commitCmds := m.commitMessages()
	if retry {
		commitCmds = append(commitCmds, m.continueOutbox())
	}
	return tea.Batch(commitCmds...)
}
