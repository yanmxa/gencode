// Settings are loaded from multiple sources with the following priority
// (lowest to highest):
//  1. ~/.claude/settings.json (Claude user level - compatibility)
//  2. ~/.san/settings.json (San user level)
//  3. .claude/settings.json (Claude project level - compatibility)
//  4. .san/settings.json (San project level)
//  5. .claude/settings.local.json (Claude local level - compatibility)
//  6. .san/settings.local.json (San local level)
//  7. Environment variables / CLI arguments
//  8. managed-settings.json (system level - cannot be overridden)
package setting

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/genai-io/san/internal/confdir"
	permissionpolicy "github.com/genai-io/san/internal/permission"
)

// Data represents the complete San configuration.
type Data struct {
	Permissions    PermissionSettings `json:"permissions,omitempty"`
	Model          string             `json:"model,omitempty"`
	Hooks          map[string][]Hook  `json:"hooks,omitempty"`
	Env            map[string]string  `json:"env,omitempty"`
	EnabledPlugins map[string]bool    `json:"enabledPlugins,omitempty"`
	DisabledTools  map[string]bool    `json:"disabledTools,omitempty"`
	Theme          string             `json:"theme,omitempty"`
	SearchProvider string             `json:"searchProvider,omitempty"`
	AllowBypass    *bool              `json:"allowBypass,omitempty"`
	// ContextBar toggles the visual context-usage bar ([██████░░░░] 71%) in
	// the status line. Pointer so an explicit "off" persists distinctly from
	// "unset"; nil (unset) means off — the bar is opt-in. The numeric
	// "ctx X/Y" label is unaffected and always shows.
	ContextBar *bool `json:"contextBar,omitempty"`
	// Persona selects an active persona directory under ~/.san/personas/<name>/
	// or .san/personas/<name>/. Empty = no persona override. The persona's own
	// settings.json is applied as the highest config overlay (see
	// ApplyPersonaOverlay).
	Persona string `json:"persona,omitempty"`
	// SelfLearn toggles + tunes the self-learning loop (per-turn background
	// review of memory and skills). Both arms are off by default (opt-in).
	SelfLearn SelfLearnSettings `json:"selfLearn,omitempty"`
}

// SelfLearnSettings configures the two independent self-learning arms.
// See notes/active/l1-background-review.md §3.1.
type SelfLearnSettings struct {
	Memory SelfLearnMemory `json:"memory,omitempty"`
	Skills SelfLearnSkills `json:"skills,omitempty"`
}

// SelfLearnMaxMemoryKB is the upper bound on memory.maxKB and the default
// when the config field is zero. It matches the injection cap
// (autoMemoryByteCap = 25 KB) so the on-disk per-file cap can never exceed
// what the loader would have to truncate — see §4.2 invariant.
const SelfLearnMaxMemoryKB = 25

// SelfLearnDefaultEveryTurns and SelfLearnDefaultEveryToolIters are the
// review-cadence defaults applied when the corresponding config field is
// zero — the single source of truth shared by the runtime reviewer
// (ResolveSettings) and the /config panel's default display.
const (
	SelfLearnDefaultEveryTurns     = 10
	SelfLearnDefaultEveryToolIters = 10
)

// SelfLearnMemory controls memory-evolving: review every N user turns. MaxKB
// is the on-disk cap per memory file; lower values force more aggressive
// pruning. May not exceed SelfLearnMaxMemoryKB.
type SelfLearnMemory struct {
	Enabled    bool `json:"enabled,omitempty"`
	EveryTurns int  `json:"everyTurns,omitempty"` // 0 = SelfLearnDefaultEveryTurns
	MaxKB      int  `json:"maxKB,omitempty"`      // 0 = SelfLearnMaxMemoryKB
}

// SelfLearnSkills controls skill-evolving: review when accumulated tool
// iterations since the last review reach EveryToolIters, plus the action
// permissions of §5.5.
//
// Permission fields are encoded as Deny* booleans so the zero value is
// "allow" — the Go idiom of "zero value should be sensible default" — and
// every settings.json that omits the field gets the conservative
// permissive default without any pointer-vs-nil ceremony.
type SelfLearnSkills struct {
	Enabled        bool `json:"enabled,omitempty"`
	EveryToolIters int  `json:"everyToolIters,omitempty"` // 0 = SelfLearnDefaultEveryToolIters

	// DenyCreate / DenyUpdate / DenyDelete gate the corresponding action on
	// agent-created skills. Zero ⇒ allowed; set to true to disable that
	// action. See §5.5.
	DenyCreate bool `json:"denyCreate,omitempty"`
	DenyUpdate bool `json:"denyUpdate,omitempty"`
	DenyDelete bool `json:"denyDelete,omitempty"`

	// AllowUpdateUserCreated is the advanced opt-in that extends update to
	// also patch user-created skills (Hermes-style). Default false. Even
	// when true, create and delete on user-created remain impossible at
	// any config setting.
	AllowUpdateUserCreated bool `json:"allowUpdateUserCreated,omitempty"`
}

