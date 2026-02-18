// Package agent provides agent (subagent) execution for GenCode.
// Agents are specialized LLM instances that can be spawned to handle
// specific tasks with isolated contexts and tool restrictions.
package agent

import (
	"time"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/message"
)

// PermissionMode controls how the agent handles permission requests
type PermissionMode string

const (
	// PermissionDefault uses normal permission flow
	PermissionDefault PermissionMode = "default"
	// PermissionAcceptEdits auto-accepts file edits but asks for commands
	PermissionAcceptEdits PermissionMode = "acceptEdits"
	// PermissionDontAsk auto-accepts all operations
	PermissionDontAsk PermissionMode = "dontAsk"
	// PermissionPlan is read-only mode (plan mode)
	PermissionPlan PermissionMode = "plan"
)

// ToolAccessMode controls how tool access is configured
type ToolAccessMode string

const (
	// ToolAccessAllowlist only allows specified tools
	ToolAccessAllowlist ToolAccessMode = "allowlist"
	// ToolAccessDenylist allows all except specified tools
	ToolAccessDenylist ToolAccessMode = "denylist"
)

// ToolAccess configures which tools are available to the agent
type ToolAccess struct {
	Mode  ToolAccessMode `yaml:"mode" json:"mode"`
	Allow []string       `yaml:"allow,omitempty" json:"allow,omitempty"`
	Deny  []string       `yaml:"deny,omitempty" json:"deny,omitempty"`
}

// AgentConfig defines the configuration for an agent type
type AgentConfig struct {
	// Name is the unique identifier for this agent type
	Name string `yaml:"name" json:"name"`

	// Description describes what this agent does (used for LLM decision making)
	Description string `yaml:"description" json:"description"`

	// Model specifies the model to use (inherit, sonnet, opus, haiku)
	Model string `yaml:"model" json:"model"`

	// PermissionMode controls how permissions are handled
	PermissionMode PermissionMode `yaml:"permission-mode" json:"permission_mode"`

	// Tools configures available tools
	Tools ToolAccess `yaml:"tools" json:"tools"`

	// Skills lists skills to preload
	Skills []string `yaml:"skills,omitempty" json:"skills,omitempty"`

	// SystemPrompt provides additional system prompt content
	SystemPrompt string `yaml:"system-prompt,omitempty" json:"system_prompt,omitempty"`

	// MaxTurns limits the number of conversation turns
	MaxTurns int `yaml:"max-turns" json:"max_turns"`

	// Background indicates whether this agent runs in background by default
	Background bool `yaml:"background" json:"background"`

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
	// Lazy load from file
	c.loadSystemPromptFromFile()
	return c.SystemPrompt
}

// loadSystemPromptFromFile loads the full system prompt from the source file.
// This is called lazily when the agent is actually executed.
func (c *AgentConfig) loadSystemPromptFromFile() {
	if c.SourceFile == "" || c.systemPromptLoaded {
		return
	}
	c.systemPromptLoaded = true

	// Use the loader's function to extract the body
	prompt := LoadAgentSystemPrompt(c.SourceFile)
	if prompt != "" {
		c.SystemPrompt = prompt
	}
}

// ProgressCallback is called when the agent makes progress
type ProgressCallback func(msg string)

// AgentRequest represents a request to spawn an agent
type AgentRequest struct {
	// Agent is the name of the agent type to use
	Agent string

	// Prompt is the task for the agent to perform
	Prompt string

	// Description is a short (3-5 word) description of the task
	Description string

	// Background if true, runs the agent in background
	Background bool

	// ResumeID is the ID of a previous agent to resume
	ResumeID string

	// Model overrides the agent's default model
	Model string

	// MaxTurns overrides the agent's max turns
	MaxTurns int

	// ParentMessages is the conversation history from the parent (for context)
	ParentMessages []message.Message

	// Cwd is the current working directory
	Cwd string

	// OnProgress is called when the agent makes progress (tool execution, etc.)
	OnProgress ProgressCallback
}

// AgentResult contains the result of an agent execution
type AgentResult struct {
	// AgentID is the unique identifier for this agent instance
	AgentID string

	// AgentName is the name of the agent type used
	AgentName string

	// Success indicates whether the agent completed successfully
	Success bool

	// Content is the final response content from the agent
	Content string

	// Summary is a brief summary of what was accomplished
	Summary string

	// Messages contains the full conversation history
	Messages []message.Message

	// TurnCount is the number of turns used
	TurnCount int

	// TokenUsage is the total tokens consumed
	TokenUsage client.TokenUsage

	// Duration is the total execution time
	Duration time.Duration

	// Error contains any error message if not successful
	Error string
}

// DefaultMaxTurns is the default maximum number of conversation turns
const DefaultMaxTurns = 100

// FallbackModel is used when no parent model is available
const FallbackModel = "claude-sonnet-4-20250514"
