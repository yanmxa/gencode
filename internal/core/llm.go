package core

import "context"

// StopReason describes why the LLM stopped generating.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopMaxTokens StopReason = "max_tokens"
	StopToolUse   StopReason = "tool_use"
)

// InferRequest is sent to the LLM for inference.
type InferRequest struct {
	System   string       // assembled system prompt
	Messages []Message    // conversation history
	Tools    []ToolSchema // available tools
}

// InferResponse is the final aggregated response from one LLM call.
type InferResponse struct {
	Content    string     // text output
	Thinking   string     // chain-of-thought (extended thinking)
	ToolCalls  []ToolCall // tool execution requests
	StopReason StopReason
	TokensIn   int
	TokensOut  int
}

// Chunk is one piece of a streaming LLM response.
type Chunk struct {
	Text     string // incremental text
	Thinking string // incremental thinking
	Done     bool   // true on final chunk

	Response *InferResponse // non-nil only when Done=true
	Err      error          // non-nil on stream error
}

// LLM is the inference abstraction — call a language model.
//
// Infer sends a request and returns a channel of streaming chunks.
// The channel is closed when the response is complete or on error.
// The final chunk has Done=true and carries the aggregated InferResponse.
type LLM interface {
	Infer(ctx context.Context, req InferRequest) (<-chan Chunk, error)
}
