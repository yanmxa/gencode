// Package loop provides a reusable agent loop that manages conversation state
// and orchestrates LLM interactions. It serves as the runtime for all agent types:
// subagents, the TUI, and custom agents.
package runtime

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
)

const (
	defaultMaxTurns          = 100
	minMessagesForCompaction = 3

	// DefaultMaxOutputRecovery is the default number of retries when LLM output
	// is truncated due to max_tokens. Exported so the TUI layer can reuse it.
	DefaultMaxOutputRecovery = 3
)

// MaxOutputRecoveryPrompt is the message injected when the LLM output is truncated.
const MaxOutputRecoveryPrompt = core.TruncatedResumePrompt

// AutoCompactResumePrompt is the user message injected after an auto-compaction
// when the caller should continue the task immediately.
const AutoCompactResumePrompt = "Continue with the task. The conversation was auto-compacted to free up context."

// Stop reason constants returned in Result.StopReason.
const (
	StopEndTurn                    = string(core.StopEndTurn)
	StopMaxTurns                   = string(core.StopMaxTurns)
	StopCancelled                  = string(core.StopCancelled)
	StopHook                       = string(core.StopHook)
	StopMaxOutputRecoveryExhausted = string(core.StopMaxOutputRecoveryExhausted)
)

// transitionReason describes why the loop moved to its current state.
type transitionReason string

const (
	transitionNextTurn          transitionReason = "next_turn"
	transitionMaxOutputRecovery transitionReason = "max_output_tokens_recovery"
	transitionPromptTooLong     transitionReason = "prompt_too_long_compact"
	transitionStopHookBlocking  transitionReason = "stop_hook_blocking"
)

// loopState holds the explicit state of the agent loop per iteration.
type loopState struct {
	turnCount                    int
	maxOutputTokensRecoveryCount int
	transition                   transitionReason
}

// CompletionAction describes what the runtime should do after receiving a completed response.
type CompletionAction int

const (
	CompletionEndTurn CompletionAction = iota
	CompletionRunTools
	CompletionRecoverMaxTokens
	CompletionStopMaxOutputRecovery
)

// CompletionDecision is the shared decision result used by both the synchronous
// core loop and the TUI-driven incremental loop.
type CompletionDecision struct {
	Action    CompletionAction
	ToolCalls []core.ToolCall
}

// RunOptions controls the synchronous Run() loop.
type RunOptions struct {
	MaxTurns            int
	MaxOutputRecovery   int // max retries on truncated output (default: 3)
	OnResponse          func(resp *core.CompletionResponse)
	OnToolStart         func(tc core.ToolCall) bool
	OnToolDone          func(tc core.ToolCall, result core.ToolResult)
	DrainInjectedInputs func() []string

	// Context compaction support (opt-in)
	SessionMemory string // previous compaction summary
	CompactFocus  string // optional focus for compaction
	InputLimit    int    // context window input limit (enables pre-stream check & reactive compact)
}

// Result is returned by Loop.Run() upon completion.
type Result struct {
	Content     string
	Messages    []core.Message
	Turns       int
	ToolUses    int
	Tokens      llm.TokenUsage
	StopReason  string
	transitions []transitionReason
	StopDetail  string
}

// --- Loop ---

// MCPCaller can execute MCP tool calls. This interface decouples runtime.Loop
// from the mcp package to avoid import cycles.
type MCPCaller interface {
	// CallTool calls an MCP tool by its full name (mcp__server__tool).
	CallTool(ctx context.Context, fullName string, arguments map[string]any) (content string, isError bool, err error)
	// IsMCPTool returns true if the name is an MCP tool (mcp__*__*).
	IsMCPTool(name string) bool
}

// LoopConfig holds the configuration for creating a Loop.
// System, Client, and Tool are required; all other fields are optional.
type LoopConfig struct {
	System          core.System          // required: system prompt provider
	Client          *llm.Client       // required: LLM client
	Tool            *tool.Set            // required: tool registry
	Permission      permission.Checker   // optional: permission checker (nil = permit all)
	Hooks           *hook.Engine        // optional: hook engine
	MCP             MCPCaller            // optional: routes mcp__*__* tool calls
	QuestionHandler tool.AskQuestionFunc // optional: interactive question handler
	Cwd             string               // required: working directory for tool execution
}

// NewLoop creates a Loop from config, validating required fields.
func NewLoop(cfg LoopConfig) (*Loop, error) {
	if cfg.System == nil {
		return nil, fmt.Errorf("runtime.NewLoop: System is required")
	}
	if cfg.Client == nil {
		return nil, fmt.Errorf("runtime.NewLoop: Client is required")
	}
	if cfg.Tool == nil {
		return nil, fmt.Errorf("runtime.NewLoop: Tool is required")
	}
	return &Loop{
		System:          cfg.System,
		Client:          cfg.Client,
		Tool:            cfg.Tool,
		Permission:      cfg.Permission,
		Hooks:           cfg.Hooks,
		MCP:             cfg.MCP,
		questionHandler: cfg.QuestionHandler,
		Cwd:             cfg.Cwd,
	}, nil
}

// Loop is a reusable agent runtime that manages conversation state
// and orchestrates LLM interactions. It supports two execution models:
//
//	Synchronous: loop.Run(ctx, opts) — drives the full turn loop
//	Incremental: loop.Stream()/Collect()/AddResponse()/FilterToolCalls()/ExecTool() — for event-driven callers
type Loop struct {
	System     core.System
	Client     *llm.Client
	Tool       *tool.Set
	Permission permission.Checker
	Hooks      *hook.Engine
	MCP        MCPCaller // optional: routes mcp__*__* tool calls
	Cwd        string    // working directory for tool execution

	questionHandler tool.AskQuestionFunc

	// Agent context: when set, tool hook events include agent_id/agent_type (subagent mode)
	agentID   string
	agentType string

	// State (managed by the loop)
	messages []core.Message
}

// SetAgentContext sets the agent identity used in hook events (subagent mode).
func (l *Loop) SetAgentContext(agentID, agentType string) {
	l.agentID = agentID
	l.agentType = agentType
}

// SetQuestionHandler sets the interactive question handler for tools that
// require user interaction (e.g., AskUserQuestion).
func (l *Loop) SetQuestionHandler(handler tool.AskQuestionFunc) {
	l.questionHandler = handler
}
