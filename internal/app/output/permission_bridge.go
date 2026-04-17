package output

import (
	"context"

	"github.com/yanmxa/gencode/internal/tool/perm"
)

// PermDecisionResult holds a permission decision and its reason.
type PermDecisionResult struct {
	Decision    perm.Decision
	Reason      string
	ToolName    string
	Description string
}

// PermDecisionFunc evaluates whether a tool call is allowed, denied, or needs prompting.
type PermDecisionFunc func(name string, args map[string]any) PermDecisionResult

type PermBridgeRequest struct {
	ToolName    string
	Description string
	Input       map[string]any
	Response    chan PermBridgeResponse
}

type PermBridgeResponse struct {
	Allow  bool
	Reason string
}

type PermissionBridge struct {
	requests chan *PermBridgeRequest
	decideFn PermDecisionFunc
}

func NewPermissionBridge(decideFn PermDecisionFunc) *PermissionBridge {
	return &PermissionBridge{
		requests: make(chan *PermBridgeRequest, 1),
		decideFn: decideFn,
	}
}

// PermissionFunc returns a perm.PermissionFunc that gates tool execution.
// Safe tools are handled by the WithPermission decorator — this function
// only deals with Permit/Reject/Prompt logic.
func (pb *PermissionBridge) PermissionFunc() perm.PermissionFunc {
	return func(ctx context.Context, name string, input map[string]any) (bool, string) {
		decision := pb.decideFn(name, input)

		switch decision.Decision {
		case perm.Permit:
			return true, decision.Reason
		case perm.Reject:
			return false, decision.Reason
		}

		req := &PermBridgeRequest{
			ToolName:    decision.ToolName,
			Description: decision.Description,
			Input:       input,
			Response:    make(chan PermBridgeResponse, 1),
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
