package core

import "context"

// EventType identifies when a hook fires.
//
// Core events are defined below. Custom events are first-class:
//
//	const MyEvent core.EventType = "MyEvent"
type EventType string

// Core agent lifecycle events.
const (
	OnStart   EventType = "AgentStart" // agent begins
	OnStop    EventType = "AgentStop"  // agent ends (error or nil in Data)
	PreInfer  EventType = "PreInfer"   // before LLM call
	PostInfer EventType = "PostInfer"  // after LLM response (*InferResponse in Data)
	OnChunk   EventType = "Chunk"      // streaming chunk (Chunk in Data)
	PreTool   EventType = "PreTool"    // before tool execution (ToolCall in Data)
	PostTool  EventType = "PostTool"   // after tool execution (ToolResult in Data)
	OnMessage EventType = "Message"    // message received on inbox (Message in Data)
	OnTurn    EventType = "Turn"       // think+act cycle completed (Result in Data)
)

// Event carries context for a hook invocation.
// Also emitted to Outbox for external observation.
type Event struct {
	Type   EventType // which event
	Source string    // who triggered (agent ID, tool name, "user")
	Data   any       // payload — type depends on EventType (see above)
}

// Action is a hook's response. Zero value = "continue normally."
//
// Fields are scoped by event:
//
//	Universal:  Block, Reason, Meta
//	PreInfer:   Inject
//	PreTool:    Modify
//
// Merge semantics (multiple hooks):
//
//	Block:  any true wins (short-circuits)
//	Modify: last writer wins
//	Inject: concatenate with newline
//	Meta:   merge maps, last writer wins per key
type Action struct {
	Block  bool   // stop the current operation
	Reason string // why (diagnostics, LLM context)

	Inject string         // [PreInfer] add context for the next LLM call
	Modify map[string]any // [PreTool] replace tool input parameters

	Meta map[string]any // app-layer extensions (UpdatedPermissions, WatchPaths, etc.)
}

// Handler is the hook function.
// Performs side effects and returns an Action.
// Use Hook.Async=true to run in a background goroutine (observe only).
type Handler func(ctx context.Context, event Event) (Action, error)

// Hook is a registered event handler with optional filtering.
type Hook struct {
	ID      string    // unique (auto-generated if empty)
	Event   EventType // which event to handle
	Matcher string    // filter on Event.Source; "" or "*" = match all
	Handle  Handler
	Async   bool // run in background goroutine (observe only, cannot block/modify)
	Once    bool // fire once then automatically remove
}

// Hooks manages event handlers for an agent.
type Hooks interface {
	Register(hook Hook) string                             // register, returns ID
	Unregister(id string) bool                             // unregister by ID
	On(ctx context.Context, event Event) (Action, error) // execute matching hooks
	Has(event EventType) bool                            // any hooks registered?
	Wait()                                               // wait for all async hook goroutines to finish
}

// Event.Data type helpers — reduce boilerplate in handlers.

func (e Event) ToolCall() (ToolCall, bool)       { tc, ok := e.Data.(ToolCall); return tc, ok }
func (e Event) ToolResult() (ToolResult, bool)   { tr, ok := e.Data.(ToolResult); return tr, ok }
func (e Event) Message() (Message, bool)         { m, ok := e.Data.(Message); return m, ok }
func (e Event) Result() (Result, bool)           { r, ok := e.Data.(Result); return r, ok }
func (e Event) Response() (*InferResponse, bool) { r, ok := e.Data.(*InferResponse); return r, ok }
func (e Event) Chunk() (Chunk, bool)             { c, ok := e.Data.(Chunk); return c, ok }
func (e Event) Error() (error, bool)             { err, ok := e.Data.(error); return err, ok }

// Typed event constructors — enforce correct Data types at construction.

func StartEvent(agentID string) Event { return Event{Type: OnStart, Source: agentID} }
func StopEvent(agentID string, err error) Event {
	return Event{Type: OnStop, Source: agentID, Data: err}
}
func ChunkEvent(agentID string, c Chunk) Event { return Event{Type: OnChunk, Source: agentID, Data: c} }
func MessageEvent(msg Message) Event           { return Event{Type: OnMessage, Source: msg.From, Data: msg} }
func TurnEvent(agentID string, r Result) Event { return Event{Type: OnTurn, Source: agentID, Data: r} }
func PreInferEvent(agentID string) Event       { return Event{Type: PreInfer, Source: agentID} }
func PostInferEvent(agentID string, r *InferResponse) Event {
	return Event{Type: PostInfer, Source: agentID, Data: r}
}
func PreToolEvent(tc ToolCall) Event    { return Event{Type: PreTool, Source: tc.Name, Data: tc} }
func PostToolEvent(tr ToolResult) Event { return Event{Type: PostTool, Source: tr.ToolName, Data: tr} }
