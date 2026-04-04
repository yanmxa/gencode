// Package permission provides tool execution permission checking.
package permission

// Checker decides whether a tool call is permitted.
type Checker interface {
	Check(name string, params map[string]any) Decision
}

// Decision represents a permission decision.
type Decision int

const (
	// Permit auto-executes the tool call.
	Permit Decision = iota
	// Reject blocks the tool call.
	Reject
	// Prompt delegates to the caller for interactive approval.
	Prompt
	// Defer means the checker has no opinion — defer to the next layer.
	// This maps to Claude Code's "passthrough" behavior.
	Defer Decision = 99
)

// --- Convenience constructors ---

type permitAll struct{}

func (permitAll) Check(_ string, _ map[string]any) Decision { return Permit }

// PermitAll returns a Checker that always permits.
func PermitAll() Checker { return permitAll{} }

type readOnly struct{}

func (readOnly) Check(name string, _ map[string]any) Decision {
	if IsReadOnlyTool(name) {
		return Permit
	}
	return Reject
}

// ReadOnly returns a Checker that permits read-only tools and rejects others.
func ReadOnly() Checker { return readOnly{} }

type denyAll struct{}

func (denyAll) Check(_ string, _ map[string]any) Decision { return Reject }

// DenyAll returns a Checker that always rejects.
func DenyAll() Checker { return denyAll{} }

type acceptEdits struct{}

func (acceptEdits) Check(name string, _ map[string]any) Decision {
	// Auto-approve read tools and file editing tools
	if IsReadOnlyTool(name) || isEditTool(name) {
		return Permit
	}
	return Prompt
}

// AcceptEdits returns a Checker that auto-approves reads and edits but prompts for others.
func AcceptEdits() Checker { return acceptEdits{} }

// BypassPermissions returns a Checker that permits everything except bypass-immune tools.
// Bypass-immune checks (sensitive paths, destructive commands) are handled upstream
// by config.HasPermissionToUseTool, not by this Checker.
func BypassPermissions() Checker { return permitAll{} }

// DontAsk returns a Checker that converts prompts to rejections (never prompts).
func DontAsk() Checker { return dontAsk{} }

type dontAsk struct{}

func (dontAsk) Check(name string, _ map[string]any) Decision {
	if IsReadOnlyTool(name) || IsSafeTool(name) {
		return Permit
	}
	return Reject
}

// Auto returns a Checker equivalent to PermitAll (auto-determines best level).
func Auto() Checker { return permitAll{} }

// isEditTool checks if a tool is a file editing tool.
func isEditTool(name string) bool {
	switch name {
	case "Edit", "Write", "NotebookEdit":
		return true
	}
	return false
}

// readOnlyTools is the set of tools that only read data without modifications.
var readOnlyTools = map[string]bool{
	"Read":      true,
	"Glob":      true,
	"Grep":      true,
	"WebFetch":  true,
	"WebSearch": true,
	"LSP":       true,
}

// IsReadOnlyTool checks if a tool is read-only.
func IsReadOnlyTool(name string) bool {
	return readOnlyTools[name]
}

// safeTools is the set of tools that are inherently safe and can skip
// permission checks entirely (task management, UI, coordination).
var safeTools = map[string]bool{
	"TaskCreate":      true,
	"TaskGet":         true,
	"TaskList":        true,
	"TaskUpdate":      true,
	"AskUserQuestion": true,
	"EnterPlanMode":   true,
	"ExitPlanMode":    true,
	"TeamCreate":      true,
	"TeamDelete":      true,
	"CronList":        true,
	"ToolSearch":      true,
}

// IsSafeTool returns true if the tool is on the safe allowlist.
func IsSafeTool(name string) bool {
	return safeTools[name]
}
