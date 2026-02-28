package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/tui/progress"
)

func (m *model) handlePermissionRequest(msg PermissionRequestMsg) (tea.Model, tea.Cmd) {
	if blocked, reason := m.checkPermissionHook(msg.Request); blocked {
		return m.abortToolWithError("Blocked by hook: " + reason)
	}

	m.permissionPrompt.Show(msg.Request, m.width, m.height)
	return m, nil
}

func (m *model) abortToolWithError(errorMsg string) (tea.Model, tea.Cmd) {
	tc := m.toolExec.pendingCalls[m.toolExec.currentIdx]
	m.messages = append(m.messages, chatMessage{
		role:     roleUser,
		toolName: tc.Name,
		toolResult: &message.ToolResult{
			ToolCallID: tc.ID,
			Content:    errorMsg,
			IsError:    true,
		},
	})
	m.toolExec.pendingCalls = nil
	m.toolExec.currentIdx = 0
	m.streaming = false
	return m, tea.Batch(m.commitMessages()...)
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

func (m *model) handlePermissionResponse(msg PermissionResponseMsg) (tea.Model, tea.Cmd) {
	if !msg.Approved {
		return m.abortToolWithError("User denied permission")
	}

	if msg.AllowAll && m.sessionPermissions != nil && msg.Request != nil {
		m.applyAllowAllPermission(msg.Request.ToolName)
	}

	if msg.Request != nil && msg.Request.ToolName == "Task" {
		m.taskProgress = nil
		return m, tea.Batch(
			ExecuteApproved(m.toolExec.pendingCalls, m.toolExec.currentIdx, m.cwd),
			progress.Check(),
		)
	}

	return m, ExecuteApproved(m.toolExec.pendingCalls, m.toolExec.currentIdx, m.cwd)
}

func (m *model) applyAllowAllPermission(toolName string) {
	switch toolName {
	case "Edit":
		m.sessionPermissions.AllowAllEdits = true
	case "Write":
		m.sessionPermissions.AllowAllWrites = true
	case "Bash":
		m.sessionPermissions.AllowAllBash = true
	case "Skill":
		m.sessionPermissions.AllowAllSkills = true
	case "Task":
		m.sessionPermissions.AllowAllTasks = true
	default:
		m.sessionPermissions.AllowTool(toolName)
	}
}
