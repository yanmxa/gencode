// Shared mutable app state: provider, session, permissions, plan, and cache.
//
// Singletons are accessed via Default() service accessors (e.g. hook.Default(),
// setting.Default(), llm.Default()) — not copied here.
package app

import (
	"context"
	"strings"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
)

type Env struct {
	// ── Provider (mutable — changes via SwitchProvider) ─────────
	LLMProvider      llm.Provider
	CurrentModel     *llm.CurrentModelInfo
	InputTokens      int
	OutputTokens     int
	ThinkingLevel    llm.ThinkingLevel
	ThinkingOverride llm.ThinkingLevel

	// ── Permission (mutable — changes per mode cycle) ───────────
	OperationMode      setting.OperationMode
	SessionPermissions *setting.SessionPermissions

	// ── Plan (mutable — changes per plan mode) ──────────────────
	PlanEnabled bool
	PlanTask    string
	PlanStore   *plan.Store

	// ── Cache (session-scoped) ──────────────────────────────────
	FileCache                 *filecache.Cache
	CachedUserInstructions    string
	CachedProjectInstructions string
}

func newEnv() Env {
	return Env{
		OperationMode:      setting.ModeNormal,
		SessionPermissions: setting.NewSessionPermissions(),

		LLMProvider:  llm.Default().Provider(),
		CurrentModel: llm.Default().CurrentModel(),

		FileCache: filecache.New(),
	}
}

