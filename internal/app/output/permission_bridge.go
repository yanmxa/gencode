package output

import (
	"context"

	"github.com/yanmxa/gencode/internal/core"
)

// PermDecision represents a permission decision outcome.
type PermDecision int

const (
	PermAllow  PermDecision = iota
	PermDeny
	PermPrompt
)

// PermDecisionResult holds a permission decision and its reason.
type PermDecisionResult struct {
	Decision    PermDecision
	Reason      string
	ToolName    string
	Description string
}

// PermDecisionFunc evaluates whether a tool call is allowed, denied, or needs prompting.
type PermDecisionFunc func(name string, args map[string]any) PermDecisionResult

type PermBridgeRequest struct {
	ToolCall    core.ToolCall
	ToolName    string
	Description string
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

func (pb *PermissionBridge) PermissionFunc() core.PermissionFunc {
	return func(ctx context.Context, tc core.ToolCall) (bool, string) {
		args, _ := core.ParseToolInput(tc.Input)
		decision := pb.decideFn(tc.Name, args)

		switch decision.Decision {
		case PermAllow:
			return true, decision.Reason
		case PermDeny:
			return false, decision.Reason
		}

		req := &PermBridgeRequest{
			ToolCall:    tc,
			ToolName:    decision.ToolName,
			Description: decision.Description,
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
