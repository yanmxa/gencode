package input

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// ApprovalModel manages the permission request UI with Claude Code style.
type ApprovalModel struct {
	active       bool
	request      *perm.PermissionRequest
	diffPreview  *approvalDiffPreview
	bashPreview  *approvalBashPreview
	skillPreview *approvalSkillPreview
	agentPreview *approvalAgentPreview
	width        int
	selectedIdx  int
}

// NewApproval creates a new ApprovalModel instance
func NewApproval() ApprovalModel {
	return ApprovalModel{
		selectedIdx: 0,
	}
}

func (p *ApprovalModel) setRequest(req *perm.PermissionRequest, width int) {
	p.active = true
	p.request = req
	p.width = width
	p.selectedIdx = 0

	if req.DiffMeta != nil {
		p.diffPreview = newApprovalDiffPreview(req.DiffMeta, req.FilePath)
	} else {
		p.diffPreview = nil
	}

	if req.BashMeta != nil {
		p.bashPreview = newApprovalBashPreview(req.BashMeta)
	} else {
		p.bashPreview = nil
	}

	if req.SkillMeta != nil {
		p.skillPreview = newApprovalSkillPreview(req.SkillMeta)
	} else {
		p.skillPreview = nil
	}

	if req.AgentMeta != nil {
		p.agentPreview = newApprovalAgentPreview(req.AgentMeta)
	} else {
		p.agentPreview = nil
	}
}

// Show displays the permission prompt with the given request.
func (p *ApprovalModel) Show(req *perm.PermissionRequest, width, height int) {
	p.setRequest(req, width)
}

// Hide hides the permission prompt
func (p *ApprovalModel) Hide() {
	p.active = false
	p.request = nil
	p.diffPreview = nil
	p.bashPreview = nil
	p.skillPreview = nil
	p.agentPreview = nil
}

// IsActive returns whether the prompt is visible
func (p *ApprovalModel) IsActive() bool {
	return p.active
}

// TogglePreview toggles the expand state of diff/bash previews.
func (p *ApprovalModel) TogglePreview() {
	if p.diffPreview != nil {
		p.diffPreview.toggleExpand()
	}
	if p.bashPreview != nil {
		p.bashPreview.toggleExpand()
	}
}

// GetRequest returns the current permission request
func (p *ApprovalModel) GetRequest() *perm.PermissionRequest {
	return p.request
}

// ApprovalRequestMsg is sent when a tool needs permission
type ApprovalRequestMsg struct {
	Request  *perm.PermissionRequest
	ToolCall any
}

// ApprovalResponseMsg is sent when the user responds to a permission request
type ApprovalResponseMsg struct {
	Approved bool
	AllowAll bool
	Persist  bool
	Request  *perm.PermissionRequest
}

// HandleKeypress handles keyboard input for the permission prompt.
// Returns (cmd, response): cmd for UI updates, response when user makes a decision.
func (p *ApprovalModel) HandleKeypress(msg tea.KeyMsg) (tea.Cmd, *ApprovalResponseMsg) {
	if !p.active {
		return nil, nil
	}

	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if p.selectedIdx > 0 {
			p.selectedIdx--
		}
		return nil, nil

	case tea.KeyDown, tea.KeyCtrlN:
		if p.selectedIdx < 3 {
			p.selectedIdx++
		}
		return nil, nil

	case tea.KeyEnter:
		return p.confirmSelection()

	case tea.KeyShiftTab:
		return p.respondFull(true, true, false)

	case tea.KeyCtrlO:
		if p.diffPreview != nil {
			p.diffPreview.toggleExpand()
		}
		if p.bashPreview != nil {
			p.bashPreview.toggleExpand()
		}
		return nil, nil

	case tea.KeyEsc, tea.KeyCtrlC:
		return p.respondFull(false, false, false)
	}

	switch msg.String() {
	case "1", "y", "Y":
		return p.respondFull(true, false, false)
	case "2":
		return p.respondFull(true, true, false)
	case "3":
		return p.respondFull(true, false, true)
	case "4", "n", "N":
		return p.respondFull(false, false, false)
	}

	return nil, nil
}

