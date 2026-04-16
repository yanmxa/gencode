package session

import (
	"testing"

	"github.com/yanmxa/gencode/internal/message"
)

func Test_messagesToEntries_roundtrip(t *testing.T) {
	// Test that messagesToEntries -> EntriesToMessages roundtrips correctly.
	msgs := []message.Message{
		{Role: message.RoleUser, Content: "hello"},
		{Role: message.RoleAssistant, Content: "hi", Thinking: "let me think",
			ToolCalls: []message.ToolCall{{ID: "tc-1", Name: "Read", Input: `{"file_path":"/tmp/test"}`}}},
		{Role: message.RoleUser, ToolResult: &message.ToolResult{
			ToolCallID: "tc-1", ToolName: "Read", Content: "file contents",
		}},
		{Role: message.RoleAssistant, Content: "I see the file."},
	}

	entries := messagesToEntries(msgs)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Verify entry types
	if entries[0].Type != EntryUser {
		t.Errorf("entry[0] type: want user, got %s", entries[0].Type)
	}
	if entries[1].Type != EntryAssistant {
		t.Errorf("entry[1] type: want assistant, got %s", entries[1].Type)
	}
	if entries[2].Type != EntryUser {
		t.Errorf("entry[2] type: want user (tool_result), got %s", entries[2].Type)
	}

	// Round-trip back to messages
	restored := EntriesToMessages(entries)
	if len(restored) != 4 {
		t.Fatalf("expected 4 messages after roundtrip, got %d", len(restored))
	}
	if restored[0].Content != "hello" {
		t.Errorf("msg[0].Content: want 'hello', got %q", restored[0].Content)
	}
	if restored[1].Thinking != "let me think" {
		t.Errorf("msg[1].Thinking: want 'let me think', got %q", restored[1].Thinking)
	}
	if restored[2].ToolResult == nil {
		t.Fatal("msg[2].ToolResult should not be nil")
	}
	if restored[2].ToolResult.ToolCallID != "tc-1" {
		t.Errorf("msg[2].ToolResult.ToolCallID: want 'tc-1', got %q", restored[2].ToolResult.ToolCallID)
	}
	// Tool name should be resolved from the tool_use block
	if restored[2].ToolResult.ToolName != "Read" {
		t.Errorf("msg[2].ToolResult.ToolName: want 'Read', got %q", restored[2].ToolResult.ToolName)
	}
}

func Test_userContentToBlocks_preserveInlineImageOrder(t *testing.T) {
	blocks := userContentToBlocks(
		"这个图片说了什么 请说一下",
		"[Image #1] 这个图片说了什么 请说一下",
		[]message.ImageData{{MediaType: "image/png", Data: "abc"}},
	)

	if len(blocks) != 2 {
		t.Fatalf("expected image and text blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "image" {
		t.Fatalf("expected first block to be image, got %q", blocks[0].Type)
	}
	if blocks[1].Type != "text" || blocks[1].Text != " 这个图片说了什么 请说一下" {
		t.Fatalf("unexpected second block: %#v", blocks[1])
	}
}

func Test_extractUserContent_restoresDisplayContent(t *testing.T) {
	msgs := EntriesToMessages([]Entry{{
		Type: EntryUser,
		Message: &EntryMessage{
			Role: "user",
			Content: []ContentBlock{
				{Type: "text", Text: "前面 "},
				{Type: "image", Source: &ImageSource{Type: "base64", MediaType: "image/png", Data: "abc"}},
				{Type: "text", Text: " 后面"},
			},
		},
	}})

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "前面  后面" {
		t.Fatalf("unexpected content: %q", msgs[0].Content)
	}
	if msgs[0].DisplayContent != "前面 [Image #1] 后面" {
		t.Fatalf("unexpected display content: %q", msgs[0].DisplayContent)
	}
}
