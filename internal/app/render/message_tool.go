package render

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
)

// ToolCallsParams holds the parameters for rendering tool calls.
type ToolCallsParams struct {
	ToolCalls         []message.ToolCall
	ToolCallsExpanded bool
	ResultMap         map[string]ToolResultData
	ParallelMode      bool
	ParallelResults   map[int]bool
	TaskProgress      map[int][]string
	PendingCalls      []message.ToolCall
	CurrentIdx        int
	SpinnerView       string
	TaskOwnerMap      map[string]string
	MDRenderer        *MDRenderer
	Width             int
}

// ToolResultData holds the data needed to render a tool result inline.
type ToolResultData struct {
	ToolName  string
	Content   string
	IsError   bool
	Expanded  bool
	ToolInput string
}

// RenderToolCalls renders the tool calls section of an assistant message.
func RenderToolCalls(params ToolCallsParams) string {
	var sb strings.Builder

	for _, tc := range params.ToolCalls {
		switch tc.Name {
		case tool.ToolTaskList, tool.ToolTaskCreate, tool.ToolTaskUpdate:
			continue
		}
		if tc.Name == tool.ToolAgent {
			label := FormatAgentLabel(tc.Input)
			_, hasResult := params.ResultMap[tc.ID]
			if hasResult {
				sb.WriteString(renderToolLine(label, params.Width) + "\n")
			} else {
				sb.WriteString(renderToolLineWithIcon(label, params.Width, params.SpinnerView))
				if !params.ToolCallsExpanded {
					sb.WriteString(ThinkingStyle.Render("  (ctrl+o to expand)"))
				}
				sb.WriteString("\n")
			}
			if params.ToolCallsExpanded && !hasResult {
				sb.WriteString(formatAgentDefinition(tc.Input))
			}
		} else if params.ToolCallsExpanded {
			toolLine := renderToolLine(tc.Name, params.Width)
			sb.WriteString(toolLine + "\n")
			var p map[string]any
			if err := json.Unmarshal([]byte(tc.Input), &p); err == nil {
				for k, v := range p {
					if s, ok := v.(string); ok {
						if len(s) > 80 {
							sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("%s:", k)) + "\n")
							sb.WriteString(ToolResultExpandedStyle.Render(s) + "\n")
						} else {
							sb.WriteString(ToolResultExpandedStyle.Render(fmt.Sprintf("%s: %s", k, s)) + "\n")
						}
					}
				}
			}
		} else {
			icon := toolCallIcon(tc, params.PendingCalls, params.CurrentIdx, params.ParallelMode, params.ParallelResults, params.SpinnerView)
			if tc.Name == tool.ToolTaskGet && params.TaskOwnerMap != nil {
				args := extractTaskGetDisplay(tc.Input, params.TaskOwnerMap)
				sb.WriteString(renderToolLineWithIcon(fmt.Sprintf("%s(%s)", tc.Name, args), params.Width, icon) + "\n")
			} else {
				args := ExtractToolArgs(tc.Input)
				sb.WriteString(renderToolLineWithIcon(fmt.Sprintf("%s(%s)", tc.Name, args), params.Width, icon) + "\n")
			}
		}

		if resultData, ok := params.ResultMap[tc.ID]; ok {
			resultData.ToolInput = tc.Input
			sb.WriteString(RenderToolResultInline(resultData, params.MDRenderer))
		} else if params.ParallelMode && tc.Name == tool.ToolAgent {
			sb.WriteString(RenderTaskProgressInline(tc, params.PendingCalls, params.ParallelResults, params.TaskProgress))
		}
	}

	return sb.String()
}

func toolCallIcon(tc message.ToolCall, pendingCalls []message.ToolCall, currentIdx int, parallelMode bool, parallelResults map[int]bool, spinnerView string) string {
	idx := -1
	for i, pending := range pendingCalls {
		if pending.ID == tc.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "● "
	}

	if parallelMode {
		if _, done := parallelResults[idx]; !done {
			return spinnerView + " "
		}
		return "● "
	}

	if idx == currentIdx {
		return spinnerView + " "
	}

	return "● "
}

// stripMarkdownHeading removes leading `#` markers from markdown headings.
func stripMarkdownHeading(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if !strings.HasPrefix(trimmed, "#") {
		return line
	}
	stripped := strings.TrimLeft(trimmed, "#")
	stripped = strings.TrimPrefix(stripped, " ")
	indent := line[:len(line)-len(trimmed)]
	return indent + stripped
}
