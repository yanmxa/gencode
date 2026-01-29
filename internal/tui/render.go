package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)


func createMarkdownRenderer(width int) *glamour.TermRenderer {
	wrapWidth := max(width-4, minWrapWidth)

	var compactStyle ansi.StyleConfig
	if lipgloss.HasDarkBackground() {
		compactStyle = styles.DarkStyleConfig
	} else {
		compactStyle = styles.LightStyleConfig
	}

	uintPtr := func(u uint) *uint { return &u }
	compactStyle.Document.Margin = uintPtr(0)
	compactStyle.Paragraph.Margin = uintPtr(0)
	compactStyle.CodeBlock.Margin = uintPtr(0)

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStyles(compactStyle),
		glamour.WithWordWrap(wrapWidth),
	)
	return renderer
}

func (m model) renderWelcome() string {
	gradient := []lipgloss.Color{
		CurrentTheme.Primary,
		CurrentTheme.AI,
		CurrentTheme.Accent,
	}

	logoLines := []string{
		"   ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄",
		"   █                             █",
		"   █   ╋╋╋╋╋   ╋╋╋╋   ╋   ╋      █",
		"   █   ╋       ╋      ╋╋  ╋      █",
		"   █   ╋  ╋╋╋  ╋╋╋╋   ╋ ╋ ╋      █",
		"   █   ╋    ╋  ╋      ╋  ╋╋      █",
		"   █   ╋╋╋╋╋   ╋╋╋╋   ╋   ╋      █",
		"   █                             █",
		"   ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
	}

	subtitleStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
	hintStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDisabled)

	var sb strings.Builder
	sb.WriteString("\n")

	for i, line := range logoLines {
		colorIdx := i % len(gradient)
		style := lipgloss.NewStyle().Foreground(gradient[colorIdx])
		sb.WriteString(style.Render(line) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString("   " + subtitleStyle.Render("AI-powered coding assistant") + "\n")
	sb.WriteString("\n")
	sb.WriteString("   " + hintStyle.Render("Enter to send · Esc to stop · Shift+Tab mode · Ctrl+C exit") + "\n")

	return sb.String()
}

func (m model) renderModeStatus() string {
	var icon, label string
	var color lipgloss.Color

	switch m.operationMode {
	case modeAutoAccept:
		icon = "⏵⏵"
		label = " accept edits on"
		color = CurrentTheme.Success
	case modePlan:
		icon = "⏸"
		label = " plan mode on"
		color = CurrentTheme.Warning
	default:
		return ""
	}

	styledIcon := lipgloss.NewStyle().Foreground(color).Render(icon)
	styledLabel := lipgloss.NewStyle().Foreground(color).Render(label)
	hint := lipgloss.NewStyle().Foreground(CurrentTheme.Muted).Render("  shift+tab to toggle")

	return "  " + styledIcon + styledLabel + hint
}

func (m model) renderMessages() string {
	if len(m.messages) == 0 {
		return m.renderWelcome()
	}

	// Build a set of message indices to skip (tool results rendered inline with tool calls)
	skipIndices := make(map[int]bool)
	for i, msg := range m.messages {
		if msg.role == "assistant" && len(msg.toolCalls) > 0 {
			// Build set of ToolCall IDs for this assistant message
			toolCallIDs := make(map[string]bool)
			for _, tc := range msg.toolCalls {
				toolCallIDs[tc.ID] = true
			}
			// Mark subsequent tool result messages that match these IDs
			for j := i + 1; j < len(m.messages); j++ {
				nextMsg := m.messages[j]
				if nextMsg.toolResult == nil {
					break
				}
				if toolCallIDs[nextMsg.toolResult.ToolCallID] {
					skipIndices[j] = true
				}
			}
		}
	}

	var sb strings.Builder

	for i, msg := range m.messages {
		// Skip tool results that were rendered inline with their tool calls
		if skipIndices[i] {
			continue
		}

		if msg.toolResult == nil {
			sb.WriteString("\n")
		}

		switch msg.role {
		case "user":
			if msg.toolResult != nil {
				sb.WriteString(m.renderToolResult(msg))
			} else {
				sb.WriteString(m.renderUserMessage(msg))
			}
		case "system":
			sb.WriteString(m.renderSystemMessage(msg))
		case "permission":
			// Skip - rendered separately in View()
		default: // assistant
			sb.WriteString(m.renderAssistantMessage(msg, i, i == len(m.messages)-1))
		}
	}

	sb.WriteString(m.renderPendingToolSpinner())

	return sb.String()
}

func (m model) renderUserMessage(msg chatMessage) string {
	prompt := inputPromptStyle.Render("❯ ")
	content := userMsgStyle.Render(msg.content)
	return prompt + content + "\n"
}

func (m model) renderSystemMessage(msg chatMessage) string {
	return systemMsgStyle.Render(msg.content) + "\n"
}

func (m model) renderToolResult(msg chatMessage) string {
	return m.renderToolResultInline(msg)
}

func (m model) renderAssistantMessage(msg chatMessage, idx int, isLast bool) string {
	var sb strings.Builder
	aiIcon := aiPromptStyle.Render("◆ ")
	aiIndent := "  "

	if msg.content == "" && len(msg.toolCalls) == 0 && m.streaming {
		content := thinkingStyle.Render(m.spinner.View() + " Thinking...")
		sb.WriteString(aiIcon + content + "\n")
	} else if m.streaming && isLast && len(msg.toolCalls) == 0 {
		content := assistantMsgStyle.Render(msg.content + "▌")
		content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	} else if m.mdRenderer != nil && msg.content != "" {
		rendered, err := m.mdRenderer.Render(msg.content)
		var content string
		if err == nil {
			content = strings.TrimLeft(rendered, " \t\n")
			content = strings.TrimRight(content, " \t\n")
			blankLines := regexp.MustCompile(`\n\s*\n`)
			content = blankLines.ReplaceAllString(content, "\n")
		} else {
			content = msg.content
		}
		content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	} else if msg.content != "" {
		content := strings.ReplaceAll(msg.content, "\n", "\n"+aiIndent)
		sb.WriteString(aiIcon + content + "\n")
	}

	if len(msg.toolCalls) > 0 {
		sb.WriteString(m.renderToolCalls(msg, idx))
	}

	return sb.String()
}

func (m model) renderToolCalls(msg chatMessage, msgIdx int) string {
	var sb strings.Builder

	if msg.content != "" {
		sb.WriteString("\n")
	}

	// Build a map from ToolCallID to the corresponding toolResult message
	resultMap := make(map[string]chatMessage)
	for j := msgIdx + 1; j < len(m.messages); j++ {
		nextMsg := m.messages[j]
		if nextMsg.toolResult == nil {
			break
		}
		resultMap[nextMsg.toolResult.ToolCallID] = nextMsg
	}

	for _, tc := range msg.toolCalls {
		if msg.toolCallsExpanded {
			toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s", tc.Name))
			sb.WriteString(toolLine + "\n")
			var params map[string]any
			if err := json.Unmarshal([]byte(tc.Input), &params); err == nil {
				for k, v := range params {
					if s, ok := v.(string); ok {
						if len(s) > 80 {
							sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("%s:", k)) + "\n")
							sb.WriteString(toolResultExpandedStyle.Render(s) + "\n")
						} else {
							sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("%s: %s", k, s)) + "\n")
						}
					}
				}
			}
		} else {
			args := extractToolArgs(tc.Input)
			toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s(%s)", tc.Name, args))
			sb.WriteString(toolLine + "\n")
		}

		// Render the corresponding result inline if found
		if resultMsg, ok := resultMap[tc.ID]; ok {
			sb.WriteString(m.renderToolResultInline(resultMsg))
		}
	}

	return sb.String()
}

