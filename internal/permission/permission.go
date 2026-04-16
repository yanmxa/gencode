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

// isEditTool checks if a tool is a file editing tool.
func isEditTool(name string) bool {
	switch name {
	case "Edit", "Write", "NotebookEdit":
		return true
	}
	return false
}

// readOnlyTools lists tools that don't modify any files or state.
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

// safeTools is the allowlist of tools that can skip permission checks entirely.
var safeTools = func() map[string]bool {
	m := map[string]bool{
		"TaskCreate":      true,
		"TaskGet":         true,
		"TaskList":        true,
		"TaskUpdate":      true,
		"AskUserQuestion": true,
		"EnterPlanMode":   true,
		"ExitPlanMode":    true,
		"CronList":        true,
		"ToolSearch":      true,
	}
	for name := range readOnlyTools {
		m[name] = true
	}
	return m
}()

// IsSafeTool returns true if the tool is on the safe allowlist.
func IsSafeTool(name string) bool {
	return safeTools[name]
}
