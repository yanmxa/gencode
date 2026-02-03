package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool/permission"
)

// AgentPreview renders a preview of agent metadata for permission prompts
type AgentPreview struct {
	agentMeta *permission.AgentMetadata
}

// NewAgentPreview creates a new AgentPreview instance
func NewAgentPreview(meta *permission.AgentMetadata) *AgentPreview {
	return &AgentPreview{agentMeta: meta}
}

// Render renders the agent preview
func (p *AgentPreview) Render(width int) string {
	if p.agentMeta == nil {
		return ""
	}

	var sb strings.Builder

	// Styles
	nameStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)
	labelStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
	modeStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Success)

	// Agent name
	sb.WriteString("   ")
	sb.WriteString(nameStyle.Render(p.agentMeta.AgentName))
	if p.agentMeta.Background {
		sb.WriteString(" ")
		sb.WriteString(modeStyle.Render("[background]"))
	}
	sb.WriteString("\n")

	// Description
	if p.agentMeta.Description != "" {
		sb.WriteString("   ")
		sb.WriteString(dimStyle.Render(p.agentMeta.Description))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Model
	if p.agentMeta.Model != "" {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Model: "))
		sb.WriteString(dimStyle.Render(p.agentMeta.Model))
		sb.WriteString("\n")
	}

	// Permission mode
	sb.WriteString("   ")
	sb.WriteString(labelStyle.Render("Mode: "))
	modeLabel := p.formatPermissionMode(p.agentMeta.PermissionMode)
	sb.WriteString(dimStyle.Render(modeLabel))
	sb.WriteString("\n")

	// Tools
	if len(p.agentMeta.Tools) > 0 {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Tools: "))
		sb.WriteString(dimStyle.Render(strings.Join(p.agentMeta.Tools, ", ")))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Task prompt (truncated if too long)
	sb.WriteString("   ")
	sb.WriteString(labelStyle.Render("Task:"))
	sb.WriteString("\n")

	// Wrap and indent the prompt
	prompt := p.agentMeta.Prompt
	if len(prompt) > 500 {
		prompt = prompt[:500] + "..."
	}

	lines := strings.Split(prompt, "\n")
	for i, line := range lines {
		if i >= 10 {
			sb.WriteString("   ")
			sb.WriteString(dimStyle.Render("..."))
			sb.WriteString("\n")
			break
		}
		sb.WriteString("   ")
		if len(line) > width-6 {
			sb.WriteString(dimStyle.Render(line[:width-9] + "..."))
		} else {
			sb.WriteString(dimStyle.Render(line))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatPermissionMode formats the permission mode for display
func (p *AgentPreview) formatPermissionMode(mode string) string {
	switch mode {
	case "plan":
		return "Read-only (plan mode)"
	case "default":
		return "Standard permissions"
	case "acceptEdits":
		return "Auto-accept file edits"
	case "dontAsk":
		return "Autonomous (all permissions)"
	default:
		return mode
	}
}
