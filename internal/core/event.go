package core

// EventType identifies an agent lifecycle event.
type EventType string

// Agent lifecycle events — emitted to the Outbox for TUI rendering.
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

// Event carries context for an agent lifecycle point.
// Emitted to Outbox for TUI observation.
type Event struct {
	Type   EventType // which event
	Source string    // who triggered (agent ID, tool name, "user")
	Data   any       // payload — type depends on EventType (see above)
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
