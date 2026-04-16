package loop

import (
	"context"

	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/core"
)

// Run drives the full conversation loop using an explicit state machine:
// while-true { pre-check → stream → error recovery → tools → next state }.
// Stops on end_turn, max turns, stop hook, recovery exhaustion, or context cancellation.
func (l *Loop) Run(ctx context.Context, opts RunOptions) (*Result, error) {
	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns
	}
	maxRecovery := opts.MaxOutputRecovery
	if maxRecovery <= 0 {
		maxRecovery = DefaultMaxOutputRecovery
	}

	state := loopState{
		turnCount:  1,
		transition: transitionNextTurn,
	}
	var toolUses int
	var transitions []transitionReason

	terminate := func(reason string) *Result {
		return l.buildResult(reason, state.turnCount, toolUses, transitions)
	}

	for {
		select {
		case <-ctx.Done():
			return terminate(StopCancelled), ctx.Err()
		default:
		}

		transitions = append(transitions, state.transition)

		if opts.DrainInjectedInputs != nil {
			for _, injected := range opts.DrainInjectedInputs() {
				if injected == "" {
					continue
				}
				l.AddUser(injected, nil)
			}
		}

		if opts.InputLimit > 0 && l.Client != nil {
			tokens := l.Client.Tokens()
			if core.NeedsCompaction(tokens.InputTokens, opts.InputLimit) && CanCompactMessages(len(l.messages)) {
				if l.compactAndReplace(ctx, opts) {
					state.transition = transitionPromptTooLong
					continue
				}
			}
		}

		resp, err := Collect(ctx, l.Stream(ctx))
		if err != nil {
			if ShouldCompactPromptTooLong(err, len(l.messages)) {
				if l.compactAndReplace(ctx, opts) {
					state.transition = transitionPromptTooLong
					continue
				}
			}
			return nil, err
		}

		calls := l.AddResponse(resp)
		if opts.OnResponse != nil {
			opts.OnResponse(resp)
		}

		decision := DecideCompletion(resp.StopReason, calls, state.maxOutputTokensRecoveryCount, maxRecovery)

		if decision.Action == CompletionRecoverMaxTokens {
			l.AddUser(MaxOutputRecoveryPrompt, nil)
			state.maxOutputTokensRecoveryCount++
			state.transition = transitionMaxOutputRecovery
			continue
		}
		if decision.Action == CompletionStopMaxOutputRecovery {
			return terminate(StopMaxOutputRecoveryExhausted), nil
		}

		if decision.Action == CompletionEndTurn {
			if l.Hooks != nil && l.Hooks.HasHooks(hook.Stop) {
				stopInput := hook.HookInput{
					LastAssistantMessage: l.lastAssistantContent(),
					StopHookActive:       l.Hooks.StopHookActive(),
				}
				outcome := l.Hooks.Execute(ctx, hook.Stop, stopInput)
				if outcome.ShouldBlock {
					r := terminate(StopHook)
					r.StopDetail = "Stop hook blocked: " + outcome.BlockReason
					r.transitions = append(r.transitions, transitionStopHookBlocking)
					return r, nil
				}
			}
			return terminate(StopEndTurn), nil
		}

		allowed, blocked, _, hookContext := l.FilterToolCalls(ctx, decision.ToolCalls)
		_ = hookContext
		for _, br := range blocked {
			l.AddToolResult(br)
		}

		for _, tc := range allowed {
			select {
			case <-ctx.Done():
				return terminate(StopCancelled), ctx.Err()
			default:
			}

			if opts.OnToolStart != nil && !opts.OnToolStart(tc) {
				continue
			}

			result := l.ExecTool(ctx, tc)
			l.AddToolResult(*result)
			toolUses++

			l.firePostToolHook(ctx, tc, result)

			if opts.OnToolDone != nil {
				opts.OnToolDone(tc, *result)
			}
		}

		if state.turnCount >= maxTurns {
			return terminate(StopMaxTurns), nil
		}

		state = loopState{
			turnCount:                    state.turnCount + 1,
			maxOutputTokensRecoveryCount: state.maxOutputTokensRecoveryCount,
			transition:                   transitionNextTurn,
		}
	}
}

func (l *Loop) buildResult(reason string, turns, toolUses int, transitions []transitionReason) *Result {
	msgs := make([]core.Message, len(l.messages))
	copy(msgs, l.messages)
	return &Result{
		Content:     l.lastAssistantContent(),
		Messages:    msgs,
		Turns:       turns,
		ToolUses:    toolUses,
		Tokens:      l.Client.Tokens(),
		StopReason:  reason,
		transitions: transitions,
	}
}