func (m *Env) GetModelID() string {
	if m.CurrentModel != nil {
		return m.CurrentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

func (m *Env) EffectiveThinkingLevel() llm.ThinkingLevel {
	return max(m.ThinkingLevel, m.ThinkingOverride)
}

func (m *Env) OperationModeName() string {
	switch m.OperationMode {
	case setting.ModeAutoAccept:
		return "auto"
	case setting.ModePlan:
		return "plan"
	case setting.ModeBypassPermissions:
		return "bypassPermissions"
	default:
		return "default"
	}
}

func (m *Env) CycleOperationMode() {
	allowBypass := setting.DefaultIfInit() != nil && setting.Default().AllowBypass()
	m.OperationMode = m.OperationMode.NextWithBypass(allowBypass)
	m.PlanEnabled = m.OperationMode == setting.ModePlan
}

func (m *Env) ResetSessionPermissions() {
	m.SessionPermissions.AllowAllEdits = false
	m.SessionPermissions.AllowAllWrites = false
	m.SessionPermissions.AllowAllBash = false
	m.SessionPermissions.AllowAllSkills = false
	m.SessionPermissions.Mode = setting.ModeNormal
}

func (m *Env) ApplyAutoAcceptPermissions(cwd string) {
	m.SessionPermissions.AllowAllEdits = true
	m.SessionPermissions.AllowAllWrites = true
	m.SessionPermissions.AddWorkingDirectory(cwd)
	for _, pattern := range setting.CommonAllowPatterns {
		m.SessionPermissions.AllowPattern(pattern)
	}
}

func (m *Env) ApplyBypassPermissions() {
	m.SessionPermissions.Mode = setting.ModeBypassPermissions
}

func (m *Env) EnableAutoAcceptMode(cwd string) {
	m.ApplyAutoAcceptPermissions(cwd)
	m.OperationMode = setting.ModeAutoAccept
	m.PlanEnabled = false
}

func (m *Env) DetectThinkingKeywords(input string) {
	lower := strings.ToLower(input)

	if strings.Contains(lower, "ultrathink") ||
		strings.Contains(lower, "think really hard") ||
		strings.Contains(lower, "think super hard") ||
		strings.Contains(lower, "maximum thinking") {
		m.ThinkingOverride = llm.ThinkingUltra
		return
	}

	if strings.Contains(lower, "think harder") ||
		strings.Contains(lower, "think hard") ||
		strings.Contains(lower, "think deeply") ||
		strings.Contains(lower, "think carefully") {
		m.ThinkingOverride = llm.ThinkingHigh
		return
	}
}

func (m *Env) ApplyModePermissions(cwd string) {
	m.ResetSessionPermissions()

	if m.OperationMode == setting.ModeAutoAccept {
		m.ApplyAutoAcceptPermissions(cwd)
	}

	if m.OperationMode == setting.ModeBypassPermissions {
		m.ApplyBypassPermissions()
	}
}

func (m *Env) EnsurePlanStore() {
	if m.PlanStore != nil {
		return
	}
	store, err := plan.NewStore()
	if err != nil {
		return
	}
	m.PlanStore = store
}

func (m *Env) ClearCachedInstructions() {
	m.CachedUserInstructions = ""
	m.CachedProjectInstructions = ""
}

func (m *Env) RefreshMemoryContext(cwd, loadReason string) {
	files := system.LoadMemoryFiles(cwd)
	var userParts, projectParts []string
	for _, f := range files {
		switch f.Level {
		case "global":
			userParts = append(userParts, f.Content)
		case "project", "local":
			projectParts = append(projectParts, f.Content)
		}
		if svc := hook.DefaultIfInit(); svc != nil {
			svc.ExecuteAsync(hook.InstructionsLoaded, hook.HookInput{
				FilePath:   f.Path,
				MemoryType: memoryTypeForLevel(f.Level),
				LoadReason: loadReason,
			})
		}
	}
	m.CachedUserInstructions = joinSections(userParts)
	m.CachedProjectInstructions = joinSections(projectParts)
}

func syncSettingsToHookEngine() {
	if svc := hook.DefaultIfInit(); svc != nil && setting.DefaultIfInit() != nil {
		svc.SetSettings(setting.Default().Snapshot())
	}
}

func (m *Env) CheckPromptHook(ctx context.Context, prompt string) (bool, string) {
	if svc := hook.DefaultIfInit(); svc != nil {
		outcome := svc.Execute(ctx, hook.UserPromptSubmit, hook.HookInput{Prompt: prompt})
		return outcome.ShouldBlock, outcome.BlockReason
	}
	return false, ""
}

func (m *Env) SwitchProvider(p llm.Provider) {
	m.LLMProvider = p
	if svc := hook.DefaultIfInit(); svc != nil {
		svc.SetLLMCompleter(buildHookCompleter(p), m.GetModelID())
	}
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

func (m *Env) SessionMode() string {
	if m.PlanEnabled {
		return "plan"
	}
	switch m.OperationMode {
	case setting.ModeAutoAccept:
		return "auto-accept"
	default:
		return "normal"
	}
}

func (m *Env) ClearThinkingOverride() {
	m.ThinkingOverride = llm.ThinkingOff
}

func (m *Env) ResetTokens() {
	m.InputTokens = 0
	m.OutputTokens = 0
}


func (m *Env) FirePostToolHook(tr core.ToolResult, sideEffect any) {
	svc := hook.DefaultIfInit()
	if svc == nil {
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
	svc.ExecuteAsync(eventType, input)
}

func (m *Env) FireStopFailureHook(lastAssistantContent string, err error) {
	svc := hook.DefaultIfInit()
	if svc == nil {
		return
	}
	svc.ExecuteAsync(hook.StopFailure, hook.HookInput{
		LastAssistantMessage: lastAssistantContent,
		Error:                err.Error(),
		StopHookActive:       svc.StopHookActive(),
	})
}

func (m *Env) FireSessionEnd(ctx context.Context, reason string) {
	svc := hook.DefaultIfInit()
	if svc == nil {
		return
	}
	svc.Execute(ctx, hook.SessionEnd, hook.HookInput{
		Reason: reason,
	})
	svc.ClearSessionHooks()
}

func (m *Env) ExecuteStartupHooks(ctx context.Context) hook.HookOutcome {
	svc := hook.DefaultIfInit()
	if svc == nil {
		return hook.HookOutcome{}
	}
	svc.ExecuteAsync(hook.Setup, hook.HookInput{
		Trigger: "init",
	})
	source := "startup"
	if session.Default().ID() != "" {
		source = "resume"
	}
	return svc.Execute(ctx, hook.SessionStart, hook.HookInput{
		Source: source,
		Model:  m.GetModelID(),
	})
}

func (m *Env) ExecuteIdleHooks(ctx context.Context, lastAssistantContent string) (blocked bool, reason string) {
	svc := hook.DefaultIfInit()
	if svc == nil {
		return false, ""
	}
	if svc.HasHooks(hook.Stop) {
		outcome := svc.Execute(ctx, hook.Stop, hook.HookInput{
			LastAssistantMessage: lastAssistantContent,
			StopHookActive:       svc.StopHookActive(),
		})
		if outcome.ShouldBlock {
			blocked = true
			reason = outcome.BlockReason
		}
	}
	svc.ExecuteAsync(hook.Notification, hook.HookInput{
		Message:          "Claude is waiting for your input",
		NotificationType: "idle_prompt",
	})
	return blocked, reason
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
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "\n\n" + parts[i]
	}
	return result
}
