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
