package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/ui/progress"
)

// updateApproval routes permission request messages.
// Note: response messages are handled directly in delegateToActiveModal.
func (m *model) updateApproval(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appapproval.RequestMsg:
		c := m.handlePermissionRequest(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handlePermissionRequest(msg appapproval.RequestMsg) tea.Cmd {
	if blocked, reason := m.checkPermissionHook(msg.Request); blocked {
		return m.abortToolWithError("Blocked by hook: " + reason)
	}

	m.approval.Show(msg.Request, m.width, m.height)
	return nil
}

func (m *model) abortToolWithError(errorMsg string) tea.Cmd {
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]
	m.conv.Append(message.ChatMessage{
		Role:     message.RoleUser,
		ToolName: tc.Name,
		ToolResult: &message.ToolResult{
			ToolCallID: tc.ID,
			Content:    errorMsg,
			IsError:    true,
		},
	})
	m.tool.PendingCalls = nil
	m.tool.CurrentIdx = 0
	m.conv.Stream.Active = false
	return tea.Batch(m.commitMessages()...)
}

func (m *model) checkPermissionHook(req *permission.PermissionRequest) (bool, string) {
	if m.hookEngine == nil || req == nil {
		return false, ""
	}

	toolInput := make(map[string]any)
	if req.FilePath != "" {
		toolInput["file_path"] = req.FilePath
	}
	if req.BashMeta != nil {
		toolInput["command"] = req.BashMeta.Command
	}

	outcome := m.hookEngine.Execute(context.Background(), hooks.PermissionRequest, hooks.HookInput{
		ToolName:  req.ToolName,
		ToolInput: toolInput,
	})
	return outcome.ShouldBlock, outcome.BlockReason
}

func (m *model) handlePermissionResponse(msg appapproval.ResponseMsg) tea.Cmd {
	if !msg.Approved {
		return m.abortToolWithError("User denied permission")
	}

	if msg.AllowAll && m.mode.SessionPermissions != nil && msg.Request != nil {
		m.applyAllowAllPermission(msg.Request.ToolName)
	}

	if msg.Request != nil && msg.Request.ToolName == "Agent" {
		m.output.TaskProgress = nil
		return tea.Batch(
			apptool.ExecuteApproved(m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd),
			progress.Check(),
		)
	}

	return apptool.ExecuteApproved(m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd)
}

func (m *model) applyAllowAllPermission(toolName string) {
	switch toolName {
	case "Edit":
		m.mode.SessionPermissions.AllowAllEdits = true
	case "Write":
		m.mode.SessionPermissions.AllowAllWrites = true
	case "Bash":
		m.mode.SessionPermissions.AllowAllBash = true
	case "Skill":
		m.mode.SessionPermissions.AllowAllSkills = true
	case "Agent":
		m.mode.SessionPermissions.AllowAllTasks = true
	default:
		m.mode.SessionPermissions.AllowTool(toolName)
	}
}

// togglePermissionPreview toggles the expand state of permission prompt previews.
func (m *model) togglePermissionPreview() {
	m.approval.TogglePreview()
}
