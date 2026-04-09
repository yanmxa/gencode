package sessionui

import (
	"testing"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/session"
)

func TestUserContentBlocksPreserveInlineImageOrder(t *testing.T) {
	blocks := session.UserContentToBlocks(
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

func TestExtractUserContentRestoresDisplayContent(t *testing.T) {
	msgs := session.EntriesToMessages([]session.Entry{{
		Type: session.EntryUser,
		Message: &session.EntryMessage{
			Role: "user",
			Content: []session.ContentBlock{
				{Type: "text", Text: "前面 "},
				{Type: "image", Source: &session.ImageSource{Type: "base64", MediaType: "image/png", Data: "abc"}},
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
