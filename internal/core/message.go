package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Role identifies who produced a message in the conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool_result"
	RoleNotice    Role = "notice"
)

// RoleToolResult is an alias for RoleTool.
const RoleToolResult = RoleTool

// Signal represents control signals sent through channels.
type Signal string

const (
	SigStop Signal = "stop"
)

// Message is the canonical message type used across the codebase.
type Message struct {
	ID                string         `json:"id,omitempty"`
	Role              Role           `json:"role"`
	Content           string         `json:"content,omitempty"`
	DisplayContent    string         `json:"display_content,omitempty"`
	Images            []Image        `json:"images,omitempty"`
	Thinking          string         `json:"thinking,omitempty"`
	ThinkingSignature string         `json:"thinking_signature,omitempty"`
	ToolCalls         []ToolCall     `json:"tool_calls,omitempty"`
	ToolResult        *ToolResult    `json:"tool_result,omitempty"`
	From              string         `json:"from,omitempty"`
	Signal            Signal         `json:"-"`
	Meta              map[string]any `json:"meta,omitempty"`
}

// ChatMessage represents a UI-layer chat message with display state.
type ChatMessage struct {
	Role              Role
	Content           string
	DisplayContent    string
	Thinking          string
	ThinkingSignature string
	Images            []Image
	ToolCalls         []ToolCall
	ToolCallsExpanded bool
	ToolResult        *ToolResult
	ToolName          string
	Expanded          bool
	RenderedInline    bool
}

// Image represents an image attachment.
type Image struct {
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

// ToolResult is the outcome of a tool execution.
type ToolResult struct {
	ToolCallID   string `json:"tool_call_id"`
	ToolName     string `json:"tool_name,omitempty"`
	Content      string `json:"content"`
	IsError      bool   `json:"is_error,omitempty"`
	HookResponse any    `json:"-"`
}

// --- Constructors ---

// UserMessage creates a user message with optional images.
func UserMessage(text string, images []Image) Message {
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

// --- Utilities ---

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

// --- Completion Response Types ---

// CompletionResponse represents a completion response from an LLM provider.
type CompletionResponse struct {
	Content           string     `json:"content,omitempty"`
	Thinking          string     `json:"thinking,omitempty"`
	ThinkingSignature string     `json:"thinking_signature,omitempty"`
	ToolCalls         []ToolCall `json:"tool_calls,omitempty"`
	StopReason        string     `json:"stop_reason"`
	Usage             Usage      `json:"usage"`
}

// Logging accessors — satisfy duck-typed interfaces in the log package so
// log does not need to import core (foundation-layer contract).
func (r CompletionResponse) LogStopReason() string  { return r.StopReason }
func (r CompletionResponse) LogContent() string     { return r.Content }
func (r CompletionResponse) LogThinking() string    { return r.Thinking }
func (r CompletionResponse) LogInputTokens() int    { return r.Usage.InputTokens }
func (r CompletionResponse) LogOutputTokens() int   { return r.Usage.OutputTokens }
func (r CompletionResponse) LogRawToolCalls() any   { return r.ToolCalls }
func (r CompletionResponse) LogRawUsage() any       { return r.Usage }

func (r CompletionResponse) LogToolCallSummary(escaper func(string) string) string {
	if len(r.ToolCalls) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "    ToolCalls(%d):\n", len(r.ToolCalls))
	for _, tc := range r.ToolCalls {
		fmt.Fprintf(&sb, "      [%s] %s(%s)\n", tc.ID, tc.Name, escaper(tc.Input))
	}
	return sb.String()
}

// Usage contains token usage information.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// --- Streaming Types ---

// ChunkType represents the type of a stream chunk from a provider.
type ChunkType string

const (
	ChunkTypeText      ChunkType = "text"
	ChunkTypeThinking  ChunkType = "thinking"
	ChunkTypeToolStart ChunkType = "tool_start"
	ChunkTypeToolInput ChunkType = "tool_input"
	ChunkTypeDone      ChunkType = "done"
	ChunkTypeError     ChunkType = "error"
)

// StreamChunk represents a chunk in a streaming response from a provider.
type StreamChunk struct {
	Type     ChunkType
	Text     string              // For text chunks
	ToolID   string              // For tool_start chunks
	ToolName string              // For tool_start chunks
	Response *CompletionResponse // For done chunks
	Error    error               // For error chunks
}

// --- Content Parts ---

// ContentPartType distinguishes text from image in interleaved content.
type ContentPartType string

const (
	ContentPartText  ContentPartType = "text"
	ContentPartImage ContentPartType = "image"
)

// ContentPart represents a text or image segment in interleaved content.
type ContentPart struct {
	Type  ContentPartType
	Text  string
	Image *Image
}

var inlineImageTokenRe = regexp.MustCompile(`\[Image #(\d+)\]`)

// InterleavedContentParts parses [Image #N] tokens from display content and returns
// interleaved text and image parts.
func InterleavedContentParts(msg Message) []ContentPart {
	if len(msg.Images) == 0 || msg.DisplayContent == "" || !inlineImageTokenRe.MatchString(msg.DisplayContent) {
		return nil
	}

	idToIdx := BuildImageIDMap(msg.DisplayContent, len(msg.Images))

	var parts []ContentPart
	last := 0
	matches := inlineImageTokenRe.FindAllStringSubmatchIndex(msg.DisplayContent, -1)
	if len(matches) > 0 {
		parts = make([]ContentPart, 0, len(matches)*2+1)
	}
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

// BuildImageIDMap parses [Image #N] tokens from displayContent and returns a map
// from token ID to sequential index (0-based). imageCount caps the number of entries.
func BuildImageIDMap(displayContent string, imageCount int) map[int]int {
	m := make(map[int]int, imageCount)
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
