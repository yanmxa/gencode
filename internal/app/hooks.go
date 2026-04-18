// Hook-forwarding model methods and related helpers.
// These methods use m.services.Hook to fire lifecycle events,
// replacing the former env hook methods that used singletons directly.
package app

import (
	"context"
	"strings"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
)

func (m *model) firePostToolHook(tr core.ToolResult, sideEffect any) {
	if m.services.Hook == nil {
		return
	}
	eventType := hook.PostToolUse
	if tr.IsError {
		eventType = hook.PostToolUseFailure
	}
	toolResponse := any(tr.Content)
	if sideEffect != nil {
		toolResponse = sideEffect
	}
	input := hook.HookInput{
		ToolName:     tr.ToolName,
		ToolUseID:    tr.ToolCallID,
		ToolResponse: toolResponse,
	}
	if tr.IsError {
		input.Error = tr.Content
	}
	m.services.Hook.ExecuteAsync(eventType, input)
}

func (m *model) fireStopFailureHook(lastAssistantContent string, err error) {
	if m.services.Hook == nil {
		return
	}
	m.services.Hook.ExecuteAsync(hook.StopFailure, hook.HookInput{
		LastAssistantMessage: lastAssistantContent,
		Error:                err.Error(),
		StopHookActive:       m.services.Hook.StopHookActive(),
	})
}

func (m *model) executeStartupHooks(ctx context.Context) hook.HookOutcome {
	if m.services.Hook == nil {
		return hook.HookOutcome{}
	}
	m.services.Hook.ExecuteAsync(hook.Setup, hook.HookInput{
		Trigger: "init",
	})
	source := "startup"
	if m.services.Session.ID() != "" {
		source = "resume"
	}
	return m.services.Hook.Execute(ctx, hook.SessionStart, hook.HookInput{
		Source: source,
		Model:  m.env.GetModelID(),
	})
}

func (m *model) executeIdleHooks(ctx context.Context, lastAssistantContent string) (blocked bool, reason string) {
	if m.services.Hook == nil {
		return false, ""
	}
	if m.services.Hook.HasHooks(hook.Stop) {
		outcome := m.services.Hook.Execute(ctx, hook.Stop, hook.HookInput{
			LastAssistantMessage: lastAssistantContent,
			StopHookActive:       m.services.Hook.StopHookActive(),
		})
		if outcome.ShouldBlock {
			blocked = true
			reason = outcome.BlockReason
		}
	}
	m.services.Hook.ExecuteAsync(hook.Notification, hook.HookInput{
		Message:          "Claude is waiting for your input",
		NotificationType: "idle_prompt",
	})
	return blocked, reason
}

func (m *model) checkPromptHook(ctx context.Context, prompt string) (bool, string) {
	if m.services.Hook == nil {
		return false, ""
	}
	outcome := m.services.Hook.Execute(ctx, hook.UserPromptSubmit, hook.HookInput{Prompt: prompt})
	return outcome.ShouldBlock, outcome.BlockReason
}

func (m *model) switchProvider(p llm.Provider) {
	m.env.LLMProvider = p
	if m.services.Hook != nil {
		m.services.Hook.SetLLMCompleter(buildHookCompleter(p), m.env.GetModelID())
	}
}

func (m *model) refreshMemoryContext(cwd, loadReason string) {
	files := system.LoadMemoryFiles(cwd)
	var userParts, projectParts []string
	for _, f := range files {
		switch f.Level {
		case "global":
			userParts = append(userParts, f.Content)
		case "project", "local":
			projectParts = append(projectParts, f.Content)
		}
		if m.services.Hook != nil {
			m.services.Hook.ExecuteAsync(hook.InstructionsLoaded, hook.HookInput{
				FilePath:   f.Path,
				MemoryType: memoryTypeForLevel(f.Level),
				LoadReason: loadReason,
			})
		}
	}
	m.env.CachedUserInstructions = joinSections(userParts)
	m.env.CachedProjectInstructions = joinSections(projectParts)
}

func (m *model) syncSettingsToHookEngine() {
	if m.services.Hook != nil && m.services.Setting != nil {
		m.services.Hook.SetSettings(m.services.Setting.Snapshot())
	}
}

func memoryTypeForLevel(level string) string {
	switch level {
	case "global":
		return "User"
	case "local":
		return "Local"
	default:
		return "Project"
	}
}

func joinSections(parts []string) string {
	return strings.Join(parts, "\n\n")
}

func buildHookCompleter(p llm.Provider) hook.LLMCompleter {
	if p == nil {
		return nil
	}
	return func(ctx context.Context, systemPrompt, userMessage, model string) (string, error) {
		c := llm.NewClient(p, model, 0)
		resp, err := c.Complete(ctx, systemPrompt, []core.Message{{
			Role:    core.RoleUser,
			Content: userMessage,
		}}, 4096)
		if err != nil {
			return "", err
		}
		return resp.Content, nil
	}
}
