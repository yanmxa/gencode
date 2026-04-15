// Package message defines the canonical message types and utilities used across the codebase.
// All packages import from here to avoid circular dependencies.
package message

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yanmxa/gencode/internal/core"
)

// ChatMessage represents a UI-layer chat message with display state.
type ChatMessage struct {
	Role              Role
	Content           string
	DisplayContent    string
	Thinking          string
	ThinkingSignature string
	Images            []ImageData
	ToolCalls         []ToolCall
	ToolCallsExpanded bool
	ToolResult        *ToolResult
	ToolName          string
	Expanded          bool
	RenderedInline    bool
}

type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleNotice     Role = "notice"
	RoleToolResult Role = "tool_result"
)

// Message represents a chat message exchanged between user and assistant.
type Message struct {
	Role              Role        `json:"role"`
	Content           string      `json:"content,omitempty"`
	DisplayContent    string      `json:"display_content,omitempty"`
	Images            []ImageData `json:"images,omitempty"`
	Thinking          string      `json:"thinking,omitempty"`
	ThinkingSignature string      `json:"thinking_signature,omitempty"`
	ToolCalls         []ToolCall  `json:"tool_calls,omitempty"`
	ToolResult        *ToolResult `json:"tool_result,omitempty"`
}

type ImageData struct {
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	FileName  string `json:"file_name"`
	Size      int    `json:"size"`
}

// ToolCall represents a tool call from the model.
type ToolCall struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Input            string `json:"input"`
	ThoughtSignature []byte `json:"thought_signature,omitempty"` // Google Gemini: opaque signature to echo back
}

type ToolResult struct {
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name,omitempty"`
	Content      string `json:"content"`
	IsError      bool   `json:"is_error,omitempty"`
	HookResponse any    `json:"-"`
}

// UserMessage creates a user message with optional images.
func UserMessage(text string, images []ImageData) Message {
	return Message{
		Role:           RoleUser,
		Content:        text,
		DisplayContent: text,
		Images:         images,
	}
}

// AssistantMessage creates an assistant message.
func AssistantMessage(text, thinking string, calls []ToolCall) Message {
	return Message{
		Role:      RoleAssistant,
		Content:   text,
		Thinking:  thinking,
		ToolCalls: calls,
	}
}

// ErrorResult creates an error ToolResult for a tool call.
func ErrorResult(tc ToolCall, content string) *ToolResult {
	return &ToolResult{
		ToolCallID: tc.ID,
		ToolName:   tc.Name,
		Content:    content,
		IsError:    true,
	}
}

// ToolResultMessage creates a tool result message.
func ToolResultMessage(result ToolResult) Message {
	return Message{
		Role:       RoleUser,
		ToolResult: &result,
	}
}

// ParseToolInput deserializes JSON tool input into a params map.
func ParseToolInput(input string) (map[string]any, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return map[string]any{}, nil
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return nil, err
	}
	return params, nil
}

