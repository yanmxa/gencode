package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool/permission"
)

// SkillPreview renders a preview of skill metadata for permission prompts
type SkillPreview struct {
	skillMeta *permission.SkillMetadata
}

// NewSkillPreview creates a new SkillPreview instance
func NewSkillPreview(meta *permission.SkillMetadata) *SkillPreview {
	return &SkillPreview{skillMeta: meta}
}

// Render renders the skill preview
func (p *SkillPreview) Render(width int) string {
	if p.skillMeta == nil {
		return ""
	}

	var sb strings.Builder

	// Skill name style
	nameStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)
	labelStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted)

	// Skill name
	sb.WriteString("   ")
	sb.WriteString(nameStyle.Render(p.skillMeta.SkillName))
	sb.WriteString("\n")

	// Description
	if p.skillMeta.Description != "" {
		sb.WriteString("   ")
		sb.WriteString(dimStyle.Render(p.skillMeta.Description))
		sb.WriteString("\n")
	}

	// Arguments if provided
	if p.skillMeta.Args != "" {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Args: "))
		sb.WriteString(dimStyle.Render(p.skillMeta.Args))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Resources summary
	var resources []string
	if p.skillMeta.ScriptCount > 0 {
		if p.skillMeta.ScriptCount == 1 {
			resources = append(resources, "1 script")
		} else {
			resources = append(resources, fmt.Sprintf("%d scripts", p.skillMeta.ScriptCount))
		}
	}
	if p.skillMeta.RefCount > 0 {
		if p.skillMeta.RefCount == 1 {
			resources = append(resources, "1 reference")
		} else {
			resources = append(resources, fmt.Sprintf("%d references", p.skillMeta.RefCount))
		}
	}

	if len(resources) > 0 {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Resources: "))
		sb.WriteString(dimStyle.Render(strings.Join(resources, ", ")))
		sb.WriteString("\n")
	}

	// List scripts if any
	if len(p.skillMeta.Scripts) > 0 && len(p.skillMeta.Scripts) <= 5 {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Scripts: "))
		sb.WriteString(dimStyle.Render(strings.Join(p.skillMeta.Scripts, ", ")))
		sb.WriteString("\n")
	}

	// List references if any
	if len(p.skillMeta.References) > 0 && len(p.skillMeta.References) <= 5 {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Refs: "))
		sb.WriteString(dimStyle.Render(strings.Join(p.skillMeta.References, ", ")))
		sb.WriteString("\n")
	}

	return sb.String()
}
