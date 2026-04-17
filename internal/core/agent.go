package core

import (
	"context"
)

// PermissionFunc is called before each tool execution.
// Returns whether the tool call is allowed, with an optional reason.
// Permission runs before PreTool hooks — see Config.Permission doc.
type PermissionFunc func(ctx context.Context, tc ToolCall) (allow bool, reason string)

// Agent is the core abstraction — an autonomous entity that reasons and acts.
//
// Four capabilities, nothing more:
//  1. System  — WHO it is (composable, mutable identity)
//  2. Tools   — WHAT it can do (the single action primitive)
//  3. Hooks   — HOW it's extended (universal extension point)
//  4. Inbox/Outbox — HOW it communicates (Go channels)
//
// Everything else is built on hooks or config:
//   - Permission  → PermissionFunc on Config (injected, runs before hooks)
//   - UX          → TUI reads Outbox for streaming chunks
//   - Persistence → OnStop hook saves session
//   - Compaction  → PreInfer hook checks tokens, compacts
//   - MCP         → OnStart hook adds tools dynamically
//   - Subagents   → Agent tool spawns new Agent with its own channels
//
// Lifecycle control:
//   - Graceful stop: send Message{Signal: SigStop} to Inbox
//   - Immediate stop: cancel the context passed to Run
type Agent interface {
	ID() string
	System() System
	Tools() Tools
	Hooks() Hooks

	// Inbox is the write channel — external world sends messages to the agent.
	// Messages are integrated into the conversation at turn boundaries.
	//
	// Ownership: caller owns the channel and must close it when done sending.
	// Sending to Inbox after Run() returns may block indefinitely.
	Inbox() chan<- Message

	// Outbox is the read channel — agent emits events to the external world.
	// Events include streaming chunks, tool execution status, and turn results.
	//
	// Ownership: agent owns the channel and closes it when Run() returns.
	// Outbox is single-consumer; for multiple consumers, build a fan-out on top.
	Outbox() <-chan Event

	// Messages returns a snapshot of the conversation history.
	// The returned slice is a shallow copy — do not mutate Message fields
	// that contain maps, slices, or pointers (Meta, ToolCalls, ToolResult).
	Messages() []Message

	// SetMessages replaces the conversation history.
	// Used by compaction (shrink context) and session restore (load saved state).
	// The provided slice is shallow-copied; same mutation caveats as Messages().
	SetMessages(msgs []Message)

	// Append adds a message to the conversation and fires the OnMessage hook.
	// This is the unified entry point for both paths:
	//   Run path:   inbox → ingest (Append internally)
	//   Direct path: caller → Append → ThinkAct
	Append(ctx context.Context, msg Message)

	// ThinkAct runs one full inference-action cycle: PreInfer → LLM stream →
	// tool execution → repeat until end_turn. Returns the result directly.
	//
	// This is the agent's atomic operation. Two callers drive it differently:
	//   Run():   loop { waitForInput → ThinkAct }, emits TurnEvent to Outbox
	//   Direct:  Append(msg) → ThinkAct(ctx), returns *Result synchronously
	ThinkAct(ctx context.Context) (*Result, error)

	// Run starts the agent's main loop. Blocks until context cancellation or SigStop.
	//
	// The run loop has three phases per cycle:
	//
	//   Phase 1 — WAIT (blocking):
	//     Block on Inbox until a message arrives. This is the idle state.
	//     On SigStop or ctx.Done(): fire OnStop hooks and return.
	//
	//   Phase 2 — DRAIN (non-blocking):
	//     Drain any additional messages that accumulated in Inbox.
	//     All drained messages are appended to the conversation.
	//
	//   Phase 3 — THINK + ACT (inference loop):
	//     Loop: LLM inference → tool execution → LLM inference → ...
	//     Between each turn, non-blocking drain of Inbox for new messages.
	//     Emit streaming chunks and tool results to Outbox.
	//     Loop until LLM returns end_turn.
	//     Then go back to Phase 1 (wait for next message).
	//
	// Signals (SigStop) are checked at every phase boundary.
	Run(ctx context.Context) error
}

// Config holds construction parameters for an agent.
//
// Required fields: LLM, System, Tools. NewAgent panics if any is nil.
// Optional fields: Hooks, Permission, ID, CWD, MaxTurns, InboxBuf, OutboxBuf.
type Config struct {
	ID                string
	LLM               LLM            // required: inference backend
	System             System         // required: system prompt layers
	Tools              Tools          // required: available tools
	Hooks              Hooks          // optional: event handlers
	Permission         PermissionFunc // optional: called before each tool execution (runs before PreTool hooks)
	AgentType          string   // optional: agent type identifier for hook events
	Color              string   // optional: display color for TUI (e.g. "#ff6600", "blue")
	AllowedTools       []string // optional: tools that skip Permission check
	CWD                string
	MaxTurns           int // max LLM inference rounds per cycle, 0 = unlimited
	MaxOutputRecovery  int // max retries on truncated output, 0 = use default (3)
	InboxBuf           int // inbox channel buffer size, default 16
	OutboxBuf          int // outbox channel buffer size, default 64
}

// NewAgent creates an agent from config.
//
// Panics if LLM, System, or Tools is nil — these are required capabilities.
// Inbox is owned by the caller (caller closes when done sending).
// Outbox is owned by the agent (closed when Run returns).
func NewAgent(cfg Config) Agent {
	if cfg.LLM == nil {
		panic("core.NewAgent: LLM is required")
	}
	if cfg.System == nil {
		panic("core.NewAgent: System is required")
	}
	if cfg.Tools == nil {
		panic("core.NewAgent: Tools is required")
	}
	if cfg.InboxBuf <= 0 {
		cfg.InboxBuf = 16
	}
	if cfg.OutboxBuf <= 0 {
		cfg.OutboxBuf = 64
	}
	var allowed map[string]bool
	if len(cfg.AllowedTools) > 0 {
		allowed = make(map[string]bool, len(cfg.AllowedTools))
		for _, name := range cfg.AllowedTools {
			allowed[name] = true
		}
	}
	return &agent{
		id:                cfg.ID,
		agentType:         cfg.AgentType,
		color:             cfg.Color,
		system:            cfg.System,
		tools:             cfg.Tools,
		hooks:             cfg.Hooks,
		permission:        cfg.Permission,
		allowedTools:      allowed,
		llm:               cfg.LLM,
		cwd:               cfg.CWD,
		maxTurns:          cfg.MaxTurns,
		maxOutputRecovery: cfg.MaxOutputRecovery,
		inbox:             make(chan Message, cfg.InboxBuf),
		outbox:            make(chan Event, cfg.OutboxBuf),
	}
}

// Result represents the outcome of one completed turn (end_turn).
// Emitted to Outbox as Event{Type: OnTurn, Data: result}.
type Result struct {
	Content    string     // final text output of this turn
	Messages   []Message  // full conversation history
	Turns      int        // LLM inference rounds in this cycle
	ToolUses   int        // tool calls in this cycle
	TokensIn   int        // input tokens consumed
	TokensOut  int        // output tokens produced
	StopReason StopReason // why the loop stopped
	StopDetail string     // human-readable detail (e.g. hook block reason)
}
