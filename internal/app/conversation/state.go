package conversation

import (
	"context"

	"github.com/yanmxa/gencode/internal/message"
)

// StreamState holds all streaming-related state for the TUI model.
type StreamState struct {
	Active       bool
	Ch           <-chan message.StreamChunk
	Cancel       context.CancelFunc
	BuildingTool string
}

// Stop clears streaming state. Caller responsible for calling Cancel() first if needed.
func (s *StreamState) Stop() {
	s.Active = false
	s.Ch = nil
	s.Cancel = nil
	s.BuildingTool = ""
}

// ChunkMsg carries a single streaming chunk from the LLM.
type ChunkMsg struct {
	Text             string
	Thinking         string
	Done             bool
	Err              error
	ToolCalls        []message.ToolCall
	BuildingToolName string
	Usage            *message.Usage
}

// ContinueMsg requests a follow-up LLM call with the given messages.
type ContinueMsg struct {
	Messages []message.Message
}
