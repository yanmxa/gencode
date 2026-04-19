// Shared mutable app state: provider, permissions, plan, and cache.
// Pure state holder — no singleton service dependencies.
package app

import (
	"strings"

	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/setting"
)

type env struct {
	// ── App-level state ─────────────────────────────────────────
	CWD           string
	IsGit         bool
	Width         int
	Height        int
	Ready         bool
	InitialPrompt string

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

func newEnv(llmSvc llm.Service, cwd string, isGit bool) env {
	return env{
		CWD:   cwd,
		IsGit: isGit,

		OperationMode:      setting.ModeNormal,
		SessionPermissions: setting.NewSessionPermissions(),

		LLMProvider:  llmSvc.Provider(),
		CurrentModel: llmSvc.CurrentModel(),

		FileCache: filecache.New(),
	}
}

func (m *env) GetModelID() string {
	if m.CurrentModel != nil {
		return m.CurrentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

func (m *env) EffectiveThinkingLevel() llm.ThinkingLevel {
	return max(m.ThinkingLevel, m.ThinkingOverride)
}

func (m *env) OperationModeName() string {
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

func (m *env) ResetSessionPermissions() {
	m.SessionPermissions.AllowAllEdits = false
	m.SessionPermissions.AllowAllWrites = false
	m.SessionPermissions.AllowAllBash = false
	m.SessionPermissions.AllowAllSkills = false
	m.SessionPermissions.Mode = setting.ModeNormal
}

func (m *env) ApplyAutoAcceptPermissions(cwd string) {
	m.SessionPermissions.AllowAllEdits = true
	m.SessionPermissions.AllowAllWrites = true
	m.SessionPermissions.AddWorkingDirectory(cwd)
	for _, pattern := range setting.CommonAllowPatterns {
		m.SessionPermissions.AllowPattern(pattern)
	}
}

func (m *env) ApplyBypassPermissions() {
	m.SessionPermissions.Mode = setting.ModeBypassPermissions
}

func (m *env) EnableAutoAcceptMode(cwd string) {
	m.ApplyAutoAcceptPermissions(cwd)
	m.OperationMode = setting.ModeAutoAccept
	m.PlanEnabled = false
}

func (m *env) DetectThinkingKeywords(input string) {
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

func (m *env) ApplyModePermissions(cwd string) {
	m.ResetSessionPermissions()

	if m.OperationMode == setting.ModeAutoAccept {
		m.ApplyAutoAcceptPermissions(cwd)
	}

	if m.OperationMode == setting.ModeBypassPermissions {
		m.ApplyBypassPermissions()
	}
}

func (m *env) EnsurePlanStore() {
	if m.PlanStore != nil {
		return
	}
	store, err := plan.NewStore()
	if err != nil {
		return
	}
	m.PlanStore = store
}

func (m *env) ClearCachedInstructions() {
	m.CachedUserInstructions = ""
	m.CachedProjectInstructions = ""
}

func (m *env) SessionMode() string {
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

func (m *env) ClearThinkingOverride() {
	m.ThinkingOverride = llm.ThinkingOff
}

func (m *env) ResetTokens() {
	m.InputTokens = 0
	m.OutputTokens = 0
}
