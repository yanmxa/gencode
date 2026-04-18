// Shared runtime state: provider, session, permissions, plan, and config.
// This is the 5th sub-Model in the root MVU, alongside user, agent, system, and output.
package runtime

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

type Model struct {
	// ── Provider ────────────────────────────────────────────────
	LLMProvider      llm.Provider
	ProviderStore    *llm.Store
	CurrentModel     *llm.CurrentModelInfo
	InputTokens      int
	OutputTokens     int
	ThinkingLevel    llm.ThinkingLevel
	ThinkingOverride llm.ThinkingLevel

	// ── Session ─────────────────────────────────────────────────
	SessionStore   *session.Store
	SessionID      string
	SessionSummary string

	// ── Permission ──────────────────────────────────────────────
	OperationMode      setting.OperationMode
	SessionPermissions *setting.SessionPermissions
	DisabledTools      map[string]bool

	// ── Plan ────────────────────────────────────────────────────
	PlanEnabled bool
	PlanTask    string
	PlanStore   *plan.Store

	// ── Config ──────────────────────────────────────────────────
	Settings   *setting.Settings
	HookEngine *hook.Engine
	FileCache  *filecache.Cache

	// ── Instructions (cached) ───────────────────────────────────
	CachedUserInstructions    string
	CachedProjectInstructions string
}

