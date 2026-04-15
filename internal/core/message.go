package core

// Role identifies who produced a message in the conversation.
type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleTool       Role = "tool"
	RoleToolResult Role = "tool_result"
	RoleNotice     Role = "notice"
)

// ToolCall is the LLM's request to execute a tool.
type ToolCall struct {
	ID    string
	Name  string
	Input map[string]any
}

// ToolResult is the outcome of a tool execution.
type ToolResult struct {
	ToolCallID   string
	ToolName     string
	Content      string
	IsError      bool
	HookResponse any
}

// Message is the universal message type.
type Message struct {
	ID                string
	Role              Role
	Content           string
	DisplayContent    string
	Thinking          string
	ThinkingSignature string
	Images            []Image
	ToolCalls         []ToolCall
	ToolResult        *ToolResult
	From              string
	Signal            Signal
	Meta              map[string]any
}

// Signal represents control signals sent through channels.
type Signal string

const (
	SigStop   Signal = "stop"
	SigPause  Signal = "pause"
	SigResume Signal = "resume"
)

// Image represents an image attachment.
type Image struct {
	MediaType string
	Data      string
	FileName  string
	Size      int
}
