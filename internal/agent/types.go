// Package agent provides agent (subagent) execution for GenCode.
// Agents are specialized LLM instances that can be spawned to handle
// specific tasks with isolated contexts and tool restrictions.
package agent

import (
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
	"gopkg.in/yaml.v3"
)

// PermissionMode controls how the agent handles permission requests
type PermissionMode string

const (
	// PermissionDefault uses normal permission flow
	PermissionDefault PermissionMode = "default"
	// PermissionAcceptEdits auto-accepts file edits but prompts for other operations
	PermissionAcceptEdits PermissionMode = "acceptEdits"
	// PermissionDontAsk auto-accepts all operations
	PermissionDontAsk PermissionMode = "dontAsk"
	// PermissionPlan is read-only mode (plan mode)
	PermissionPlan PermissionMode = "plan"
	// PermissionBypassPermissions auto-approves everything without asking
	PermissionBypassPermissions PermissionMode = "bypassPermissions"
	// PermissionAuto automatically determines the best permission level
	PermissionAuto PermissionMode = "auto"
)

// ToolList is a list of tool names with flexible YAML parsing.
// nil means "all tools". Non-nil means only these tools are allowed.
// Supports CC-compatible formats: string, array, or map.
type ToolList []string

// UnmarshalYAML handles multiple YAML formats for tool lists:
//   - string: "Read, Write, Bash" (comma-separated)
//   - array:  [Read, Write, Bash]
//   - map:    {Read: true, Write: true} (CC format)
//   - legacy: {mode: allowlist, allow: [...]} (old GenCode format)
func (t *ToolList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		for _, p := range strings.Split(value.Value, ",") {
			if s := strings.TrimSpace(p); s != "" {
				*t = append(*t, s)
			}
		}
	case yaml.SequenceNode:
		var items []string
		if err := value.Decode(&items); err != nil {
			return err
		}
		*t = items
	case yaml.MappingNode:
		var m map[string]any
		if err := value.Decode(&m); err != nil {
			return err
		}
		// Legacy GenCode format: {mode: allowlist, allow: [...]}
		if _, hasMode := m["mode"]; hasMode {
			if allow, ok := m["allow"].([]any); ok {
				for _, a := range allow {
					if s, ok := a.(string); ok {
						*t = append(*t, s)
					}
				}
			}
			return nil
		}
		// CC format: {Read: true, Write: true}
		for k, v := range m {
			if b, ok := v.(bool); ok && b {
				*t = append(*t, k)
			}
		}
	}
	return nil
}

// AgentConfig defines the configuration for an agent type
type AgentConfig struct {
	// Name is the unique identifier for this agent type
	Name string `yaml:"name" json:"name"`

	// Description describes what this agent does (used for LLM decision making)
	Description string `yaml:"description" json:"description"`

	// WhenToUse provides guidance on when to use this agent (shown in <available-agents>)
	WhenToUse string `yaml:"when-to-use,omitempty" json:"when_to_use,omitempty"`

	// Model specifies the model to use (inherit, sonnet, opus, haiku)
	Model string `yaml:"model" json:"model"`

	// PermissionMode controls how permissions are handled
	PermissionMode PermissionMode `yaml:"permission-mode" json:"permission_mode"`

	// Tools lists allowed tools. nil = all tools; non-nil = only listed tools.
	// Supports CC-compatible formats (comma string, array, map).
	Tools ToolList `yaml:"tools" json:"tools"`

	// DisallowedTools lists tools to exclude. Applied after allow list filtering.
	// Supports the same CC-compatible formats as Tools.
	DisallowedTools ToolList `yaml:"disallowed-tools,omitempty" json:"disallowed_tools,omitempty"`

	// Skills lists skills to preload
	Skills []string `yaml:"skills,omitempty" json:"skills,omitempty"`

	// SystemPrompt provides additional system prompt content
	SystemPrompt string `yaml:"system-prompt,omitempty" json:"system_prompt,omitempty"`

	// MaxTurns limits the number of conversation turns
	MaxTurns int `yaml:"max-turns" json:"max_turns"`

	// Source indicates where this agent was loaded from (built-in, user, project, plugin)
	Source string `yaml:"-" json:"source,omitempty"`

	// McpServers lists MCP servers this agent should connect to
	McpServers []string `yaml:"mcp-servers,omitempty" json:"mcp_servers,omitempty"`

	// SourceFile is the file path if loaded from AGENT.md (internal use)
	SourceFile string `yaml:"-" json:"-"`

	// systemPromptLoaded indicates if the full system prompt has been loaded
	systemPromptLoaded bool `yaml:"-" json:"-"`
}

