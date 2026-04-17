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
