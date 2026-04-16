package render

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
)

var inlineImageTokenPattern = regexp.MustCompile(`\[Image #\d+\]`)

// RenderUserMessage renders a user message with prompt and optional images.
func RenderUserMessage(content, displayContent string, images []message.ImageData, mdRenderer *MDRenderer, width int) string {
	var sb strings.Builder
	prompt := InputPromptStyle.Render("❯ ")
	if displayContent == "" {
		displayContent = content
	}

	if len(images) > 0 && inlineImageTokenPattern.MatchString(displayContent) {
		sb.WriteString(lipgloss.JoinHorizontal(
			lipgloss.Top,
			prompt,
			userMsgStyle.Render(styleInlineImageTokens(displayContent)),
		) + "\n")
		return sb.String()
	}

	if len(images) > 0 {
		imgParts := make([]string, 0, len(images))
		for i := range images {
			imgParts = append(imgParts, PendingImageStyle.Render(fmt.Sprintf("[Image #%d]", i+1)))
		}
		imageLabel := strings.Join(imgParts, " ")
		if content != "" {
			sb.WriteString(prompt + imageLabel + " " + userMsgStyle.Render(content) + "\n")
		} else {
			sb.WriteString(prompt + imageLabel + "\n")
		}
	} else if content != "" {
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, prompt, userMsgStyle.Render(content)) + "\n")
	}

	return sb.String()
}

func styleInlineImageTokens(content string) string {
	return inlineImageTokenPattern.ReplaceAllStringFunc(content, func(token string) string {
		return PendingImageStyle.Render(token)
	})
}

// AssistantParams holds the parameters for rendering an assistant message.
type AssistantParams struct {
	Content           string
	Thinking          string
	ToolCalls         []message.ToolCall
	ToolCallsExpanded bool
	StreamActive      bool
	IsLast            bool
	SpinnerView       string
	MDRenderer        *MDRenderer
	Width             int
	ExecutingTool     string
}

// RenderAssistantMessage renders an assistant message with thinking, content, and tool calls.
func RenderAssistantMessage(params AssistantParams) string {
	var sb strings.Builder
	aiIcon := aiPromptStyle.Render("● ")
	if params.StreamActive && params.IsLast {
		aiIcon = aiPromptStyle.Render(params.SpinnerView + " ")
	}

	if params.Thinking != "" {
		wrapWidth := max(params.Width-2, minWrapWidth)
		wrapped := lipgloss.NewStyle().Width(wrapWidth).Render(params.Thinking)
		var lines []string
		for _, line := range strings.Split(wrapped, "\n") {
			if strings.TrimSpace(line) != "" {
				if lines == nil {
					lines = make([]string, 0, 8)
				}
				lines = append(lines, ThinkingStyle.Render(line))
			}
		}
		thinkingIcon := ThinkingStyle.Render("✦ ")
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, thinkingIcon, strings.Join(lines, "\n")) + "\n\n")
	}

	content := formatAssistantContent(params)
	if content != "" {
		sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, aiIcon, content) + "\n")
	}

	return sb.String()
}

// formatAssistantContent formats the assistant message content based on streaming state.
func formatAssistantContent(params AssistantParams) string {
	if params.Content == "" && len(params.ToolCalls) == 0 && params.StreamActive && params.Thinking == "" {
		if params.ExecutingTool != "" {
			return ThinkingStyle.Render(getToolExecutionDesc(params.ExecutingTool))
		}
		return ThinkingStyle.Render("Thinking...")
	}

	if params.StreamActive && params.IsLast && len(params.ToolCalls) == 0 {
		return assistantMsgStyle.Render(params.Content + "▌")
	}

	if params.Content == "" {
		return ""
	}

	if params.MDRenderer != nil {
		return renderMarkdownContent(params.MDRenderer, params.Content)
	}

	return params.Content
}

// renderMarkdownContent renders content through the markdown renderer.
func renderMarkdownContent(mdRenderer *MDRenderer, content string) string {
	rendered, err := mdRenderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimSpace(rendered)
}

// getToolExecutionDesc returns a human-readable description for a tool being executed.
func getToolExecutionDesc(toolName string) string {
	switch toolName {
	case tool.ToolExitPlanMode:
		return "Preparing implementation plan..."
	case "Read":
		return "Reading file..."
	case "Write":
		return "Writing file..."
	case "Edit":
		return "Editing file..."
	case "Bash":
		return "Executing command..."
	case "Glob":
		return "Finding files..."
	case "Grep":
		return "Searching files..."
	case "WebFetch":
		return "Fetching web content..."
	case "WebSearch":
		return "Searching the web..."
	case "AskUserQuestion":
		return "Preparing question..."
	case tool.ToolSkill:
		return "Loading skill..."
	default:
		return "Executing..."
	}
}

// RenderSystemMessage renders a system/notice message.
func RenderSystemMessage(content string) string {
	return systemMsgStyle.Render(content) + "\n"
}
