// Shared mutable app state: provider, permissions, and cache.
// Pure state holder — no singleton service dependencies.
package app

import (
	"strings"

	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/llm"
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
	LLMProvider  llm.Provider
	CurrentModel *llm.CurrentModelInfo
	// InputTokens / OutputTokens track the latest infer call only.
	// They back the bottom-right context display, so they reflect the most
	// recent prompt/output size rather than a turn or session aggregate.
	InputTokens  int
	OutputTokens int
	// TurnInputTokens / TurnOutputTokens track the current agent turn.
	// A "turn" here means the whole think-act cycle, which may include multiple
	// LLM calls around tool use. These totals are reset at the first infer of a
	// new turn and then accumulated after each infer in that turn.
	TurnInputTokens  int
	TurnOutputTokens int
	turnUsageActive  bool
	ConversationCost llm.Money
	ThinkingEffort   string

	// ── Permission (mutable — changes per mode cycle) ───────────
	OperationMode      setting.OperationMode
	SessionPermissions *setting.SessionPermissions

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

func (m *env) EffectiveThinkingEffort() string {
	return llm.ResolveThinkingEffort(m.LLMProvider, m.GetModelID(), m.ThinkingEffort)
}

func (m *env) OperationModeName() string {
	switch m.OperationMode {
	case setting.ModeAutoAccept:
		return "auto"
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
}

func (m *env) DetectThinkingKeywords(input string) {
	lower := strings.ToLower(input)
	efforts := llm.ThinkingEfforts(m.LLMProvider, m.GetModelID())
	if len(efforts) == 0 {
		return
	}

	if strings.Contains(lower, "ultrathink") ||
		strings.Contains(lower, "think really hard") ||
		strings.Contains(lower, "think super hard") ||
		strings.Contains(lower, "maximum thinking") {
		m.ThinkingEffort = efforts[len(efforts)-1]
		return
	}

	if strings.Contains(lower, "think harder") ||
		strings.Contains(lower, "think hard") ||
		strings.Contains(lower, "think deeply") ||
		strings.Contains(lower, "think carefully") {
		if len(efforts) >= 2 {
			m.ThinkingEffort = efforts[len(efforts)-2]
		}
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

func (m *env) ClearCachedInstructions() {
	m.CachedUserInstructions = ""
	m.CachedProjectInstructions = ""
}

func (m *env) SessionMode() string {
	switch m.OperationMode {
	case setting.ModeAutoAccept:
		return "auto-accept"
	default:
		return "normal"
	}
}

func (m *env) ResetContextDisplay() {
	m.InputTokens = 0
	m.OutputTokens = 0
	m.ConversationCost = llm.Money{}
}

func (m *env) ResetTokens() {
	m.ResetContextDisplay()
	m.TurnInputTokens = 0
	m.TurnOutputTokens = 0
	m.turnUsageActive = false
}
