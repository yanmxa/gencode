// Package permission owns permission modes, runtime grants, rule matching, and
// bypass-immune safety policy. Configuration packages only serialize its
// PermissionSettings value and adapt their snapshots into a Policy.
package permission

import "strings"

type PermissionSettings struct {
	DefaultMode string   `json:"defaultMode,omitempty"`
	Allow       []string `json:"allow,omitempty"`
	Deny        []string `json:"deny,omitempty"`
	Ask         []string `json:"ask,omitempty"`
}

type SessionPermissions struct {
	Mode            OperationMode
	AllowAllEdits   bool
	AllowAllWrites  bool
	AllowAllBash    bool
	AllowAllSkills  bool
	AllowAllTasks   bool
	AllowedTools    map[string]bool
	AllowedPatterns map[string]bool
	Denials         DenialTracking

	WorkingDirectories []string
	ShouldAvoidPrompts bool
}

func NewSessionPermissions() *SessionPermissions {
	return &SessionPermissions{
		AllowedTools:    make(map[string]bool),
		AllowedPatterns: make(map[string]bool),
	}
}

func (sp *SessionPermissions) AllowTool(toolName string) {
	if sp.AllowedTools == nil {
		sp.AllowedTools = make(map[string]bool)
	}
	sp.AllowedTools[toolName] = true
}

func (sp *SessionPermissions) AllowPattern(pattern string) {
	if sp.AllowedPatterns == nil {
		sp.AllowedPatterns = make(map[string]bool)
	}
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

func (sp *SessionPermissions) AddWorkingDirectory(dir string) {
	for _, existing := range sp.WorkingDirectories {
		if existing == dir {
			return
		}
	}
	sp.WorkingDirectories = append(sp.WorkingDirectories, dir)
}

type OperationMode int

const (
	ModeNormal OperationMode = iota
	ModeAutoAccept
	ModeBypassPermissions
	ModeDontAsk
	ModeReadOnly
)

var cycleModes = []OperationMode{ModeNormal, ModeAutoAccept}
var cycleModesWithBypass = []OperationMode{ModeNormal, ModeAutoAccept, ModeBypassPermissions}

func (m OperationMode) String() string {
	switch m {
	case ModeAutoAccept:
		return "accept edits"
	case ModeBypassPermissions:
		return "bypass permissions"
	case ModeDontAsk:
		return "don't ask"
	case ModeReadOnly:
		return "read-only"
	default:
		return "normal"
	}
}

func OperationModeFromString(mode string) OperationMode {
	switch strings.TrimSpace(mode) {
	case "acceptEdits", "accept-edits", "autoAccept", "auto-accept":
		return ModeAutoAccept
	case "bypassPermissions", "bypass-permissions", "bypass":
		return ModeBypassPermissions
	case "dontAsk", "dont-ask":
		return ModeDontAsk
	default:
		return ModeNormal
	}
}

func (m OperationMode) Next() OperationMode {
	for i, mode := range cycleModes {
		if mode == m {
			return cycleModes[(i+1)%len(cycleModes)]
		}
	}
	return ModeNormal
}

func (m OperationMode) NextWithBypass(enabled bool) OperationMode {
	modes := cycleModes
	if enabled {
		modes = cycleModesWithBypass
	}
	for i, mode := range modes {
		if mode == m {
			return modes[(i+1)%len(modes)]
		}
	}
	return ModeNormal
}
