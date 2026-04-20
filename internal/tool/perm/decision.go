package perm

import (
	"context"
	"fmt"
)

// Decision represents a permission decision.
type Decision int

const (
	Permit Decision = iota
	Reject
	Prompt
)

// Checker decides whether a tool call is permitted.
type Checker interface {
	Check(name string, params map[string]any) Decision
}

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
	if IsReadOnlyTool(name) || isEditTool(name) {
		return Permit
	}
	return Prompt
}

// AcceptEdits returns a Checker that auto-approves reads and edits but prompts for others.
func AcceptEdits() Checker { return acceptEdits{} }

// BypassPermissions returns a Checker that permits everything.
// Bypass-immune checks (sensitive paths, destructive commands) are handled upstream.
func BypassPermissions() Checker { return permitAll{} }

func isEditTool(name string) bool {
	switch name {
	case "Edit", "Write", "NotebookEdit":
		return true
	}
	return false
}

// --- Tool classification ---

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

var safeTools = func() map[string]bool {
	m := map[string]bool{
		"TaskCreate":      true,
		"TaskGet":         true,
		"TaskList":        true,
		"TaskUpdate":      true,
		"AskUserQuestion": true,
		"CronList":        true,
		"ToolSearch":      true,
	}
	for name := range readOnlyTools {
		m[name] = true
	}
	return m
}()

// IsSafeTool returns true if the tool is on the safe allowlist.
// Safe tools bypass permission checks entirely.
func IsSafeTool(name string) bool {
	return safeTools[name]
}

// PermissionFunc gates tool execution.
// Called with tool name and parsed input. May block (e.g., TUI approval).
type PermissionFunc func(ctx context.Context, name string, input map[string]any) (allow bool, reason string)

// AsPermissionFunc converts a Checker to a PermissionFunc.
// Reject → (false, reason), Permit/Prompt → (true, "").
func AsPermissionFunc(c Checker) PermissionFunc {
	if c == nil {
		return nil
	}
	return func(_ context.Context, name string, input map[string]any) (bool, string) {
		if c.Check(name, input) == Reject {
			return false, fmt.Sprintf("tool %s is not permitted in this mode", name)
		}
		return true, ""
	}
}
