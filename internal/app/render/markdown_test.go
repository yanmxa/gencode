package render

import (
	"regexp"
	"strings"
	"testing"
)

// stripANSI removes ANSI escape sequences from a string.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func TestMDRenderer_Heading(t *testing.T) {
	r := NewMDRenderer(80)

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{"h1", "# Hello", "Hello"},
		{"h2", "## World", "World"},
		{"h3", "### Details", "Details"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := r.Render(tt.input)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			if !strings.Contains(out, tt.contains) {
				t.Errorf("output %q should contain %q", out, tt.contains)
			}
		})
	}
}

func TestMDRenderer_Emphasis(t *testing.T) {
	r := NewMDRenderer(80)

	t.Run("bold", func(t *testing.T) {
		out, err := r.Render("**bold text**")
		if err != nil {
			t.Fatalf("Render error: %v", err)
		}
		plain := stripANSI(out)
		if !strings.Contains(plain, "bold text") {
			t.Errorf("output should contain 'bold text', got:\n%s", plain)
		}
	})

	t.Run("italic", func(t *testing.T) {
		out, err := r.Render("*italic text*")
		if err != nil {
			t.Fatalf("Render error: %v", err)
		}
		plain := stripANSI(out)
		if !strings.Contains(plain, "italic text") {
			t.Errorf("output should contain 'italic text', got:\n%s", plain)
		}
	})
}

func TestMDRenderer_CodeSpan(t *testing.T) {
	r := NewMDRenderer(80)

	out, err := r.Render("Use `fmt.Println` here")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(out, "fmt.Println") {
		t.Errorf("output %q should contain 'fmt.Println'", out)
	}
}

func TestMDRenderer_FencedCodeBlock(t *testing.T) {
	r := NewMDRenderer(80)

	input := "```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "func main()") {
		t.Errorf("output should contain 'func main()', got:\n%s", plain)
	}
	// Code block should be padded for visual distinction
	for line := range strings.SplitSeq(plain, "\n") {
		if strings.Contains(line, "func") {
			if !strings.HasPrefix(line, " ") {
				t.Errorf("code line should be padded: %q", line)
			}
			break
		}
	}
}

func TestMDRenderer_UnorderedList(t *testing.T) {
	r := NewMDRenderer(80)

	input := "- item one\n- item two\n- item three"
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "item one") {
		t.Errorf("output should contain 'item one', got:\n%s", plain)
	}
	if !strings.Contains(plain, "item two") {
		t.Errorf("output should contain 'item two', got:\n%s", plain)
	}
	if !strings.Contains(plain, "•") {
		t.Errorf("output should contain bullet character '•'")
	}
}

func TestMDRenderer_OrderedList(t *testing.T) {
	r := NewMDRenderer(80)

	input := "1. first\n2. second\n3. third"
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "first") {
		t.Errorf("output should contain 'first', got:\n%s", plain)
	}
	if !strings.Contains(plain, "1.") {
		t.Errorf("output should contain '1.'")
	}
}

func TestMDRenderer_Link(t *testing.T) {
	r := NewMDRenderer(80)

	input := "Visit [Go](https://golang.org) for info"
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "Go") {
		t.Errorf("output should contain link text 'Go', got:\n%s", plain)
	}
}

func TestMDRenderer_ThematicBreak(t *testing.T) {
	r := NewMDRenderer(80)

	input := "above\n\n---\n\nbelow"
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "above") || !strings.Contains(plain, "below") {
		t.Errorf("output should contain text above and below the rule, got:\n%s", plain)
	}
	if !strings.Contains(plain, "---") && !strings.Contains(plain, "─") {
		t.Errorf("output should contain horizontal rule, got:\n%s", plain)
	}
}

func TestMDRenderer_Blockquote(t *testing.T) {
	r := NewMDRenderer(80)

	input := "> This is a quote"
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "This is a quote") {
		t.Errorf("output should contain quote text, got:\n%s", plain)
	}
}

func TestMDRenderer_Paragraph(t *testing.T) {
	r := NewMDRenderer(80)

	input := "Hello world, this is a test paragraph."
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(out, "Hello world") {
		t.Errorf("output should contain paragraph text")
	}
}

func TestMDRenderer_WordWrap(t *testing.T) {
	r := NewMDRenderer(40) // narrow width

	input := "This is a long paragraph that should wrap at the specified width boundary for proper terminal display."
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Errorf("expected word wrap to produce multiple lines, got %d", len(lines))
	}
}

func TestMDRenderer_MixedContent(t *testing.T) {
	r := NewMDRenderer(80)

	input := `# Title

Some **bold** and *italic* text with ` + "`code`" + `.

- item 1
- item 2

` + "```go\nfmt.Println(\"hi\")\n```"

	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	plain := stripANSI(out)
	checks := []string{"Title", "bold", "italic", "code", "item 1", "item 2", "Println"}
	for _, check := range checks {
		if !strings.Contains(plain, check) {
			t.Errorf("mixed content output should contain %q, got:\n%s", check, plain)
		}
	}
}

func TestRenderMarkdownContent(t *testing.T) {
	r := NewMDRenderer(80)

	result := RenderMarkdownContent(r, "# Hello\n\nWorld")
	if !strings.Contains(result, "Hello") {
		t.Errorf("result should contain 'Hello'")
	}
	if !strings.Contains(result, "World") {
		t.Errorf("result should contain 'World'")
	}
	// Should be trimmed
	if strings.HasPrefix(result, "\n") || strings.HasSuffix(result, "\n") {
		t.Errorf("result should be trimmed, got: %q", result)
	}
}

func TestMDRenderer_Table(t *testing.T) {
	r := NewMDRenderer(80)

	input := "| Name | Value |\n|------|-------|\n| foo  | bar   |\n| baz  | qux   |"
	out, err := r.Render(input)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	plain := stripANSI(out)

	// Should contain table content
	if !strings.Contains(plain, "Name") {
		t.Errorf("table should contain 'Name', got:\n%s", plain)
	}
	if !strings.Contains(plain, "foo") {
		t.Errorf("table should contain 'foo', got:\n%s", plain)
	}
	// Should have internal separators
	if !strings.Contains(plain, "│") {
		t.Errorf("table should have column separators │, got:\n%s", plain)
	}
	if !strings.Contains(plain, "─") {
		t.Errorf("table should have row separators ─, got:\n%s", plain)
	}
}
