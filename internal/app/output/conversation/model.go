package conversation

import (
	appcompact "github.com/yanmxa/gencode/internal/app/output/compact"
	"github.com/yanmxa/gencode/internal/core"
)

// Model holds the conversation message history, commit tracking, stream and compact state.
type Model struct {
	Messages       []core.ChatMessage
	CommittedCount int
	Stream         StreamState
	Compact        appcompact.State

	// TurnsSinceLastTaskTool counts LLM turns since the last Task* tool was used.
	// Reset to 0 when any TaskCreate/TaskGet/TaskUpdate/TaskList tool is called.
	// Used to inject task reminder nudges after a threshold.
	TurnsSinceLastTaskTool int
}

// New returns an empty conversation model.
func New() Model {
	return Model{Messages: []core.ChatMessage{}}
}
