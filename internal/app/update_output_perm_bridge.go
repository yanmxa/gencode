// Permission bridge: routes agent-side permission requests through the TUI
// approval modal so the user can approve/deny tool calls made by core.Agent.
package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appoutput "github.com/yanmxa/gencode/internal/app/output"
	appapproval "github.com/yanmxa/gencode/internal/app/user/approval"
)

// --- Permission bridge types ---

type agentPermissionMsg struct {
	Request *appoutput.PermBridgeRequest
}

func pollPermBridge(pb *appoutput.PermissionBridge) tea.Cmd {
	return func() tea.Msg {
		req, ok := pb.Recv()
		if !ok {
			return nil
		}
		return agentPermissionMsg{Request: req}
	}
}

func (m *model) handlePermBridgeResponse(msg appapproval.ResponseMsg) tea.Cmd {
	if m.agentSess == nil {
		return nil
	}
	req := m.agentSess.pendingPermRequest
	m.agentSess.pendingPermRequest = nil

	if req == nil {
		return nil
	}

	resp := appoutput.PermBridgeResponse{
		Allow:  msg.Approved,
		Reason: "user decision",
	}

	if msg.Approved {
		if msg.AllowAll && m.sessionPermissions != nil && msg.Request != nil {
			m.sessionPermissions.AllowTool(msg.Request.ToolName)
		}
		resp.Reason = "user approved"
	} else {
		resp.Reason = "user denied"
	}

	select {
	case req.Response <- resp:
	default:
	}

	return pollPermBridge(m.agentSess.permBridge)
}

// showPermissionPrompt activates the approval modal for an agent-side permission request.
func (m *model) showPermissionPrompt(req *appoutput.PermBridgeRequest) tea.Cmd {
	if req == nil || req.Request == nil {
		return nil
	}
	m.approval.Show(req.Request, m.width, m.height)
	return nil
}
