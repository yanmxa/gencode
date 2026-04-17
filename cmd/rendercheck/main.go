package main

import (
	"fmt"
	"strings"

	appoutput "github.com/yanmxa/gencode/internal/app/output"
)

func main() {
	md := appoutput.NewMDRenderer(80)

	tests := []struct {
		name    string
		content string
	}{
		{"Simple paragraph", "This is a simple paragraph with some text."},
		{"Two paragraphs", "First paragraph.\n\nSecond paragraph."},
		{"Heading + paragraph", "## Title\n\nSome content here."},
		{"List items", "Here are items:\n\n- Item 1\n- Item 2\n- Item 3"},
		{"Ordered list", "Steps:\n\n1. First step\n2. Second step\n3. Third step"},
		{"Code block", "Here is code:\n\n```go\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```\n\nAfter code."},
		{"Inline code", "Use `fmt.Println` to print."},
		{"Nested list", "- Parent 1\n  - Child 1a\n  - Child 1b\n- Parent 2"},
		{"Blockquote", "> This is a quote\n> spanning multiple lines"},
		{"Bold and italic", "This is **bold** and *italic* text."},
		{"CJK text", "这是一段中文文本，\n用于测试换行效果。"},
		{"Heading + list + code", "## Summary\n\nKey changes:\n\n- Added `foo` function\n- Fixed bug in `bar`\n\n```go\nfunc foo() {}\n```"},
		{"Multi heading", "## Section 1\n\nContent 1.\n\n## Section 2\n\nContent 2."},
		{"List then paragraph", "- Item 1\n- Item 2\n\nSome text after list."},
		{"Paragraph then list", "Some text before list.\n\n- Item 1\n- Item 2"},
	}

	for _, tt := range tests {
		fmt.Printf("=== %s ===\n", tt.name)
		result, err := md.Render(tt.content)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		lines := strings.Split(result, "\n")
		for i, line := range lines {
			fmt.Printf("%2d|%s|\n", i, line)
		}
		fmt.Println()
	}
}
