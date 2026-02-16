// Package message defines the canonical message types and utilities used across the codebase.
// All packages import from here to avoid circular dependencies.
package message

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Role represents the role of a message participant.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "tool_result"
)

// Message represents a chat message exchanged between user and assistant.
type Message struct {
	Role       Role        `json:"role"`
	Content    string      `json:"content,omitempty"`
	Images     []ImageData `json:"images,omitempty"`
	Thinking   string      `json:"thinking,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolResult *ToolResult `json:"tool_result,omitempty"`
}

// ImageData represents image data for multimodal messages.
type ImageData struct {
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	FileName  string `json:"file_name"`
	Size      int    `json:"size"`
}

// ToolCall represents a tool call from the model.
type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name,omitempty"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// UserMessage creates a user message with optional images.
func UserMessage(text string, images []ImageData) Message {
	return Message{
		Role:    RoleUser,
		Content: text,
		Images:  images,
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

// NeedsCompaction checks if token usage exceeds the threshold percentage of the input limit.
func NeedsCompaction(inputTokens, inputLimit int) bool {
	if inputLimit == 0 || inputTokens == 0 {
		return false
	}
	return float64(inputTokens)/float64(inputLimit)*100 >= 95
}

// CompletionResponse represents a completion response from an LLM provider.
type CompletionResponse struct {
	Content    string     `json:"content,omitempty"`
	Thinking   string     `json:"thinking,omitempty"` // Reasoning content for thinking models
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	StopReason string     `json:"stop_reason"` // "end_turn", "tool_use", "max_tokens"
	Usage      Usage      `json:"usage"`
}

// Usage contains token usage information.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
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
