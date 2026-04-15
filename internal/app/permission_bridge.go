package app

import (
	"context"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// PermBridgeRequest holds a pending permission request from the core.Agent
// goroutine, along with a channel to send the response back.
type PermBridgeRequest struct {
	ToolCall core.ToolCall
	Request  *perm.PermissionRequest
	Response chan permBridgeResponse
}

type permBridgeResponse struct {
	Allow  bool
	Reason string
}

// PermissionBridge connects the core.Agent's synchronous PermissionFunc
// with the TUI's asynchronous approval dialog. The agent goroutine blocks
// on the response channel while the TUI shows the approval prompt.
type PermissionBridge struct {
	mu        sync.Mutex
	pending   *PermBridgeRequest
	settingsFn func() *config.Settings
	sessionFn  func() *config.SessionPermissions
	cwdFn      func() string
}

// NewPermissionBridge creates a PermissionBridge that checks permissions using
// the provided accessor functions.
func NewPermissionBridge(
	settingsFn func() *config.Settings,
	sessionFn func() *config.SessionPermissions,
	cwdFn func() string,
) *PermissionBridge {
	return &PermissionBridge{
		settingsFn: settingsFn,
		sessionFn:  sessionFn,
		cwdFn:      cwdFn,
	}
}

// PermissionFunc returns a core.PermissionFunc that blocks the agent goroutine
// when user confirmation is needed. Allow and Deny decisions return immediately.
func (pb *PermissionBridge) PermissionFunc() core.PermissionFunc {
	return func(ctx context.Context, tc core.ToolCall) (bool, string) {
		settings := pb.settingsFn()
		if settings == nil {
			return true, ""
		}

		decision := settings.HasPermissionToUseTool(tc.Name, tc.Input, pb.sessionFn())

		switch decision.Behavior {
		case config.Allow:
			return true, decision.Reason
		case config.Deny:
			return false, decision.Reason
		}

		// Ask — block the agent goroutine and wait for TUI response
		req := &PermBridgeRequest{
			ToolCall: tc,
			Request: &perm.PermissionRequest{
				ToolName:    tc.Name,
				Description: decision.Reason,
			},
			Response: make(chan permBridgeResponse, 1),
		}

		pb.mu.Lock()
		pb.pending = req
		pb.mu.Unlock()

		select {
		case <-ctx.Done():
			return false, "cancelled"
		case resp := <-req.Response:
			return resp.Allow, resp.Reason
		}
	}
}

// PollCmd returns a tea.Cmd that polls for pending permission requests
// and emits them as agentPermissionMsg for the TUI to handle.
func (pb *PermissionBridge) PollCmd() tea.Cmd {
	return func() tea.Msg {
		pb.mu.Lock()
		req := pb.pending
		pb.pending = nil
		pb.mu.Unlock()

		if req != nil {
			return agentPermissionMsg{Request: req}
		}
		return nil
	}
}

// agentPermissionMsg carries a permission request from the bridge to the TUI.
type agentPermissionMsg struct {
	Request *PermBridgeRequest
}

// handlePermBridgeResponse sends the user's approval response back to the
// blocked agent goroutine and clears the pending state.
func (m *model) handlePermBridgeResponse(msg appapproval.ResponseMsg) tea.Cmd {
	req := m.pendingPermBridge
	m.pendingPermBridge = nil

	if req == nil {
		return nil
	}

	resp := permBridgeResponse{
		Allow:  msg.Approved,
		Reason: "user decision",
	}

	if msg.Approved {
		// Record session-level permission if "allow all" was selected
		if msg.AllowAll && m.mode.SessionPermissions != nil && msg.Request != nil {
			m.mode.SessionPermissions.AllowTool(msg.Request.ToolName)
		}
		resp.Reason = "user approved"
	} else {
		resp.Reason = "user denied"
	}

	select {
	case req.Response <- resp:
	default:
	}

	if msg.Approved {
		// Resume polling for more permission requests
		return m.agentSess.permBridge.PollCmd()
	}
	return nil
}
