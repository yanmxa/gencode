package session

import (
	"github.com/yanmxa/gencode/internal/message"
)

func ConvertToEntries(messages []message.ChatMessage) []Entry {
	entries := make([]Entry, 0, len(messages))
	var prevUUID string

	for _, msg := range messages {
		if msg.Role == message.RoleNotice {
			continue
		}

		uuid := GenerateShortID()

		var parentUuid *string
		if prevUUID != "" {
			s := prevUUID
			parentUuid = &s
		}

		entry := Entry{
			UUID:       uuid,
			ParentUuid: parentUuid,
			Version:    AppVersion,
		}

		switch msg.Role {
		case message.RoleUser:
			entry.Type = EntryUser
			if msg.ToolResult != nil {
				entry.Message = &EntryMessage{
					Role:    "user",
					Content: ToolResultToBlocks(msg.ToolResult),
				}
			} else {
				entry.Message = &EntryMessage{
					Role:    "user",
					Content: UserContentToBlocks(msg.Content, msg.DisplayContent, msg.Images),
				}
			}

		case message.RoleAssistant:
			entry.Type = EntryAssistant
			entry.Message = &EntryMessage{
				Role:    "assistant",
				Content: AssistantContentToBlocks(msg.Content, msg.Thinking, msg.ThinkingSignature, msg.ToolCalls),
			}

		case message.RoleToolResult:
			entry.Type = EntryUser
			if msg.ToolResult != nil {
				entry.Message = &EntryMessage{
					Role:    "user",
					Content: ToolResultToBlocks(msg.ToolResult),
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

func ConvertFromEntries(entries []Entry) []message.ChatMessage {
	coreMsgs := EntriesToMessages(entries)
	messages := make([]message.ChatMessage, 0, len(coreMsgs))

	for _, m := range coreMsgs {
		chatMsg := message.ChatMessage{
			Role:              m.Role,
			Content:           m.Content,
			DisplayContent:    m.DisplayContent,
			Images:            m.Images,
			Thinking:          m.Thinking,
			ThinkingSignature: m.ThinkingSignature,
			ToolCalls:         m.ToolCalls,
		}
		if m.ToolResult != nil {
			chatMsg.ToolResult = m.ToolResult
			chatMsg.ToolName = m.ToolResult.ToolName
		}
		messages = append(messages, chatMsg)
	}

	return messages
}