// GetSystemPrompt returns the system prompt, loading it lazily if needed.
// For file-based agents, the prompt is loaded from SourceFile on first access.
func (c *AgentConfig) GetSystemPrompt() string {
	if c.systemPromptLoaded || c.SourceFile == "" {
		return c.SystemPrompt
	}
	c.loadSystemPromptFromFile()
	return c.SystemPrompt
}

// loadSystemPromptFromFile loads the full system prompt from the source file.
func (c *AgentConfig) loadSystemPromptFromFile() {
	if c.SourceFile == "" || c.systemPromptLoaded {
		return
	}
	c.systemPromptLoaded = true
	if prompt := LoadAgentSystemPrompt(c.SourceFile); prompt != "" {
		c.SystemPrompt = prompt
	}
}

// ProgressCallback is called when the agent makes progress
type ProgressCallback func(msg string)

// AgentRequest represents a request to spawn an agent
type AgentRequest struct {
	// Agent is the name of the agent type to use
	Agent string

	// Name is an optional display name for the agent instance
	Name string

	// Prompt is the task for the agent to perform
	Prompt string

	// Description is a short (3-5 word) description of the task
	Description string

	// Background if true, runs the agent in background
	Background bool

	// Model overrides the agent's default model
	Model string

	// MaxTurns overrides the agent's max turns
	MaxTurns int

	// Mode overrides the agent's permission mode for this invocation
	Mode string

	// ResumeID is an optional agent ID to resume from a previous invocation
	ResumeID string

	// LiveTaskID links a running background task to this request so follow-up
	// messages can be injected at safe loop boundaries while the worker is active.
	LiveTaskID string

	// Isolation specifies isolation mode (e.g., "worktree" for git worktree)
	Isolation string

	// TeamName is the team name for spawning
	TeamName string

	// ParentMessages is the conversation history from the parent (for context)
	ParentMessages []message.Message

	// OnProgress is called when the agent makes progress (tool execution, etc.)
	OnProgress ProgressCallback

	// OnQuestion is called when the agent needs an interactive question answered.
	OnQuestion tool.AskQuestionFunc
}

// AgentResult contains the result of an agent execution
type AgentResult struct {
	// AgentID is the unique identifier for this agent instance
	AgentID string

	// AgentName is the name of the agent type used
	AgentName string

	// Model is the model ID used for execution
	Model string

	// Success indicates whether the agent completed successfully
	Success bool

	// Content is the final response content from the agent
	Content string

	// Summary is a brief summary of what was accomplished
	Summary string

	// TranscriptPath points at the persisted transcript/output file when available.
	TranscriptPath string

	// Messages contains the full conversation history
	Messages []message.Message

	// TurnCount is the number of turns used
	TurnCount int

	// ToolUses is the number of tool calls executed
	ToolUses int

	// TokenUsage is the total tokens consumed
	TokenUsage client.TokenUsage

	// Duration is the total execution time
	Duration time.Duration

	// Progress contains all intermediate progress messages (tool calls made)
	Progress []string

	// Error contains any error message if not successful
	Error string
}

// DefaultMaxTurns is the default maximum number of conversation turns
const DefaultMaxTurns = 100

// modelAliases maps short model aliases to full Vertex AI model IDs.
var modelAliases = map[string]string{
	"sonnet": "claude-sonnet-4-20250514",
	"opus":   "claude-opus-4-20250514",
	"haiku":  "claude-haiku-4-5-20251001",
}

// ResolveModelAlias returns the full model ID for a known alias,
// or the input unchanged if it is not an alias.
func ResolveModelAlias(model string) string {
	if full, ok := modelAliases[model]; ok {
		return full
	}
	return model
}
