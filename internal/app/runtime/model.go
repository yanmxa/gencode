// Shared runtime state: provider, session, permissions, plan, and config.
// This is the 5th sub-Model in the root MVU, alongside user, agent, system, and output.
package runtime

import (
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
