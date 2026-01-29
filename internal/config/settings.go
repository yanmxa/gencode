// Package config provides multi-level settings management for GenCode.
// Settings are loaded from multiple sources with the following priority (lowest to highest):
//   1. ~/.claude/settings.json (Claude user level - compatibility)
//   2. ~/.gen/settings.json (Gen user level)
//   3. .claude/settings.json (Claude project level - compatibility)
//   4. .gen/settings.json (Gen project level)
//   5. .claude/settings.local.json (Claude local level - compatibility)
//   6. .gen/settings.local.json (Gen local level)
//   7. Environment variables / CLI arguments
//   8. managed-settings.json (system level - cannot be overridden)
package config

// Settings represents the complete GenCode configuration.
// Compatible with Claude Code settings format.
type Settings struct {
	// Permissions defines permission rules for tools
	Permissions PermissionSettings `json:"permissions,omitempty"`

	// Model is the default model to use (e.g., "claude-sonnet-4-20250514")
	Model string `json:"model,omitempty"`

	// Hooks defines event hooks (e.g., PreToolUse, PostToolUse)
	Hooks map[string][]Hook `json:"hooks,omitempty"`

	// Env defines environment variables to set
	Env map[string]string `json:"env,omitempty"`

	// EnabledPlugins defines which plugins are enabled
	EnabledPlugins map[string]bool `json:"enabledPlugins,omitempty"`

	// DisabledTools defines which tools are disabled
	// Key is tool name, value true means disabled
	// Project-level settings can override user-level by setting to false
	DisabledTools map[string]bool `json:"disabledTools,omitempty"`
}

// PermissionSettings defines permission rules for tool execution.
// Rules use the format "Tool(pattern)" where pattern uses glob-like syntax.
//
// Example rules:
//   - "Bash(npm:*)" - Match npm commands
//   - "Read(**/.env)" - Match .env files in any directory
//   - "Edit(/path/**)" - Match files under /path
//   - "WebFetch(domain:github.com)" - Match specific domain
type PermissionSettings struct {
	// Allow contains patterns that are automatically allowed
	Allow []string `json:"allow,omitempty"`

	// Deny contains patterns that are automatically denied
	Deny []string `json:"deny,omitempty"`

	// Ask contains patterns that require user confirmation
	Ask []string `json:"ask,omitempty"`
}

// Hook defines an event hook configuration
type Hook struct {
	// Matcher is a pattern to match against the event
	Matcher string `json:"matcher,omitempty"`

	// Hooks are the commands to execute when matched
	Hooks []HookCmd `json:"hooks,omitempty"`
}

// HookCmd defines a single hook command
type HookCmd struct {
	// Type is the hook type (e.g., "command")
	Type string `json:"type"`

	// Command is the shell command to execute
	Command string `json:"command"`
}

// SessionPermissions tracks runtime permission state for the current session.
// This allows "allow all" type responses to persist during a session.
type SessionPermissions struct {
	// AllowAllEdits allows all edit operations without prompting
	AllowAllEdits bool

	// AllowAllWrites allows all write operations without prompting
	AllowAllWrites bool

	// AllowAllBash allows all bash commands without prompting
	AllowAllBash bool

	// AllowedTools contains tools that have been allowed for this session
	AllowedTools map[string]bool

	// AllowedPatterns contains specific patterns that have been allowed
	AllowedPatterns map[string]bool
}

// NewSessionPermissions creates a new session permissions instance
func NewSessionPermissions() *SessionPermissions {
	return &SessionPermissions{
		AllowedTools:    make(map[string]bool),
		AllowedPatterns: make(map[string]bool),
	}
}

// AllowTool marks a tool as allowed for this session
func (sp *SessionPermissions) AllowTool(toolName string) {
	sp.AllowedTools[toolName] = true
}

// AllowPattern marks a specific pattern as allowed for this session
func (sp *SessionPermissions) AllowPattern(pattern string) {
	sp.AllowedPatterns[pattern] = true
}

// IsToolAllowed checks if a tool is allowed for this session
func (sp *SessionPermissions) IsToolAllowed(toolName string) bool {
	if sp.AllowedTools[toolName] {
		return true
	}
	if toolName == "Edit" && sp.AllowAllEdits {
		return true
	}
	if toolName == "Write" && sp.AllowAllWrites {
		return true
	}
	if toolName == "Bash" && sp.AllowAllBash {
		return true
	}
	return false
}

// IsPatternAllowed checks if a specific pattern is allowed for this session
func (sp *SessionPermissions) IsPatternAllowed(pattern string) bool {
	return sp.AllowedPatterns[pattern]
}

// OperationMode defines the current operation mode
type OperationMode int

const (
	ModeNormal     OperationMode = iota // Normal: permission prompts enabled
	ModeAutoAccept                      // Auto-accept: auto-approve edits/writes
	ModePlan                            // Plan: read-only tools only
)

// String returns the display name for the mode
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

// Next returns the next mode in the cycle
func (m OperationMode) Next() OperationMode {
	return (m + 1) % 3
}

// NewSettings creates a new Settings instance with default values
func NewSettings() *Settings {
	return &Settings{
		Permissions: PermissionSettings{
			Allow: []string{},
			Deny:  []string{},
			Ask:   []string{},
		},
		Hooks:          make(map[string][]Hook),
		Env:            make(map[string]string),
		EnabledPlugins: make(map[string]bool),
		DisabledTools:  make(map[string]bool),
	}
}
