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
}

// New returns an empty conversation model.
func New() Model {
	return Model{Messages: []core.ChatMessage{}}
}
