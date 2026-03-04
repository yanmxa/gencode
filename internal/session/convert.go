package session

import (
	"encoding/json"

	"github.com/yanmxa/gencode/internal/message"
)

// MessagesToEntries converts core message.Message slices (used by core.Loop)
// to Entry slices for session persistence.
func MessagesToEntries(msgs []message.Message) []Entry {
	entries := make([]Entry, 0, len(msgs))
	var prevUUID string

	for _, msg := range msgs {
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
					Content: UserContentToBlocks(msg.Content, msg.Images),
				}
			}
		case message.RoleAssistant:
			entry.Type = EntryAssistant
			entry.Message = &EntryMessage{
				Role:    "assistant",
				Content: AssistantContentToBlocks(msg.Content, msg.Thinking, msg.ToolCalls),
			}
		default:
			continue
		}

		entries = append(entries, entry)
		prevUUID = uuid
	}

	return entries
}

// EntriesToMessages converts Entry slices back to core message.Message slices.
func EntriesToMessages(entries []Entry) []message.Message {
	// Build a map of tool_use_id → tool_name for resolving tool result names.
	toolNameMap := make(map[string]string)
	for _, entry := range entries {
		if entry.Type == EntryAssistant && entry.Message != nil {
			for _, block := range entry.Message.Content {
				if block.Type == "tool_use" {
					toolNameMap[block.ID] = block.Name
				}
			}
		}
	}

	msgs := make([]message.Message, 0, len(entries))

	for _, entry := range entries {
		switch entry.Type {
		case EntryUser:
			msg := message.Message{Role: message.RoleUser}
			if entry.Message != nil {
				extractUserContent(entry.Message.Content, &msg)
			}
			// Resolve tool name from the preceding tool_use block.
			if msg.ToolResult != nil && msg.ToolResult.ToolName == "" {
				if name, ok := toolNameMap[msg.ToolResult.ToolCallID]; ok {
					msg.ToolResult.ToolName = name
				}
			}
			msgs = append(msgs, msg)

		case EntryAssistant:
			msg := message.Message{Role: message.RoleAssistant}
			if entry.Message != nil {
				extractAssistantContent(entry.Message.Content, &msg)
			}
			msgs = append(msgs, msg)

		// Skip unknown entry types (including legacy "summary" entries).
		}
	}

	return msgs
}

// --- Content block builders (exported for use by app/session) ---

// UserContentToBlocks converts user text and images to content blocks.
func UserContentToBlocks(content string, images []message.ImageData) []ContentBlock {
	var blocks []ContentBlock
	for _, img := range images {
		blocks = append(blocks, ContentBlock{
			Type: "image",
			Source: &ImageSource{
				Type:      "base64",
				MediaType: img.MediaType,
				Data:      img.Data,
			},
		})
	}
	if content != "" {
		blocks = append(blocks, ContentBlock{
			Type: "text",
			Text: content,
		})
	}
	return blocks
}

// AssistantContentToBlocks converts assistant text, thinking, and tool calls to content blocks.
func AssistantContentToBlocks(content, thinking string, toolCalls []message.ToolCall) []ContentBlock {
	var blocks []ContentBlock
	if thinking != "" {
		blocks = append(blocks, ContentBlock{
			Type:     "thinking",
			Thinking: thinking,
		})
	}
	if content != "" {
		blocks = append(blocks, ContentBlock{
			Type: "text",
			Text: content,
		})
	}
	for _, tc := range toolCalls {
		block := ContentBlock{
			Type: "tool_use",
			ID:   tc.ID,
			Name: tc.Name,
		}
		if tc.Input != "" {
			block.Input = json.RawMessage(tc.Input)
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// ToolResultToBlocks converts a tool result to content blocks.
func ToolResultToBlocks(tr *message.ToolResult) []ContentBlock {
	block := ContentBlock{
		Type:      "tool_result",
		ToolUseID: tr.ToolCallID,
		IsError:   tr.IsError,
	}
	if tr.Content != "" {
		block.Content = []ContentBlock{{
			Type: "text",
			Text: tr.Content,
		}}
	}
	return []ContentBlock{block}
}

// --- Content block extractors ---

func extractUserContent(blocks []ContentBlock, msg *message.Message) {
	for _, block := range blocks {
		switch block.Type {
		case "text":
			msg.Content = block.Text
		case "image":
			if block.Source != nil {
				msg.Images = append(msg.Images, message.ImageData{
					MediaType: block.Source.MediaType,
					Data:      block.Source.Data,
				})
			}
		case "tool_result":
			tr := &message.ToolResult{
				ToolCallID: block.ToolUseID,
				IsError:    block.IsError,
			}
			for _, sub := range block.Content {
				if sub.Type == "text" {
					tr.Content = sub.Text
				}
			}
			msg.ToolResult = tr
		}
	}
}

func extractAssistantContent(blocks []ContentBlock, msg *message.Message) {
	for _, block := range blocks {
		switch block.Type {
		case "text":
			msg.Content = block.Text
		case "thinking":
			msg.Thinking = block.Thinking
		case "tool_use":
			tc := message.ToolCall{
				ID:   block.ID,
				Name: block.Name,
			}
			if block.Input != nil {
				tc.Input = string(block.Input)
			}
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
	}
}

// --- Helpers ---

// ExtractFirstUserText returns the first text content from user entries,
// skipping tool_result entries. Used for title generation and index building.
func ExtractFirstUserText(entries []Entry) string {
	for _, entry := range entries {
		if text, ok := extractUserText(entry); ok {
			return text
		}
	}
	return ""
}

// extractUserText returns the text from a user entry if it's a plain text
// message (not a tool_result). Returns ("", false) otherwise.
func extractUserText(entry Entry) (string, bool) {
	if entry.Type != EntryUser || entry.Message == nil {
		return "", false
	}
	var text string
	for _, block := range entry.Message.Content {
		if block.Type == "tool_result" {
			return "", false
		}
		if block.Type == "text" && block.Text != "" && text == "" {
			text = block.Text
		}
	}
	if text != "" {
		return text, true
	}
	return "", false
}
