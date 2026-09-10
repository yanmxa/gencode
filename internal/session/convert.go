package session

import (
	"github.com/genai-io/san/internal/core"
)

// ConvertToEntries turns domain messages into transcript entries. Display-only
// notices are dropped; stable IDs survive so the append-only store can dedupe.
func ConvertToEntries(messages []core.Message) []Entry {
	msgs := make([]core.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == core.RoleNotice {
			continue
		}
		msgs = append(msgs, msg)
	}
	return messagesToEntries(msgs)
}

func ConvertFromEntries(entries []Entry) []core.Message {
	return EntriesToMessages(entries)
}