// renderToolResultInline renders a tool result inline (without leading newline)
func (m model) renderToolResultInline(msg chatMessage) string {
	toolName := msg.toolName
	if toolName == "" {
		toolName = "Tool"
	}

	sizeInfo := formatToolResultSize(toolName, msg.toolResult.Content)

	icon := "⎿"
	if msg.toolResult.IsError {
		icon = "✗"
	}

	var sb strings.Builder
	summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s → %s", icon, toolName, sizeInfo))
	sb.WriteString(summary + "\n")

	if msg.expanded || msg.toolResult.IsError {
		lines := strings.Split(msg.toolResult.Content, "\n")
		for _, line := range lines {
			sb.WriteString(toolResultExpandedStyle.Render(line) + "\n")
		}
	}

	return sb.String()
}

func (m model) renderPendingToolSpinner() string {
	interactivePromptActive := m.questionPrompt.IsActive() || (m.planPrompt != nil && m.planPrompt.IsActive())

	if m.buildingToolName != "" && !interactivePromptActive {
		var sb strings.Builder
		sb.WriteString("\n")
		toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s", m.buildingToolName))
		sb.WriteString(toolLine + "\n")

		desc := getToolExecutionDesc(m.buildingToolName)
		spinnerLine := thinkingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), desc))
		sb.WriteString(spinnerLine + "\n")
		return sb.String()
	}

	if m.pendingToolCalls != nil && m.pendingToolIdx < len(m.pendingToolCalls) && !interactivePromptActive {
		tc := m.pendingToolCalls[m.pendingToolIdx]
		var sb strings.Builder
		sb.WriteString("\n")
		toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s", tc.Name))
		sb.WriteString(toolLine + "\n")

		desc := getToolExecutionDesc(tc.Name)
		spinnerLine := thinkingStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), desc))
		sb.WriteString(spinnerLine + "\n")
		return sb.String()
	}

	return ""
}

func getToolExecutionDesc(toolName string) string {
	switch toolName {
	case "ExitPlanMode":
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
	default:
		return "Executing..."
	}
}

func extractToolArgs(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}

	if fp, ok := params["file_path"].(string); ok {
		return fp
	}
	if p, ok := params["pattern"].(string); ok {
		return p
	}
	if p, ok := params["path"].(string); ok {
		return p
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s, ok := params[k].(string); ok {
			if len(s) > 60 {
				return s[:60] + "..."
			}
			return s
		}
	}
	return ""
}

func formatToolResultSize(toolName, content string) string {
	switch toolName {
	case "WebFetch":
		size := len(content)
		if size >= 1024*1024 {
			return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
		}
		if size >= 1024 {
			return fmt.Sprintf("%.1f KB", float64(size)/1024)
		}
		return fmt.Sprintf("%d bytes", size)

	case "Write", "Edit":
		start := strings.Index(content, "(")
		if start == -1 {
			return "completed"
		}
		end := strings.Index(content[start:], ")")
		if end == -1 {
			return "completed"
		}
		return content[start+1 : start+end]

	default:
		trimmed := strings.TrimSuffix(content, "\n")
		if trimmed == "" {
			return "0 lines"
		}
		lineCount := strings.Count(trimmed, "\n") + 1
		return fmt.Sprintf("%d lines", lineCount)
	}
}

