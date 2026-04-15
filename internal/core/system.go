package core

// Source describes where a system prompt layer originated.
type Source string

const (
	Predefined Source = "predefined" // embedded templates
	FromFile   Source = "file"       // YAML/markdown (GEN.md, skills, agents)
	Injected   Source = "injected"   // passed by parent agent or caller
	Dynamic    Source = "dynamic"    // added during conversation (hooks, compaction)
)

// Layer is one composable piece of the system prompt.
//
// Priority determines render order (lower = first):
//
//	0-99:   Identity        (base template, provider-specific)
//	100-199: Environment    (cwd, git, platform, model)
//	200-299: Instructions   (user-level, project-level)
//	300-399: Memory         (session summary, compaction)
//	400-499: Capabilities   (skills, agents, deferred tools)
//	500-599: Guidelines     (tool usage, git workflow)
//	600-699: Mode           (plan mode, worktree mode)
//	700+:    Extra          (agent identity, skill invocation)
type Layer struct {
	Name     string // unique key (e.g. "identity", "user-instructions")
	Priority int    // render order
	Content  string // prompt text
	Source   Source // origin
}

// System manages the composable, mutable system prompt.
//
// The system prompt defines WHO the agent is, WHAT it knows, HOW it behaves.
// It's a layered structure that evolves during conversation:
//   - Predefined: embedded templates at init
//   - FromFile: parsed from YAML/markdown (GEN.md, skill definitions)
//   - Injected: passed from parent agent
//   - Dynamic: adjusted mid-conversation (hooks inject context, compaction replaces summary)
type System interface {
	// Prompt returns the assembled system prompt (cached, rebuilds on change).
	Prompt() string

	// Layer management
	Set(layer Layer)
	Get(name string) (Layer, bool)
	Remove(name string)
	Layers() []Layer

	// Invalidate forces rebuild on next Prompt() call.
	Invalidate()
}