func (p *ApprovalModel) respondFull(approved, allowAll, persist bool) (tea.Cmd, *ApprovalResponseMsg) {
	req := p.request
	p.Hide()
	return nil, &ApprovalResponseMsg{Approved: approved, AllowAll: allowAll, Persist: persist, Request: req}
}

func (p *ApprovalModel) confirmSelection() (tea.Cmd, *ApprovalResponseMsg) {
	switch p.selectedIdx {
	case 0:
		return p.respondFull(true, false, false)
	case 1:
		return p.respondFull(true, true, false)
	case 2:
		return p.respondFull(true, false, true)
	case 3:
		return p.respondFull(false, false, false)
	}
	return nil, nil
}

// --- Approval style helpers ---

func approvalSeparatorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Separator)
}

func approvalQuestionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
}

func approvalSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success).Bold(true)
}

func approvalUnselectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
}

func approvalHintStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted).Italic(true)
}

func approvalFooterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
}

func approvalTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)
}

func (p *ApprovalModel) renderInline() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder
	contentWidth := p.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	title := p.getTitle()
	sb.WriteString(" ")
	sb.WriteString(approvalTitleStyle().Render(title))
	sb.WriteString("\n\n")

	if p.diffPreview != nil {
		sb.WriteString(p.diffPreview.render(contentWidth))
	} else if p.bashPreview != nil {
		sb.WriteString(p.bashPreview.render(contentWidth))
	} else if p.skillPreview != nil {
		sb.WriteString(p.skillPreview.render(contentWidth))
	} else if p.agentPreview != nil {
		sb.WriteString(p.agentPreview.render(contentWidth))
	}
	sb.WriteString("\n")

	sb.WriteString(" ")
	sb.WriteString(approvalQuestionStyle().Render("Do you want to proceed?"))
	sb.WriteString("\n")

	sb.WriteString(p.renderMenu())
	sb.WriteString("\n")

	footer := " Esc to cancel"
	hasExpandableContent := (p.diffPreview != nil && len(p.diffPreview.diffMeta.Lines) > approvalDefaultMaxVisibleLines) ||
		(p.bashPreview != nil && p.bashPreview.needsExpand())
	if hasExpandableContent {
		footer += " · Ctrl+O expand"
	}
	sb.WriteString(approvalFooterStyle().Render(footer))
	sb.WriteString("\n")

	solidSep := strings.Repeat("─", contentWidth)
	sb.WriteString(approvalSeparatorStyle().Render(solidSep))

	return sb.String()
}

func (p *ApprovalModel) getTitle() string {
	var title string
	switch p.request.ToolName {
	case "Edit":
		title = "Edit file"
	case "Write":
		title = "Write to file"
	case "Bash":
		title = "Bash command"
	case tool.ToolSkill:
		title = "Load skill"
	case tool.ToolAgent, tool.ToolContinueAgent, tool.ToolSendMessage:
		title = "Spawn agent"
	default:
		title = p.request.Description
	}

	if p.request.CallerAgent != "" {
		title = "@" + p.request.CallerAgent + " · " + title
	}
	return title
}

func (p *ApprovalModel) getAllSessionLabel() string {
	switch p.request.ToolName {
	case "Edit":
		return "Yes, allow all edits during this session"
	case "Write":
		return "Yes, allow all writes during this session"
	case "Bash":
		return "Yes, allow all commands during this session"
	case tool.ToolSkill:
		return "Yes, allow all skills during this session"
	case tool.ToolAgent, tool.ToolContinueAgent, tool.ToolSendMessage:
		return "Yes, allow all agents during this session"
	default:
		return "Yes, allow all during this session"
	}
}

func (p *ApprovalModel) getAlwaysAllowLabel() string {
	if p.request != nil && len(p.request.SuggestedRules) > 0 {
		return "Always allow: " + p.request.SuggestedRules[0]
	}
	return "Always allow"
}

