package session

import (
	"crypto/rand"
	"encoding/json"
	"fmt"

	"github.com/yanmxa/gencode/internal/message"
)

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
				entry.Message = &EntryMessage{Role: "user", Content: ToolResultToBlocks(msg.ToolResult)}
			} else {
				entry.Message = &EntryMessage{Role: "user", Content: UserContentToBlocks(msg.Content, msg.Images)}
			}
		case message.RoleAssistant:
			entry.Type = EntryAssistant
			entry.Message = &EntryMessage{
				Role:    "assistant",
				Content: AssistantContentToBlocks(msg.Content, msg.Thinking, msg.ThinkingSignature, msg.ToolCalls),
			}
		default:
			continue
		}

		entries = append(entries, entry)
		prevUUID = uuid
	}

	return entries
}

func EntriesToMessages(entries []Entry) []message.Message {
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
		}
	}
	return msgs
}

func UserContentToBlocks(content string, images []message.ImageData) []ContentBlock {
	var blocks []ContentBlock
	for _, img := range images {
		blocks = append(blocks, ContentBlock{
			Type:   "image",
			Source: &ImageSource{Type: "base64", MediaType: img.MediaType, Data: img.Data},
		})
	}
	if content != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: content})
	}
	return blocks
}

func AssistantContentToBlocks(content, thinking, thinkingSignature string, toolCalls []message.ToolCall) []ContentBlock {
	var blocks []ContentBlock
	if thinking != "" {
		blocks = append(blocks, ContentBlock{Type: "thinking", Thinking: thinking, Signature: thinkingSignature})
	}
	if content != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: content})
	}
	for _, tc := range toolCalls {
		block := ContentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name}
		if tc.Input != "" {
			block.Input = json.RawMessage(tc.Input)
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func ToolResultToBlocks(tr *message.ToolResult) []ContentBlock {
	block := ContentBlock{Type: "tool_result", ToolUseID: tr.ToolCallID, IsError: tr.IsError}
	if tr.Content != "" {
		block.Content = []ContentBlock{{Type: "text", Text: tr.Content}}
	}
	return []ContentBlock{block}
}

func ExtractFirstUserText(entries []Entry) string {
	for _, entry := range entries {
		if text, ok := extractUserText(entry); ok {
			return text
		}
	}
	return ""
}

func ExtractLastUserText(entries []Entry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if text, ok := extractUserText(entries[i]); ok {
			return text
		}
	}
	return ""
}

func extractUserContent(blocks []ContentBlock, msg *message.Message) {
	for _, block := range blocks {
		switch block.Type {
		case "text":
			msg.Content = block.Text
		case "image":
			if block.Source != nil {
				msg.Images = append(msg.Images, message.ImageData{MediaType: block.Source.MediaType, Data: block.Source.Data})
			}
		case "tool_result":
			tr := &message.ToolResult{ToolCallID: block.ToolUseID, IsError: block.IsError}
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
			msg.ThinkingSignature = block.Signature
		case "tool_use":
			tc := message.ToolCall{ID: block.ID, Name: block.Name}
			if block.Input != nil {
				tc.Input = string(block.Input)
			}
			msg.ToolCalls = append(msg.ToolCalls, tc)
		}
	}
}

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

func GenerateShortID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b[:])
}