// BuildConversationText converts messages to text for summarization.
func BuildConversationText(msgs []Message) string {
	var sb strings.Builder
	sb.WriteString("Please summarize this coding conversation:\n\n")

	for _, msg := range msgs {
		switch msg.Role {
		case RoleUser:
			if msg.ToolResult != nil {
				content := msg.ToolResult.Content
				if len(content) > 500 {
					content = content[:500] + "...[truncated]"
				}
				fmt.Fprintf(&sb, "[Tool Result: %s]\n%s\n\n", msg.ToolResult.ToolName, content)
			} else {
				fmt.Fprintf(&sb, "User: %s\n\n", msg.Content)
			}

		case RoleAssistant:
			if msg.Content != "" {
				fmt.Fprintf(&sb, "Assistant: %s\n\n", msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					fmt.Fprintf(&sb, "[Tool Call: %s]\n", tc.Name)
				}
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// LastAssistantContent returns the most recent non-empty assistant content from provider messages.
func LastAssistantContent(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleAssistant && msgs[i].Content != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// LastAssistantChatContent returns the most recent non-empty assistant content from chat messages.
func LastAssistantChatContent(msgs []ChatMessage) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleAssistant && msgs[i].Content != "" {
			return msgs[i].Content
		}
	}
	return ""
}

// NeedsCompaction checks if token usage exceeds the threshold percentage of the input limit.
func NeedsCompaction(inputTokens, inputLimit int) bool {
	if inputLimit == 0 || inputTokens == 0 {
		return false
	}
	return float64(inputTokens)/float64(inputLimit)*100 >= 95
}

// CompletionResponse represents a completion response from an LLM provider.
type CompletionResponse struct {
	Content           string     `json:"content,omitempty"`
	Thinking          string     `json:"thinking,omitempty"`           // Reasoning content for thinking models
	ThinkingSignature string     `json:"thinking_signature,omitempty"` // Anthropic: signature for thinking block replay
	ToolCalls         []ToolCall `json:"tool_calls,omitempty"`
	StopReason        string     `json:"stop_reason"` // "end_turn", "tool_use", "max_tokens"
	Usage             Usage      `json:"usage"`
}

// Usage contains token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ChunkType represents the type of a stream chunk.
type ChunkType string

const (
	ChunkTypeText      ChunkType = "text"
	ChunkTypeThinking  ChunkType = "thinking"
	ChunkTypeToolStart ChunkType = "tool_start"
	ChunkTypeToolInput ChunkType = "tool_input"
	ChunkTypeDone      ChunkType = "done"
	ChunkTypeError     ChunkType = "error"
)

// StreamChunk represents a chunk in a streaming response.
type StreamChunk struct {
	Type     ChunkType
	Text     string              // For text chunks
	ToolID   string              // For tool_start chunks
	ToolName string              // For tool_start chunks
	Response *CompletionResponse // For done chunks
	Error    error               // For error chunks
}

type ContentPartType string

const (
	ContentPartText  ContentPartType = "text"
	ContentPartImage ContentPartType = "image"
)

type ContentPart struct {
	Type  ContentPartType
	Text  string
	Image *ImageData
}

var inlineImageTokenRe = regexp.MustCompile(`\[Image #(\d+)\]`)

func InterleavedContentParts(msg Message) []ContentPart {
	if len(msg.Images) == 0 || msg.DisplayContent == "" || !inlineImageTokenRe.MatchString(msg.DisplayContent) {
		return nil
	}

	idToIdx := buildImageIDMap(msg.DisplayContent, len(msg.Images))

	var parts []ContentPart
	last := 0
	matches := inlineImageTokenRe.FindAllStringSubmatchIndex(msg.DisplayContent, -1)
	for _, match := range matches {
		start, end := match[0], match[1]
		idStart, idEnd := match[2], match[3]

		if text := msg.DisplayContent[last:start]; text != "" {
			parts = append(parts, ContentPart{Type: ContentPartText, Text: text})
		}

		id, err := strconv.Atoi(msg.DisplayContent[idStart:idEnd])
		if err == nil {
			if idx, ok := idToIdx[id]; ok && idx < len(msg.Images) {
				img := msg.Images[idx]
				parts = append(parts, ContentPart{Type: ContentPartImage, Image: &img})
			}
		}

		last = end
	}

	if tail := msg.DisplayContent[last:]; tail != "" {
		parts = append(parts, ContentPart{Type: ContentPartText, Text: tail})
	}

	if len(parts) == 0 {
		return nil
	}
	return parts
}

func buildImageIDMap(displayContent string, imageCount int) map[int]int {
	m := make(map[int]int)
	matches := inlineImageTokenRe.FindAllStringSubmatch(displayContent, -1)
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

func ToCore(m Message) core.Message {
	msg := core.Message{
		Content:           m.Content,
		DisplayContent:    m.DisplayContent,
		Thinking:          m.Thinking,
		ThinkingSignature: m.ThinkingSignature,
	}
	switch m.Role {
	case RoleUser:
		msg.Role = core.RoleUser
		msg.Images = imagesToCore(m.Images)
	case RoleAssistant:
		msg.Role = core.RoleAssistant
		msg.ToolCalls = toolCallsToCore(m.ToolCalls)
	case RoleToolResult:
		msg.Role = core.RoleTool
		if m.ToolResult != nil {
			msg.ToolResult = &core.ToolResult{
				ToolCallID: m.ToolResult.ToolCallID,
				ToolName:   m.ToolResult.ToolName,
				Content:    m.ToolResult.Content,
				IsError:    m.ToolResult.IsError,
			}
			msg.Content = m.ToolResult.Content
		}
	}
	return msg
}

func ToCoreSlice(msgs []Message) []core.Message {
	out := make([]core.Message, len(msgs))
	for i, m := range msgs {
		out[i] = ToCore(m)
	}
	return out
}

func FromCore(m core.Message) Message {
	msg := Message{
		Content:  m.Content,
		Thinking: m.Thinking,
	}
	switch m.Role {
	case core.RoleUser:
		msg.Role = RoleUser
		msg.Images = imagesFromCore(m.Images)
	case core.RoleAssistant:
		msg.Role = RoleAssistant
		msg.ToolCalls = toolCallsFromCore(m.ToolCalls)
	case core.RoleTool:
		msg.Role = RoleToolResult
		if m.ToolResult != nil {
			msg.ToolResult = &ToolResult{
				ToolCallID: m.ToolResult.ToolCallID,
				ToolName:   m.ToolResult.ToolName,
				Content:    m.ToolResult.Content,
				IsError:    m.ToolResult.IsError,
			}
		}
	}
	return msg
}

func FromCoreSlice(msgs []core.Message) []Message {
	out := make([]Message, len(msgs))
	for i, m := range msgs {
		out[i] = FromCore(m)
	}
	return out
}

func toolCallsToCore(calls []ToolCall) []core.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]core.ToolCall, len(calls))
	for i, tc := range calls {
		input := make(map[string]any)
		_ = json.Unmarshal([]byte(tc.Input), &input)
		out[i] = core.ToolCall{
			ID:    tc.ID,
			Name:  tc.Name,
			Input: input,
		}
	}
	return out
}

func toolCallsFromCore(calls []core.ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, len(calls))
	for i, tc := range calls {
		inputJSON, _ := json.Marshal(tc.Input)
		out[i] = ToolCall{
			ID:    tc.ID,
			Name:  tc.Name,
			Input: string(inputJSON),
		}
	}
	return out
}

func imagesToCore(imgs []ImageData) []core.Image {
	if len(imgs) == 0 {
		return nil
	}
	out := make([]core.Image, len(imgs))
	for i, img := range imgs {
		out[i] = core.Image{
			MediaType: img.MediaType,
			Data:      img.Data,
			FileName:  img.FileName,
			Size:      img.Size,
		}
	}
	return out
}

func imagesFromCore(imgs []core.Image) []ImageData {
	if len(imgs) == 0 {
		return nil
	}
	out := make([]ImageData, len(imgs))
	for i, img := range imgs {
		out[i] = ImageData{
			MediaType: img.MediaType,
			Data:      img.Data,
			FileName:  img.FileName,
			Size:      img.Size,
		}
	}
	return out
}


