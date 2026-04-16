package output

import (
	"context"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

type PermBridgeRequest struct {
	ToolCall core.ToolCall
	Request  *perm.PermissionRequest
	Response chan PermBridgeResponse
}

type PermBridgeResponse struct {
	Allow  bool
	Reason string
}

type PermissionBridge struct {
	requests   chan *PermBridgeRequest
	settingsFn func() *config.Settings
	sessionFn  func() *config.SessionPermissions
	cwdFn      func() string
}

func NewPermissionBridge(
	settingsFn func() *config.Settings,
	sessionFn func() *config.SessionPermissions,
	cwdFn func() string,
) *PermissionBridge {
	return &PermissionBridge{
		requests:   make(chan *PermBridgeRequest, 1),
		settingsFn: settingsFn,
		sessionFn:  sessionFn,
		cwdFn:      cwdFn,
	}
}

func (pb *PermissionBridge) PermissionFunc() core.PermissionFunc {
	return func(ctx context.Context, tc core.ToolCall) (bool, string) {
		settings := pb.settingsFn()
		if settings == nil {
			return true, ""
		}

		args, _ := core.ParseToolInput(tc.Input)
		decision := settings.HasPermissionToUseTool(tc.Name, args, pb.sessionFn())

		switch decision.Behavior {
		case config.Allow:
			return true, decision.Reason
		case config.Deny:
			return false, decision.Reason
		}

		req := &PermBridgeRequest{
			ToolCall: tc,
			Request: &perm.PermissionRequest{
				ToolName:    tc.Name,
				Description: decision.Reason,
			},
			Response: make(chan PermBridgeResponse, 1),
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

func (pb *PermissionBridge) Recv() (*PermBridgeRequest, bool) {
	req, ok := <-pb.requests
	return req, ok
}

func (pb *PermissionBridge) Close() {
	close(pb.requests)
}
