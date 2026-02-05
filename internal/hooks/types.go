// Package hooks implements the event hooks system for GenCode.
// Compatible with Claude Code hooks that execute shell commands on events.
package hooks

// EventType represents the type of hook event.
type EventType string

// Event types with their matcher support noted.
const (
	SessionStart       EventType = "SessionStart"       // matcher: startup, resume, clear, compact
	UserPromptSubmit   EventType = "UserPromptSubmit"   // no matcher
	PreToolUse         EventType = "PreToolUse"         // matcher: tool name
	PermissionRequest  EventType = "PermissionRequest"  // matcher: tool name
	PostToolUse        EventType = "PostToolUse"        // matcher: tool name
	PostToolUseFailure EventType = "PostToolUseFailure" // matcher: tool name
	Notification       EventType = "Notification"       // matcher: notification_type
	SubagentStart      EventType = "SubagentStart"      // matcher: agent_type
	SubagentStop       EventType = "SubagentStop"       // matcher: agent_type
	Stop               EventType = "Stop"               // no matcher
	PreCompact         EventType = "PreCompact"         // matcher: manual, auto
	SessionEnd         EventType = "SessionEnd"         // matcher: reason
)

// HookInput is the JSON input passed to hook commands via stdin.
type HookInput struct {
	// Common fields
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	HookEventName  string `json:"hook_event_name"`

	// Tool events
	ToolName     string         `json:"tool_name,omitempty"`
	ToolInput    map[string]any `json:"tool_input,omitempty"`
	ToolUseID    string         `json:"tool_use_id,omitempty"`
	ToolResponse any            `json:"tool_response,omitempty"`
	Error        string         `json:"error,omitempty"`
	IsInterrupt  bool           `json:"is_interrupt,omitempty"`

	// UserPromptSubmit
	Prompt string `json:"prompt,omitempty"`

	// Notification
	Message          string `json:"message,omitempty"`
	Title            string `json:"title,omitempty"`
	NotificationType string `json:"notification_type,omitempty"`

	// Agent events
	AgentID             string `json:"agent_id,omitempty"`
	AgentType           string `json:"agent_type,omitempty"`
	AgentTranscriptPath string `json:"agent_transcript_path,omitempty"`
	StopHookActive      bool   `json:"stop_hook_active,omitempty"`

	// Session events
	Source             string `json:"source,omitempty"`
	Model              string `json:"model,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Trigger            string `json:"trigger,omitempty"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
}

// HookOutput is the JSON output from hook commands.
type HookOutput struct {
	Continue           *bool              `json:"continue,omitempty"`
	StopReason         string             `json:"stopReason,omitempty"`
	SuppressOutput     bool               `json:"suppressOutput,omitempty"`
	SystemMessage      string             `json:"systemMessage,omitempty"`
	Decision           string             `json:"decision,omitempty"`
	Reason             string             `json:"reason,omitempty"`
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput contains event-specific output fields.
type HookSpecificOutput struct {
	HookEventName            string                     `json:"hookEventName"`
	PermissionDecision       string                     `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string                     `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             map[string]any             `json:"updatedInput,omitempty"`
	AdditionalContext        string                     `json:"additionalContext,omitempty"`
	PermissionRequestDecision *PermissionRequestDecision `json:"decision,omitempty"`
}

// PermissionRequestDecision represents permission request hook decision.
type PermissionRequestDecision struct {
	Behavior           string         `json:"behavior"`
	UpdatedInput       map[string]any `json:"updatedInput,omitempty"`
	UpdatedPermissions []any          `json:"updatedPermissions,omitempty"`
	Message            string         `json:"message,omitempty"`
	Interrupt          bool           `json:"interrupt,omitempty"`
}

// HookOutcome is the processed result from hook execution.
type HookOutcome struct {
	ShouldContinue    bool
	ShouldBlock       bool
	BlockReason       string
	AdditionalContext string
	UpdatedInput      map[string]any
	Error             error
}
