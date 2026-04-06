package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/message"
)

const hookJSONResponseInstruction = "Return exactly one JSON object matching the hook output schema. Do not include markdown fences or commentary."

func (e *Engine) executePromptHook(ctx context.Context, hookCmd config.HookCmd, input HookInput) HookOutcome {
	outcome := HookOutcome{ShouldContinue: true}
	llmProvider, model := e.getLLMConfig(hookCmd)
	if llmProvider == nil || model == "" {
		outcome.Error = fmt.Errorf("prompt hook requires an active provider and model")
		return outcome
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		outcome.Error = fmt.Errorf("failed to marshal input: %w", err)
		return outcome
	}

	c := &client.Client{Provider: llmProvider, Model: model}
	resp, err := c.Complete(ctx, hookJSONResponseInstruction, []message.Message{{
		Role:    message.RoleUser,
		Content: buildHookPrompt(hookCmd.Prompt, string(inputJSON)),
	}}, 2048)
	if err != nil {
		outcome.Error = err
		return outcome
	}

	return e.parseOutput(strings.TrimSpace(resp.Content), outcome)
}

func (e *Engine) executeAgentHook(ctx context.Context, hookCmd config.HookCmd, input HookInput) HookOutcome {
	outcome := HookOutcome{ShouldContinue: true}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		outcome.Error = fmt.Errorf("failed to marshal input: %w", err)
		return outcome
	}

	model := hookCmd.Model
	if model == "" {
		_, model = e.getLLMConfig(hookCmd)
	}
	prompt := buildHookPrompt(hookCmd.Prompt, string(inputJSON))

	if runner := e.getAgentRunner(); runner != nil {
		resp, err := runner.RunAgentHook(ctx, prompt, model)
		if err != nil {
			outcome.Error = err
			return outcome
		}
		return e.parseOutput(strings.TrimSpace(resp), outcome)
	}

	llmProvider, resolvedModel := e.getLLMConfig(hookCmd)
	if llmProvider == nil || resolvedModel == "" {
		outcome.Error = fmt.Errorf("agent hook requires an active provider/model or injected agent runner")
		return outcome
	}

	c := &client.Client{Provider: llmProvider, Model: resolvedModel}
	resp, err := c.Complete(ctx, "You are an autonomous hook verifier. Reason through the request and return only hook JSON.\n"+hookJSONResponseInstruction, []message.Message{{
		Role:    message.RoleUser,
		Content: prompt,
	}}, 4096)
	if err != nil {
		outcome.Error = err
		return outcome
	}
	return e.parseOutput(strings.TrimSpace(resp.Content), outcome)
}