// AllowCreate / AllowUpdate / AllowDelete report whether the named action
// is permitted under the current configuration. These are the read paths
// the runtime takes — settings.json stores Deny*.
func (s SelfLearnSkills) AllowCreate() bool { return !s.DenyCreate }
func (s SelfLearnSkills) AllowUpdate() bool { return !s.DenyUpdate }
func (s SelfLearnSkills) AllowDelete() bool { return !s.DenyDelete }

// ResolvedMaxKB returns the resolved MaxKB (default SelfLearnMaxMemoryKB if zero).
func (m SelfLearnMemory) ResolvedMaxKB() int {
	if m.MaxKB <= 0 {
		return SelfLearnMaxMemoryKB
	}
	return m.MaxKB
}

// ResolvedEveryTurns returns the resolved memory review cadence in user turns
// (default SelfLearnDefaultEveryTurns when the field is zero).
func (m SelfLearnMemory) ResolvedEveryTurns() int {
	if m.EveryTurns <= 0 {
		return SelfLearnDefaultEveryTurns
	}
	return m.EveryTurns
}

// ResolvedEveryToolIters returns the resolved skill review cadence in tool
// iterations (default SelfLearnDefaultEveryToolIters when the field is zero).
func (s SelfLearnSkills) ResolvedEveryToolIters() int {
	if s.EveryToolIters <= 0 {
		return SelfLearnDefaultEveryToolIters
	}
	return s.EveryToolIters
}

// Validate enforces the cross-field invariants of §3.1: two illegal skill
// boolean combinations are rejected, and memory.maxKB must lie in [0, 25].
// Returns nil when the configuration is acceptable (including the all-zero
// "feature off" case).
//
// denyDelete is intentionally NOT constrained: "let the reviewer create and
// refine its own skills but never auto-delete them" is a legitimate
// conservative config. Delete is already restricted to agent-created skills,
// so opting out of it removes no safety.
func (s SelfLearnSettings) Validate() error {
	if s.Memory.MaxKB < 0 || s.Memory.MaxKB > SelfLearnMaxMemoryKB {
		return fmt.Errorf(
			"memory size must be between 1 and %d KB (got %d)",
			SelfLearnMaxMemoryKB, s.Memory.MaxKB,
		)
	}
	create := s.Skills.AllowCreate()
	update := s.Skills.AllowUpdate()
	if create && !update {
		return fmt.Errorf(
			"\"Create new skills\" needs \"Update existing skills\" — otherwise created skills could never be refined",
		)
	}
	if s.Skills.AllowUpdateUserCreated && !update {
		return fmt.Errorf(
			"\"Update user-authored skills\" needs \"Update existing skills\" — the base update permission",
		)
	}
	return nil
}

// ShowContextBar reports whether the visual context-usage bar is enabled.
// Nil (unset) resolves to off — the bar is opt-in.
func (s *Data) ShowContextBar() bool {
	return s != nil && s.ContextBar != nil && *s.ContextBar
}

// PermissionSettings is the persisted permission rule set. It is aliased here
// so the settings JSON contract remains stable while policy lives in
// internal/permission.
type PermissionSettings = permissionpolicy.PermissionSettings

// Hook defines an event hook configuration.
type Hook struct {
	Matcher string    `json:"matcher,omitempty"`
	Hooks   []HookCmd `json:"hooks,omitempty"`
}

