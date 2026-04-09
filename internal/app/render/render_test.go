package render

import (
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/message"
)

func TestExtractIntField(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		prefix   string
		expected int
	}{
		{
			name:     "valid turns",
			content:  "Agent: Explore\nStatus: completed\nTurns: 12\nTokens: 1500",
			prefix:   "Turns: ",
			expected: 12,
		},
		{
			name:     "turns at start",
			content:  "Turns: 5\nOther info",
			prefix:   "Turns: ",
			expected: 5,
		},
		{
			name:     "large turns number",
			content:  "Some text\nTurns: 999\nMore text",
			prefix:   "Turns: ",
			expected: 999,
		},
		{
			name:     "no turns field",
			content:  "Agent: Explore\nStatus: completed",
			prefix:   "Turns: ",
			expected: 0,
		},
		{
			name:     "empty content",
			content:  "",
			prefix:   "Turns: ",
			expected: 0,
		},
		{
			name:     "turns with zero",
			content:  "Turns: 0\n",
			prefix:   "Turns: ",
			expected: 0,
		},
		{
			name:     "single digit",
			content:  "Turns: 1",
			prefix:   "Turns: ",
			expected: 1,
		},
		{
			name:     "turns followed by text",
			content:  "Turns: 42abc",
			prefix:   "Turns: ",
			expected: 42,
		},
		{
			name:     "extract tokens",
			content:  "Turns: 10\nTokens: 1500",
			prefix:   "Tokens: ",
			expected: 1500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractIntField(tt.content, tt.prefix)
			if result != tt.expected {
				t.Errorf("ExtractIntField(%q, %q) = %d, want %d", tt.content, tt.prefix, result, tt.expected)
			}
		})
	}
}

func TestExtractToolArgsPreservesFullCommand(t *testing.T) {
	input := `{"command":"cd /Users/myan/Workspace/ideas/gencode && git describe --tags --abbrev=0 2>/dev/null"}`
	got := ExtractToolArgs(input)
	if !strings.Contains(got, "git describe --tags --abbrev=0") {
		t.Fatalf("ExtractToolArgs() = %q, want full command", got)
	}
}

func TestRenderToolCallsUsesEightyPercentWidth(t *testing.T) {
	params := ToolCallsParams{
		ToolCalls: []message.ToolCall{{
			ID:    "tc-1",
			Name:  "Bash",
			Input: `{"command":"cd /Users/myan/Workspace/ideas/gencode && git describe --tags --abbrev=0 2>/dev/null"}`,
		}},
		ResultMap: map[string]ToolResultData{},
		Width:     100,
	}

	rendered := RenderToolCalls(params)
	if !strings.Contains(rendered, "git describe --tags --abbrev") {
		t.Fatalf("RenderToolCalls() = %q, want wider command preview", rendered)
	}
	if !strings.Contains(rendered, "...") {
		t.Fatalf("RenderToolCalls() = %q, want truncation at 80%% width", rendered)
	}
}

func TestRenderToolCallsShowsRunningStateForPendingBash(t *testing.T) {
	params := ToolCallsParams{
		ToolCalls: []message.ToolCall{{
			ID:    "tc-1",
			Name:  "Bash",
			Input: `{"command":"find /Users/myan -name test"}`,
		}},
		ResultMap: map[string]ToolResultData{},
		PendingCalls: []message.ToolCall{{
			ID:    "tc-1",
			Name:  "Bash",
			Input: `{"command":"find /Users/myan -name test"}`,
		}},
		CurrentIdx:  0,
		SpinnerView: "⋯",
		Width:       100,
	}

	rendered := RenderToolCalls(params)
	if !strings.Contains(rendered, "⋯ Bash(find /Users/myan -name test)") {
		t.Fatalf("RenderToolCalls() = %q, want spinner on the main tool line", rendered)
	}
	if strings.Contains(rendered, "running...") {
		t.Fatalf("RenderToolCalls() = %q, should not add extra running text", rendered)
	}
}

func TestRenderToolCallsShowsGapForPendingAgent(t *testing.T) {
	params := ToolCallsParams{
		ToolCalls: []message.ToolCall{{
			ID:    "tc-1",
			Name:  "Agent",
			Input: `{"subagent_type":"Explore","description":"HA code structure","prompt":"Inspect the codebase"}`,
		}},
		ResultMap: map[string]ToolResultData{},
		PendingCalls: []message.ToolCall{{
			ID:    "tc-1",
			Name:  "Agent",
			Input: `{"subagent_type":"Explore","description":"HA code structure","prompt":"Inspect the codebase"}`,
		}},
		CurrentIdx:  0,
		SpinnerView: "◓",
		Width:       100,
	}

	rendered := RenderToolCalls(params)
	if !strings.Contains(rendered, "◓ Agent: Explore HA code structure") {
		t.Fatalf("RenderToolCalls() = %q, want a single visible gap before explicit agent label", rendered)
	}
}

func TestFormatToolResultSizeUsesNoOutputForEmptyContent(t *testing.T) {
	if got := FormatToolResultSize("Bash", ""); got != "no output" {
		t.Fatalf("FormatToolResultSize() = %q, want %q", got, "no output")
	}
}

func TestRenderPlanForScrollbackDoesNotInjectTitle(t *testing.T) {
	plan := "# Context\nBody"
	rendered := RenderPlanForScrollback(plan, nil)
	if rendered != plan {
		t.Fatalf("RenderPlanForScrollback() = %q, want raw plan content", rendered)
	}
	if strings.Contains(rendered, "Implementation Plan") {
		t.Fatalf("RenderPlanForScrollback() = %q, should not inject a title", rendered)
	}
}

func TestRenderTaskOutputResultInlineShowsErrorText(t *testing.T) {
	rendered := RenderTaskOutputResultInline(ToolResultData{
		ToolName: "TaskOutput",
		IsError:  true,
		Error:    "task not found: 10f7b381",
	})

	if !strings.Contains(rendered, "TaskOutput → Error") {
		t.Fatalf("expected TaskOutput error header, got %q", rendered)
	}
	if !strings.Contains(rendered, "task not found: 10f7b381") {
		t.Fatalf("expected TaskOutput error text, got %q", rendered)
	}
}
