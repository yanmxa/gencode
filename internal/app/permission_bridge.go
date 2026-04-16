package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appapproval "github.com/yanmxa/gencode/internal/app/approval"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// permBridgeRequest holds a pending permission request from the core.Agent
// goroutine, along with a channel to send the response back.
type permBridgeRequest struct {
	ToolCall core.ToolCall
	Request  *perm.PermissionRequest
	Response chan permBridgeResponse
}

type permBridgeResponse struct {
	Allow  bool
	Reason string
}

// permissionBridge connects the core.Agent's synchronous PermissionFunc
// with the TUI's asynchronous approval dialog. The agent goroutine blocks
// on the response channel while the TUI shows the approval prompt.
type permissionBridge struct {
	requests   chan *permBridgeRequest
	settingsFn func() *config.Settings
	sessionFn  func() *config.SessionPermissions
	cwdFn      func() string
}

// newPermissionBridge creates a permissionBridge that checks permissions using
// the provided accessor functions.
func newPermissionBridge(
	settingsFn func() *config.Settings,
	sessionFn func() *config.SessionPermissions,
	cwdFn func() string,
) *permissionBridge {
	return &permissionBridge{
		requests:   make(chan *permBridgeRequest, 1),
		settingsFn: settingsFn,
		sessionFn:  sessionFn,
		cwdFn:      cwdFn,
	}
}

// PermissionFunc returns a core.PermissionFunc that blocks the agent goroutine
// when user confirmation is needed. Allow and Deny decisions return immediately.
func (pb *permissionBridge) PermissionFunc() core.PermissionFunc {
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
		req := &permBridgeRequest{
			ToolCall: tc,
			Request: &perm.PermissionRequest{
				ToolName:    tc.Name,
				Description: decision.Reason,
			},
			Response: make(chan permBridgeResponse, 1),
		}

		select {
		case pb.requests <- req:
		case <-ctx.Done():
			return false, "cancelled"
		}

		select {
		case <-ctx.Done():
			return false, "cancelled"
		case resp := <-req.Response:
			return resp.Allow, resp.Reason
		}
	}
}

// PollCmd returns a tea.Cmd that blocks until a permission request arrives
// from the agent goroutine, then emits it as an agentPermissionMsg.
// Each successful delivery re-schedules itself to handle subsequent requests.
func (pb *permissionBridge) PollCmd() tea.Cmd {
	return func() tea.Msg {
		req, ok := <-pb.requests
		if !ok {
			return nil // bridge closed
		}
		return agentPermissionMsg{Request: req}
	}
}

// Close shuts down the bridge, unblocking any pending PollCmd.
func (pb *permissionBridge) Close() {
	close(pb.requests)
}

// agentPermissionMsg carries a permission request from the bridge to the TUI.
type agentPermissionMsg struct {
	Request *permBridgeRequest
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

	// Always resume polling for subsequent permission requests
	return m.agentSess.permBridge.PollCmd()
}
