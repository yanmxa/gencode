package approval

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// agentPreview renders a preview of agent metadata for permission prompts.
type agentPreview struct {
	agentMeta *perm.AgentMetadata
}

func newAgentPreview(meta *perm.AgentMetadata) *agentPreview {
	return &agentPreview{agentMeta: meta}
}

// Render renders the agent preview.
func (p *agentPreview) Render(width int) string {
	if p.agentMeta == nil {
		return ""
	}

	var sb strings.Builder

	nameStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
	labelStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	modeStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success)

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

func (p *agentPreview) formatPermissionMode(mode string) string {
	switch mode {
	case "plan":
		return "Read-only (plan mode)"
	case "default":
		return "Standard permissions"
	case "acceptEdits":
		return "Auto-accept edits"
	case "dontAsk", "bypassPermissions":
		return "Autonomous (all permissions)"
	case "auto":
		return "Auto (determines best level)"
	default:
		return mode
	}
}
