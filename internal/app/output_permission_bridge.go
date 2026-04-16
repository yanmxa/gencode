package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

func (m *model) StorePendingPermRequest(req *appoutput.PermBridgeRequest) {
	if m.agentSess != nil {
		m.agentSess.pendingPermRequest = req
	}
}

func (m *model) ShowPermissionPrompt(req *appoutput.PermBridgeRequest) tea.Cmd {
	if req == nil || req.Request == nil {
		return nil
	}
	m.approval.Show(req.Request, m.width, m.height)
	return nil
}

type permissionDecision struct {
	Approved bool
	AllowAll bool
	Request  *perm.PermissionRequest
}

func (m *model) handlePermBridgeDecision(decision permissionDecision) tea.Cmd {
	if m.agentSess == nil {
		return nil
	}
	req := m.agentSess.pendingPermRequest
	m.agentSess.pendingPermRequest = nil

	if req == nil {
		return nil
	}

	resp := appoutput.PermBridgeResponse{
		Allow:  decision.Approved,
		Reason: "user decision",
	}

	if decision.Approved {
		if decision.AllowAll && m.sessionPermissions != nil && decision.Request != nil {
			m.sessionPermissions.AllowTool(decision.Request.ToolName)
		}
		resp.Reason = "user approved"
	} else {
		resp.Reason = "user denied"
	}

	select {
	case req.Response <- resp:
	default:
	}

	return appoutput.PollPermBridge(m.agentSess.permBridge)
}
