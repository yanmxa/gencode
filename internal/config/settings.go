// Package config provides multi-level settings management for GenCode.
// Settings are loaded from multiple sources with the following priority (lowest to highest):
//  1. ~/.claude/settings.json (Claude user level - compatibility)
//  2. ~/.gen/settings.json (Gen user level)
//  3. .claude/settings.json (Claude project level - compatibility)
//  4. .gen/settings.json (Gen project level)
//  5. .claude/settings.local.json (Claude local level - compatibility)
//  6. .gen/settings.local.json (Gen local level)
//  7. Environment variables / CLI arguments
//  8. managed-settings.json (system level - cannot be overridden)
package config

// Settings represents the complete GenCode configuration.
type Settings struct {
	Permissions    PermissionSettings `json:"permissions,omitempty"`
	Model          string             `json:"model,omitempty"`
	Hooks          map[string][]Hook  `json:"hooks,omitempty"`
	Env            map[string]string  `json:"env,omitempty"`
	EnabledPlugins map[string]bool    `json:"enabledPlugins,omitempty"`
	DisabledTools  map[string]bool    `json:"disabledTools,omitempty"`
	Theme          string             `json:"theme,omitempty"`
}

// PermissionSettings defines permission rules for tool execution.
// Rule format: "Tool(pattern)" — e.g. "Bash(npm:*)", "Read(**/.env)".
type PermissionSettings struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
	Ask   []string `json:"ask,omitempty"`
}

// Hook defines an event hook configuration.
type Hook struct {
	Matcher string    `json:"matcher,omitempty"`
	Hooks   []HookCmd `json:"hooks,omitempty"`
}

type HookCmd struct {
	Type          string `json:"type"`
	Command       string `json:"command,omitempty"`
	Prompt        string `json:"prompt,omitempty"`
	Model         string `json:"model,omitempty"`
	Async         bool   `json:"async,omitempty"`
	Timeout       int    `json:"timeout,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
	Once          bool   `json:"once,omitempty"`
}

// SessionPermissions tracks runtime permission state for the current session.
type SessionPermissions struct {
	AllowAllEdits   bool
	AllowAllWrites  bool
	AllowAllBash    bool
	AllowAllSkills  bool
	AllowAllTasks   bool
	AllowedTools    map[string]bool
	AllowedPatterns map[string]bool
}

func NewSessionPermissions() *SessionPermissions {
	return &SessionPermissions{
		AllowedTools:    make(map[string]bool),
		AllowedPatterns: make(map[string]bool),
	}
}

func (sp *SessionPermissions) AllowTool(toolName string) {
	sp.AllowedTools[toolName] = true
}

func (sp *SessionPermissions) AllowPattern(pattern string) {
	sp.AllowedPatterns[pattern] = true
}

func (sp *SessionPermissions) IsToolAllowed(toolName string) bool {
	if sp.AllowedTools[toolName] {
		return true
	}
	switch toolName {
	case "Edit":
		return sp.AllowAllEdits
	case "Write":
		return sp.AllowAllWrites
	case "Bash":
		return sp.AllowAllBash
	case "Skill":
		return sp.AllowAllSkills
	case "Agent":
		return sp.AllowAllTasks
	}
	return false
}

func (sp *SessionPermissions) IsPatternAllowed(pattern string) bool {
	return sp.AllowedPatterns[pattern]
}

// OperationMode defines the current operation mode.
type OperationMode int

const (
	ModeNormal     OperationMode = iota
	ModeAutoAccept               // auto-approve edits/writes
	ModePlan                     // read-only tools only
)

func (m OperationMode) String() string {
	switch m {
	case ModeAutoAccept:
		return "accept edits"
	case ModePlan:
		return "plan mode"
	default:
		return "normal"
	}
}

func (m OperationMode) Next() OperationMode {
	return (m + 1) % 3
}

func NewSettings() *Settings {
	return &Settings{
		Hooks:          make(map[string][]Hook),
		Env:            make(map[string]string),
		EnabledPlugins: make(map[string]bool),
		DisabledTools:  make(map[string]bool),
	}
}
