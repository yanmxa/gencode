package message

import (
	"strings"
	"testing"
)

func TestUserMessage(t *testing.T) {
	msg := UserMessage("hello", nil)
	if msg.Role != RoleUser {
		t.Errorf("expected role %q, got %q", RoleUser, msg.Role)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", msg.Content)
	}
	if len(msg.Images) != 0 {
		t.Errorf("expected 0 images, got %d", len(msg.Images))
	}
}

func TestUserMessageWithImages(t *testing.T) {
	images := []ImageData{
		{MediaType: "image/png", Data: "abc123", FileName: "test.png", Size: 100},
	}
	msg := UserMessage("describe this", images)
	if msg.Role != RoleUser {
		t.Errorf("expected role %q, got %q", RoleUser, msg.Role)
	}
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(msg.Images))
	}
	if msg.Images[0].MediaType != "image/png" {
		t.Errorf("expected media type 'image/png', got %q", msg.Images[0].MediaType)
	}
}

func TestAssistantMessage(t *testing.T) {
	calls := []ToolCall{
		{ID: "tc1", Name: "Read", Input: `{"file_path": "/tmp"}`},
	}
	msg := AssistantMessage("hello", "thinking...", calls)
	if msg.Role != RoleAssistant {
		t.Errorf("expected role %q, got %q", RoleAssistant, msg.Role)
	}
	if msg.Content != "hello" {
		t.Errorf("expected content 'hello', got %q", msg.Content)
	}
	if msg.Thinking != "thinking..." {
		t.Errorf("expected thinking 'thinking...', got %q", msg.Thinking)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
}

func TestToolResultMessage(t *testing.T) {
	result := ToolResult{
		ToolCallID: "tc1",
		ToolName:   "Read",
		Content:    "file content",
		IsError:    false,
	}
	msg := ToolResultMessage(result)
	if msg.Role != RoleUser {
		t.Errorf("expected role %q, got %q", RoleUser, msg.Role)
	}
	if msg.ToolResult == nil {
		t.Fatal("expected tool result, got nil")
	}
	if msg.ToolResult.Content != "file content" {
		t.Errorf("expected content 'file content', got %q", msg.ToolResult.Content)
	}
}

func TestErrorResult(t *testing.T) {
	tc := ToolCall{ID: "tc1", Name: "Bash", Input: `{"command": "ls"}`}
	r := ErrorResult(tc, "permission denied")
	if r.ToolCallID != "tc1" {
		t.Errorf("expected ToolCallID 'tc1', got %q", r.ToolCallID)
	}
	if r.ToolName != "Bash" {
		t.Errorf("expected ToolName 'Bash', got %q", r.ToolName)
	}
	if r.Content != "permission denied" {
		t.Errorf("expected content 'permission denied', got %q", r.Content)
	}
	if !r.IsError {
		t.Error("expected IsError true")
	}
}

func TestRoleStringConversion(t *testing.T) {
	// Verify typed Role is comparable with string literals
	if string(RoleUser) != "user" {
		t.Errorf("RoleUser should be 'user', got %q", RoleUser)
	}
	if string(RoleAssistant) != "assistant" {
		t.Errorf("RoleAssistant should be 'assistant', got %q", RoleAssistant)
	}
	if string(RoleToolResult) != "tool_result" {
		t.Errorf("RoleToolResult should be 'tool_result', got %q", RoleToolResult)
	}
}

func TestBuildConversationText(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "hello"},
		{Role: RoleAssistant, Content: "hi there", ToolCalls: []ToolCall{
			{ID: "tc1", Name: "Read"},
		}},
		{Role: RoleUser, ToolResult: &ToolResult{
			ToolCallID: "tc1", ToolName: "Read", Content: "file data",
		}},
	}

	text := BuildConversationText(msgs)
	if !strings.Contains(text, "User: hello") {
		t.Error("expected user message in output")
	}
	if !strings.Contains(text, "Assistant: hi there") {
		t.Error("expected assistant message in output")
	}
	if !strings.Contains(text, "[Tool Call: Read]") {
		t.Error("expected tool call in output")
	}
	if !strings.Contains(text, "[Tool Result: Read]") {
		t.Error("expected tool result in output")
	}
}

func TestBuildConversationTextTruncation(t *testing.T) {
	longContent := strings.Repeat("x", 600)
	msgs := []Message{
		{Role: RoleUser, ToolResult: &ToolResult{
			ToolCallID: "tc1", ToolName: "Read", Content: longContent,
		}},
	}

	text := BuildConversationText(msgs)
	if !strings.Contains(text, "...[truncated]") {
		t.Error("expected truncation marker for long tool result")
	}
}

func TestParseToolInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantLen int
	}{
		{"empty", "", false, 0},
		{"valid", `{"key": "value"}`, false, 1},
		{"invalid", `not json`, true, 0},
		{"whitespace", "  ", false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := ParseToolInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseToolInput() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && len(params) != tt.wantLen {
				t.Errorf("expected %d params, got %d", tt.wantLen, len(params))
			}
		})
	}
}

func TestNeedsCompaction(t *testing.T) {
	tests := []struct {
		name        string
		inputTokens int
		inputLimit  int
		want        bool
	}{
		{"zero limit", 100, 0, false},
		{"zero tokens", 0, 1000, false},
		{"below threshold", 500, 1000, false},
		{"at threshold", 950, 1000, true},
		{"above threshold", 960, 1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsCompaction(tt.inputTokens, tt.inputLimit)
			if got != tt.want {
				t.Errorf("NeedsCompaction(%d, %d) = %v, want %v", tt.inputTokens, tt.inputLimit, got, tt.want)
			}
		})
	}
}
