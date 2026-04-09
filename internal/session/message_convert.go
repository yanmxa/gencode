package session

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
)

var inlineImageTokenPattern = regexp.MustCompile(`\[Image #(\d+)\]`)

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
				entry.Message = &EntryMessage{Role: "user", Content: UserContentToBlocks(msg.Content, msg.DisplayContent, msg.Images)}
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

func UserContentToBlocks(content, displayContent string, images []message.ImageData) []ContentBlock {
	if len(images) > 0 && displayContent != "" && inlineImageTokenPattern.MatchString(displayContent) {
		return interleavedUserContentToBlocks(content, displayContent, images)
	}

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

func interleavedUserContentToBlocks(content, displayContent string, images []message.ImageData) []ContentBlock {
	var blocks []ContentBlock
	var contentBuilder strings.Builder
	last := 0

	idToIdx := buildTokenIDMap(displayContent, len(images))

	matches := inlineImageTokenPattern.FindAllStringSubmatchIndex(displayContent, -1)
	for _, match := range matches {
		start, end := match[0], match[1]
		idStart, idEnd := match[2], match[3]

		textPart := displayContent[last:start]
		if textPart != "" {
			blocks = append(blocks, ContentBlock{Type: "text", Text: textPart})
			contentBuilder.WriteString(textPart)
		}

		id, err := strconv.Atoi(displayContent[idStart:idEnd])
		if err == nil {
			if idx, ok := idToIdx[id]; ok && idx < len(images) {
				img := images[idx]
				blocks = append(blocks, ContentBlock{
					Type:   "image",
					Source: &ImageSource{Type: "base64", MediaType: img.MediaType, Data: img.Data},
				})
			}
		}

		last = end
	}

	if tail := displayContent[last:]; tail != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: tail})
		contentBuilder.WriteString(tail)
	}

	if len(blocks) == 0 && content != "" {
		blocks = append(blocks, ContentBlock{Type: "text", Text: content})
	}

	return blocks
}

func buildTokenIDMap(displayContent string, imageCount int) map[int]int {
	m := make(map[int]int)
	matches := inlineImageTokenPattern.FindAllStringSubmatch(displayContent, -1)
	idx := 0
	for _, match := range matches {
		id, err := strconv.Atoi(match[1])
		if err == nil && idx < imageCount {
			m[id] = idx
			idx++
		}
	}
	return m
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
	imageCount := 0
	var display strings.Builder
	var content strings.Builder

	for _, block := range blocks {
		switch block.Type {
		case "text":
			msg.Content = block.Text
			content.WriteString(block.Text)
			display.WriteString(block.Text)
		case "image":
			if block.Source != nil {
				msg.Images = append(msg.Images, message.ImageData{MediaType: block.Source.MediaType, Data: block.Source.Data})
				imageCount++
				display.WriteString(fmt.Sprintf("[Image #%d]", imageCount))
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

	if msg.ToolResult == nil {
		msg.Content = content.String()
		msg.DisplayContent = display.String()
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

func ExtractUserText(entry Entry) (string, bool) {
	return extractUserText(entry)
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