func (m *Model) GetModelID() string {
	if m.CurrentModel != nil {
		return m.CurrentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

func (m *Model) EffectiveThinkingLevel() llm.ThinkingLevel {
	return max(m.ThinkingLevel, m.ThinkingOverride)
}

// OperationModeName returns the string name for hook engine configuration.
func (m *Model) OperationModeName() string {
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

// CycleOperationMode advances the operation mode to the next value.
func (m *Model) CycleOperationMode() {
	allowBypass := m.Settings != nil && m.Settings.AllowBypass != nil && *m.Settings.AllowBypass
	m.OperationMode = m.OperationMode.NextWithBypass(allowBypass)
	m.PlanEnabled = m.OperationMode == setting.ModePlan
}

// ResetSessionPermissions resets all session permissions to defaults.
func (m *Model) ResetSessionPermissions() {
	m.SessionPermissions.AllowAllEdits = false
	m.SessionPermissions.AllowAllWrites = false
	m.SessionPermissions.AllowAllBash = false
	m.SessionPermissions.AllowAllSkills = false
	m.SessionPermissions.Mode = setting.ModeNormal
}

// ApplyAutoAcceptPermissions enables auto-accept permissions for the given cwd.
func (m *Model) ApplyAutoAcceptPermissions(cwd string) {
	m.SessionPermissions.AllowAllEdits = true
	m.SessionPermissions.AllowAllWrites = true
	m.SessionPermissions.AddWorkingDirectory(cwd)
	for _, pattern := range setting.CommonAllowPatterns {
		m.SessionPermissions.AllowPattern(pattern)
	}
}

// ApplyBypassPermissions enables bypass mode.
func (m *Model) ApplyBypassPermissions() {
	m.SessionPermissions.Mode = setting.ModeBypassPermissions
}

// EnableAutoAcceptMode fully enables auto-accept mode with permissions.
func (m *Model) EnableAutoAcceptMode(cwd string) {
	m.ApplyAutoAcceptPermissions(cwd)
	m.OperationMode = setting.ModeAutoAccept
	m.PlanEnabled = false
}

// DetectThinkingKeywords sets per-turn thinking override based on user input keywords.
func (m *Model) DetectThinkingKeywords(input string) {
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

// ApplyModePermissions resets session permissions and applies mode-specific rules.
func (m *Model) ApplyModePermissions(cwd string) {
	m.ResetSessionPermissions()

	if m.OperationMode == setting.ModeAutoAccept {
		m.ApplyAutoAcceptPermissions(cwd)
	}

	if m.OperationMode == setting.ModeBypassPermissions {
		m.ApplyBypassPermissions()
	}
}

// EnsurePlanStore lazily initializes the plan store.
func (m *Model) EnsurePlanStore() {
	if m.PlanStore != nil {
		return
	}
	store, err := plan.NewStore()
	if err != nil {
		return
	}
	m.PlanStore = store
}

// ClearCachedInstructions resets cached memory instructions.
func (m *Model) ClearCachedInstructions() {
	m.CachedUserInstructions = ""
	m.CachedProjectInstructions = ""
}

// RefreshMemoryContext loads GEN.md/CLAUDE.md files, fires InstructionsLoaded
// hooks, and updates cached instructions.
func (m *Model) RefreshMemoryContext(cwd, loadReason string) {
	files := system.LoadMemoryFiles(cwd)
	var userParts, projectParts []string
	for _, f := range files {
		switch f.Level {
		case "global":
			userParts = append(userParts, f.Content)
		case "project", "local":
			projectParts = append(projectParts, f.Content)
		}
		if m.HookEngine != nil {
			m.HookEngine.ExecuteAsync(hook.InstructionsLoaded, hook.HookInput{
				FilePath:   f.Path,
				MemoryType: memoryTypeForLevel(f.Level),
				LoadReason: loadReason,
			})
		}
	}
	m.CachedUserInstructions = joinSections(userParts)
	m.CachedProjectInstructions = joinSections(projectParts)
}

// ApplySettings updates runtime settings, disabled tools, and hook engine config.
func (m *Model) ApplySettings(s *setting.Settings) {
	m.Settings = s
	if m.DisabledTools == nil {
		m.DisabledTools = make(map[string]bool)
	} else {
		for k := range m.DisabledTools {
			delete(m.DisabledTools, k)
		}
	}
	for k, v := range s.DisabledTools {
		m.DisabledTools[k] = v
	}
	if m.HookEngine != nil {
		m.HookEngine.SetSettings(s)
	}
}

// CheckPromptHook runs UserPromptSubmit hook and returns (blocked, reason).
func (m *Model) CheckPromptHook(ctx context.Context, prompt string) (bool, string) {
	if m.HookEngine == nil {
		return false, ""
	}
	outcome := m.HookEngine.Execute(ctx, hook.UserPromptSubmit, hook.HookInput{Prompt: prompt})
	return outcome.ShouldBlock, outcome.BlockReason
}

// SwitchProvider updates the LLM provider and notifies the hook engine.
func (m *Model) SwitchProvider(p llm.Provider) {
	m.LLMProvider = p
	if m.HookEngine != nil {
		m.HookEngine.SetLLMProvider(m.LLMProvider, m.GetModelID())
	}
}

// SessionMode returns the current session mode string for session metadata.
func (m *Model) SessionMode() string {
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

// ClearThinkingOverride resets the per-turn thinking override.
func (m *Model) ClearThinkingOverride() {
	m.ThinkingOverride = llm.ThinkingOff
}

// ResetTokens clears input/output token counts.
func (m *Model) ResetTokens() {
	m.InputTokens = 0
	m.OutputTokens = 0
}

// EnsureSessionStore lazily initializes the session store.
func (m *Model) EnsureSessionStore(cwd string) error {
	if m.SessionStore == nil {
		store, err := session.NewStore(cwd)
		if err != nil {
			return err
		}
		m.SessionStore = store
	}
	return nil
}

// FirePostToolHook fires PostToolUse or PostToolUseFailure hook asynchronously.
func (m *Model) FirePostToolHook(tr core.ToolResult, sideEffect any) {
	if m.HookEngine == nil {
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
	m.HookEngine.ExecuteAsync(eventType, input)
}

// FireStopFailureHook fires the StopFailure hook asynchronously.
func (m *Model) FireStopFailureHook(lastAssistantContent string, err error) {
	if m.HookEngine == nil {
		return
	}
	m.HookEngine.ExecuteAsync(hook.StopFailure, hook.HookInput{
		LastAssistantMessage: lastAssistantContent,
		Error:                err.Error(),
		StopHookActive:       m.HookEngine.StopHookActive(),
	})
}

// FireSessionEnd fires the SessionEnd hook synchronously.
func (m *Model) FireSessionEnd(ctx context.Context, reason string) {
	if m.HookEngine == nil {
		return
	}
	m.HookEngine.Execute(ctx, hook.SessionEnd, hook.HookInput{
		Reason: reason,
	})
	m.HookEngine.ClearSessionHooks()
}

// ExecuteStartupHooks fires Setup and SessionStart hooks, returning the outcome.
func (m *Model) ExecuteStartupHooks(ctx context.Context) hook.HookOutcome {
	if m.HookEngine == nil {
		return hook.HookOutcome{}
	}
	m.HookEngine.ExecuteAsync(hook.Setup, hook.HookInput{
		Trigger: "init",
	})
	source := "startup"
	if m.SessionID != "" {
		source = "resume"
	}
	return m.HookEngine.Execute(ctx, hook.SessionStart, hook.HookInput{
		Source: source,
		Model:  m.GetModelID(),
	})
}

// ExecuteIdleHooks fires Stop and Notification hooks. Returns whether execution
// was blocked and the block reason.
func (m *Model) ExecuteIdleHooks(ctx context.Context, lastAssistantContent string) (blocked bool, reason string) {
	if m.HookEngine == nil {
		return false, ""
	}
	if m.HookEngine.HasHooks(hook.Stop) {
		outcome := m.HookEngine.Execute(ctx, hook.Stop, hook.HookInput{
			LastAssistantMessage: lastAssistantContent,
			StopHookActive:       m.HookEngine.StopHookActive(),
		})
		if outcome.ShouldBlock {
			blocked = true
			reason = outcome.BlockReason
		}
	}
	m.HookEngine.ExecuteAsync(hook.Notification, hook.HookInput{
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
