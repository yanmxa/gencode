package agent

import (
	"strings"
	"sync"
)

// Registry manages agent type definitions
type Registry struct {
	mu           sync.RWMutex
	agents       map[string]*AgentConfig
	userStore    *AgentStore // User-level enabled/disabled states
	projectStore *AgentStore // Project-level enabled/disabled states
	cwd          string      // Current working directory
}

// NewRegistry creates a new agent registry
func NewRegistry() *Registry {
	r := &Registry{
		agents: make(map[string]*AgentConfig),
	}
	// Register built-in agents
	r.registerBuiltins()
	return r
}

// DefaultRegistry is the global agent registry
var DefaultRegistry = NewRegistry()

// Register adds an agent configuration to the registry
func (r *Registry) Register(config *AgentConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[strings.ToLower(config.Name)] = config
}

// Get retrieves an agent configuration by name
func (r *Registry) Get(name string) (*AgentConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	config, ok := r.agents[strings.ToLower(name)]
	return config, ok
}

// List returns all registered agent names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	return names
}

// ListConfigs returns all registered agent configurations
func (r *Registry) ListConfigs() []*AgentConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	configs := make([]*AgentConfig, 0, len(r.agents))
	for _, config := range r.agents {
		configs = append(configs, config)
	}
	return configs
}

// registerBuiltins registers the built-in agent types
func (r *Registry) registerBuiltins() {
	// Explore agent - read-only codebase exploration
	r.agents["explore"] = &AgentConfig{
		Name:           "Explore",
		Description:    "Fast codebase exploration and understanding. Use for finding files, searching code, and answering questions about the codebase.",
		Model:          "inherit",
		PermissionMode: PermissionPlan,
		Tools: ToolAccess{
			Mode:  ToolAccessAllowlist,
			Allow: []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch"},
		},
		MaxTurns:   30,
		Background: false,
	}

	// Plan agent - implementation planning
	r.agents["plan"] = &AgentConfig{
		Name:           "Plan",
		Description:    "Software architect for designing implementation plans. Use for planning complex tasks, identifying critical files, and considering architectural trade-offs.",
		Model:          "inherit",
		PermissionMode: PermissionPlan,
		Tools: ToolAccess{
			Mode:  ToolAccessAllowlist,
			Allow: []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch"},
		},
		MaxTurns:   50,
		Background: false,
	}

	// Bash agent - command execution specialist
	r.agents["bash"] = &AgentConfig{
		Name:           "Bash",
		Description:    "Command execution specialist for running bash commands, git operations, and terminal tasks.",
		Model:          "inherit",
		PermissionMode: PermissionDefault,
		Tools: ToolAccess{
			Mode:  ToolAccessAllowlist,
			Allow: []string{"Bash", "Read", "Glob", "Grep"},
		},
		MaxTurns:   30,
		Background: false,
	}

	// Review agent - code review specialist
	r.agents["review"] = &AgentConfig{
		Name:           "Review",
		Description:    "Code review specialist for analyzing code changes, identifying issues, and suggesting improvements.",
		Model:          "inherit",
		PermissionMode: PermissionPlan,
		Tools: ToolAccess{
			Mode:  ToolAccessAllowlist,
			Allow: []string{"Read", "Glob", "Grep", "Bash"},
		},
		MaxTurns:   30,
		Background: false,
	}

	// General-purpose agent - full access
	r.agents["general-purpose"] = &AgentConfig{
		Name:           "general-purpose",
		Description:    "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks.",
		Model:          "inherit",
		PermissionMode: PermissionDefault,
		Tools: ToolAccess{
			Mode: ToolAccessDenylist,
			Deny: []string{"Task"}, // Prevent nested Task spawning
		},
		MaxTurns:   50,
		Background: false,
	}
}

// InitStores initializes the user and project stores for enabled/disabled state
func (r *Registry) InitStores(cwd string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cwd = cwd
	r.userStore = NewUserAgentStore()
	r.projectStore = NewProjectAgentStore(cwd)
	return nil
}

// IsEnabled returns whether an agent is enabled
// An agent is enabled unless explicitly disabled in either store
// Project-level settings take priority over user-level
func (r *Registry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lowerName := strings.ToLower(name)

	// Check project store first (higher priority)
	if r.projectStore != nil && r.projectStore.IsDisabled(lowerName) {
		return false
	}

	// Check user store
	if r.userStore != nil && r.userStore.IsDisabled(lowerName) {
		return false
	}

	return true
}

// SetEnabled sets the enabled state for an agent at the specified level
func (r *Registry) SetEnabled(name string, enabled bool, userLevel bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	lowerName := strings.ToLower(name)

	if userLevel {
		if r.userStore != nil {
			return r.userStore.SetDisabled(lowerName, !enabled)
		}
	} else {
		if r.projectStore != nil {
			return r.projectStore.SetDisabled(lowerName, !enabled)
		}
	}
	return nil
}

// GetDisabledAt returns the disabled agents from the specified level
func (r *Registry) GetDisabledAt(userLevel bool) map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if userLevel {
		if r.userStore != nil {
			return r.userStore.GetDisabled()
		}
	} else {
		if r.projectStore != nil {
			return r.projectStore.GetDisabled()
		}
	}
	return make(map[string]bool)
}

// isDisabledInternal checks if an agent is disabled (must be called with lock held)
func (r *Registry) isDisabledInternal(name string) bool {
	if r.projectStore != nil && r.projectStore.IsDisabled(name) {
		return true
	}
	if r.userStore != nil && r.userStore.IsDisabled(name) {
		return true
	}
	return false
}

// ListEnabled returns only enabled agent configurations
func (r *Registry) ListEnabled() []*AgentConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]*AgentConfig, 0, len(r.agents))
	for name, config := range r.agents {
		if !r.isDisabledInternal(name) {
			configs = append(configs, config)
		}
	}
	return configs
}

// GetAgentPromptForLLM returns a formatted string describing available agents.
// This is used to inform the LLM about what agents are available.
// Only includes enabled agents.
func (r *Registry) GetAgentPromptForLLM() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Available agent types:\n")

	for name, config := range r.agents {
		if r.isDisabledInternal(name) {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(config.Name)
		sb.WriteString(": ")
		sb.WriteString(config.Description)
		sb.WriteString("\n")
	}

	return sb.String()
}