type HookCmd struct {
	Type           string            `json:"type"`
	Command        string            `json:"command,omitempty"`
	Prompt         string            `json:"prompt,omitempty"`
	URL            string            `json:"url,omitempty"`
	If             string            `json:"if,omitempty"`
	Shell          string            `json:"shell,omitempty"`
	Model          string            `json:"model,omitempty"`
	Interactive    bool              `json:"interactive,omitempty"` // keep stdin open for line-based prompt/response hooks
	Async          bool              `json:"async,omitempty"`
	AsyncRewake    bool              `json:"asyncRewake,omitempty"`
	Timeout        int               `json:"timeout,omitempty"`
	StatusMessage  string            `json:"statusMessage,omitempty"`
	Once           bool              `json:"once,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	AllowedEnvVars []string          `json:"allowedEnvVars,omitempty"`
}

type SessionPermissions = permissionpolicy.SessionPermissions
type DenialTracking = permissionpolicy.DenialTracking

func NewSessionPermissions() *SessionPermissions {
	return permissionpolicy.NewSessionPermissions()
}

type OperationMode = permissionpolicy.OperationMode

const (
	ModeNormal            = permissionpolicy.ModeNormal
	ModeAutoAccept        = permissionpolicy.ModeAutoAccept
	ModeBypassPermissions = permissionpolicy.ModeBypassPermissions
	ModeDontAsk           = permissionpolicy.ModeDontAsk
	ModeReadOnly          = permissionpolicy.ModeReadOnly
)

func OperationModeFromString(mode string) OperationMode {
	return permissionpolicy.OperationModeFromString(mode)
}

func NewData() *Data {
	return &Data{
		Hooks:          make(map[string][]Hook),
		Env:            make(map[string]string),
		EnabledPlugins: make(map[string]bool),
		DisabledTools:  make(map[string]bool),
	}
}

// InitForApp loads settings for cwd, deep-clones them, and returns
// an isolated copy safe for mutation by the app layer.
// It also merges external provider preferences (e.g., search provider
// from providers.json) into the unified Data struct.
func InitForApp(cwd string) *Data {
	var (
		settings *Data
		err      error
	)
	if cwd != "" {
		settings, err = LoadForCwd(cwd)
	} else {
		settings, err = Load()
	}
	_ = err
	if settings == nil {
		settings = defaultData()
	}
	mergeProviderPreferences(settings)
	return settings.Clone()
}

// mergeProviderPreferences reads external provider config files and merges
// relevant preferences into Data. Currently reads searchProvider from
// ~/.san/providers.json (owned by the llm package) so that search config
// is accessible via the unified Data struct.
func mergeProviderPreferences(s *Data) {
	if s.SearchProvider != "" {
		return
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(confdir.Dir(homeDir), "providers.json"))
	if err != nil {
		return
	}
	var raw struct {
		SearchProvider *string `json:"searchProvider"`
	}
	if json.Unmarshal(data, &raw) == nil && raw.SearchProvider != nil {
		s.SearchProvider = *raw.SearchProvider
	}
}

// Clone returns a deep copy of the Data.
func (s *Data) Clone() *Data {
	if s == nil {
		return defaultData()
	}
	dst := NewData()
	dst.Permissions.DefaultMode = s.Permissions.DefaultMode
	dst.Permissions.Allow = append([]string(nil), s.Permissions.Allow...)
	dst.Permissions.Deny = append([]string(nil), s.Permissions.Deny...)
	dst.Permissions.Ask = append([]string(nil), s.Permissions.Ask...)
	dst.Model = s.Model
	dst.Theme = s.Theme
	dst.SearchProvider = s.SearchProvider
	dst.Persona = s.Persona
	dst.SelfLearn = s.SelfLearn // value-typed; shallow copy is correct
	if s.AllowBypass != nil {
		v := *s.AllowBypass
		dst.AllowBypass = &v
	}
	if s.ContextBar != nil {
		v := *s.ContextBar
		dst.ContextBar = &v
	}
	for k, v := range s.Env {
		dst.Env[k] = v
	}
	for k, v := range s.EnabledPlugins {
		dst.EnabledPlugins[k] = v
	}
	for k, v := range s.DisabledTools {
		dst.DisabledTools[k] = v
	}
	for event, hooks := range s.Hooks {
		clonedHooks := make([]Hook, len(hooks))
		for i, hook := range hooks {
			clonedHooks[i].Matcher = hook.Matcher
			clonedHooks[i].Hooks = make([]HookCmd, len(hook.Hooks))
			for j, cmd := range hook.Hooks {
				clonedHooks[i].Hooks[j] = HookCmd{
					Type:           cmd.Type,
					Command:        cmd.Command,
					Prompt:         cmd.Prompt,
					URL:            cmd.URL,
					If:             cmd.If,
					Shell:          cmd.Shell,
					Model:          cmd.Model,
					Async:          cmd.Async,
					AsyncRewake:    cmd.AsyncRewake,
					Timeout:        cmd.Timeout,
					StatusMessage:  cmd.StatusMessage,
					Once:           cmd.Once,
					Headers:        maps.Clone(cmd.Headers),
					AllowedEnvVars: append([]string(nil), cmd.AllowedEnvVars...),
				}
			}
		}
		dst.Hooks[event] = clonedHooks
	}
	return dst
}
