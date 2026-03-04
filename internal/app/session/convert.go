package session

import (
	"github.com/yanmxa/gencode/internal/message"
	coresession "github.com/yanmxa/gencode/internal/session"
)

// ConvertToEntries converts ChatMessages to Entries for session persistence.
func ConvertToEntries(messages []message.ChatMessage) []coresession.Entry {
	entries := make([]coresession.Entry, 0, len(messages))
	var prevUUID string

	for _, msg := range messages {
		if msg.Role == message.RoleNotice {
			continue
		}

		uuid := coresession.GenerateShortID()

		var parentUuid *string
		if prevUUID != "" {
			s := prevUUID
			parentUuid = &s
		}

		entry := coresession.Entry{
			UUID:       uuid,
			ParentUuid: parentUuid,
			Version:    coresession.AppVersion,
		}

		switch msg.Role {
		case message.RoleUser:
			entry.Type = coresession.EntryUser
			if msg.ToolResult != nil {
				entry.Message = &coresession.EntryMessage{
					Role:    "user",
					Content: coresession.ToolResultToBlocks(msg.ToolResult),
				}
			} else {
				entry.Message = &coresession.EntryMessage{
					Role:    "user",
					Content: coresession.UserContentToBlocks(msg.Content, msg.Images),
				}
			}

		case message.RoleAssistant:
			entry.Type = coresession.EntryAssistant
			entry.Message = &coresession.EntryMessage{
				Role:    "assistant",
				Content: coresession.AssistantContentToBlocks(msg.Content, msg.Thinking, msg.ToolCalls),
			}

		case message.RoleToolResult:
			entry.Type = coresession.EntryUser
			if msg.ToolResult != nil {
				entry.Message = &coresession.EntryMessage{
					Role:    "user",
					Content: coresession.ToolResultToBlocks(msg.ToolResult),
				}
			}

		default:
			continue
		}

		entries = append(entries, entry)
		prevUUID = uuid
	}

	return entries
}

// ConvertFromEntries converts Entries back to ChatMessages after loading.
// It reuses EntriesToMessages from the core session package and converts the result.
func ConvertFromEntries(entries []coresession.Entry) []message.ChatMessage {
	coreMsgs := coresession.EntriesToMessages(entries)
	messages := make([]message.ChatMessage, 0, len(coreMsgs))

	for _, m := range coreMsgs {
		chatMsg := message.ChatMessage{
			Role:      m.Role,
			Content:   m.Content,
			Images:    m.Images,
			Thinking:  m.Thinking,
			ToolCalls: m.ToolCalls,
		}
		if m.ToolResult != nil {
			chatMsg.ToolResult = m.ToolResult
			chatMsg.ToolName = m.ToolResult.ToolName
		}
		messages = append(messages, chatMsg)
	}

	return messages
}
