package app

import (
	"context"
	"encoding/json"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
)

// filterToolCallsResult holds the results from PreToolUse hook filtering.
type filterToolCallsResult struct {
	Allowed           []message.ToolCall
	Blocked           []message.ToolResult
	HookAllowed       map[string]bool // tool call IDs pre-approved by hooks
	HookForceAsk      map[string]bool // tool call IDs forced to prompt by hooks
	AdditionalContext string
}

// filterToolCallsWithEngine runs PreToolUse hooks via the given engine and
// returns the filtering result.
func filterToolCallsWithEngine(ctx context.Context, engine *hooks.Engine, calls []message.ToolCall) filterToolCallsResult {
	r := filterToolCallsResult{
		HookAllowed:  make(map[string]bool),
		HookForceAsk: make(map[string]bool),
	}
	if engine == nil {
		r.Allowed = calls
		return r
	}

	for _, tc := range calls {
		params, _ := message.ParseToolInput(tc.Input)
		hookInput := hooks.HookInput{
			ToolName:  tc.Name,
			ToolInput: params,
			ToolUseID: tc.ID,
		}
		outcome := engine.Execute(ctx, hooks.PreToolUse, hookInput)

		if outcome.ShouldBlock {
			r.Blocked = append(r.Blocked, *message.ErrorResult(tc, "Blocked by hook: "+outcome.BlockReason))
			continue
		}

		if outcome.UpdatedInput != nil {
			if updated, err := json.Marshal(outcome.UpdatedInput); err == nil {
				tc.Input = string(updated)
			}
		}

		if outcome.AdditionalContext != "" {
			if r.AdditionalContext == "" {
				r.AdditionalContext = outcome.AdditionalContext
			} else {
				r.AdditionalContext += "\n" + outcome.AdditionalContext
			}
		}

		if outcome.PermissionAllow {
			r.HookAllowed[tc.ID] = true
		}
		if outcome.ForceAsk {
			r.HookForceAsk[tc.ID] = true
		}

		r.Allowed = append(r.Allowed, tc)
	}
	return r
}