func (p *ApprovalModel) renderMenu() string {
	var sb strings.Builder

	options := []struct {
		label string
		hint  string
	}{
		{"Yes", ""},
		{p.getAllSessionLabel(), "(shift+tab)"},
		{p.getAlwaysAllowLabel(), ""},
		{"No", ""},
	}

	for i, opt := range options {
		if i == p.selectedIdx {
			sb.WriteString(approvalSelectedStyle().Render(fmt.Sprintf(" ❯ %d. %s", i+1, opt.label)))
		} else {
			sb.WriteString(approvalUnselectedStyle().Render(fmt.Sprintf("   %d. %s", i+1, opt.label)))
		}
		if opt.hint != "" {
			sb.WriteString(" ")
			sb.WriteString(approvalHintStyle().Render(opt.hint))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Render renders the permission prompt (calls renderInline)
func (p *ApprovalModel) Render() string {
	return p.renderInline()
}

// --- Agent preview ---

type approvalAgentPreview struct {
	agentMeta *perm.AgentMetadata
}

func newApprovalAgentPreview(meta *perm.AgentMetadata) *approvalAgentPreview {
	return &approvalAgentPreview{agentMeta: meta}
}

func (p *approvalAgentPreview) render(width int) string {
	if p.agentMeta == nil {
		return ""
	}

	var sb strings.Builder

	nameStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
	labelStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	modeStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success)

	sb.WriteString("   ")
	sb.WriteString(nameStyle.Render(p.agentMeta.AgentName))
	if p.agentMeta.Background {
		sb.WriteString(" ")
		sb.WriteString(modeStyle.Render("[background]"))
	}
	sb.WriteString("\n")

	if p.agentMeta.Description != "" {
		sb.WriteString("   ")
		sb.WriteString(dimStyle.Render(p.agentMeta.Description))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	if p.agentMeta.Model != "" {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Model: "))
		sb.WriteString(dimStyle.Render(p.agentMeta.Model))
		sb.WriteString("\n")
	}

	sb.WriteString("   ")
	sb.WriteString(labelStyle.Render("Mode: "))
	modeLabel := approvalFormatPermissionMode(p.agentMeta.PermissionMode)
	sb.WriteString(dimStyle.Render(modeLabel))
	sb.WriteString("\n")

	if len(p.agentMeta.Tools) > 0 {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Tools: "))
		sb.WriteString(dimStyle.Render(strings.Join(p.agentMeta.Tools, ", ")))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

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

func approvalFormatPermissionMode(mode string) string {
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

// --- Skill preview ---

type approvalSkillPreview struct {
	skillMeta *perm.SkillMetadata
}

func newApprovalSkillPreview(meta *perm.SkillMetadata) *approvalSkillPreview {
	return &approvalSkillPreview{skillMeta: meta}
}

func (p *approvalSkillPreview) render(width int) string {
	if p.skillMeta == nil {
		return ""
	}

	var sb strings.Builder

	nameStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
	labelStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)

	sb.WriteString("   ")
	sb.WriteString(nameStyle.Render(p.skillMeta.SkillName))
	sb.WriteString("\n")

	if p.skillMeta.Description != "" {
		sb.WriteString("   ")
		sb.WriteString(dimStyle.Render(p.skillMeta.Description))
		sb.WriteString("\n")
	}

	if p.skillMeta.Args != "" {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Args: "))
		sb.WriteString(dimStyle.Render(p.skillMeta.Args))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

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

	if len(p.skillMeta.Scripts) > 0 && len(p.skillMeta.Scripts) <= 5 {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Scripts: "))
		sb.WriteString(dimStyle.Render(strings.Join(p.skillMeta.Scripts, ", ")))
		sb.WriteString("\n")
	}

	if len(p.skillMeta.References) > 0 && len(p.skillMeta.References) <= 5 {
		sb.WriteString("   ")
		sb.WriteString(labelStyle.Render("Refs: "))
		sb.WriteString(dimStyle.Render(strings.Join(p.skillMeta.References, ", ")))
		sb.WriteString("\n")
	}

	return sb.String()
}
