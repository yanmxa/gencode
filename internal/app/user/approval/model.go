package approval

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// Model manages the permission request UI with Claude Code style.
type Model struct {
	active       bool
	request      *perm.PermissionRequest
	diffPreview  *diffPreview
	bashPreview  *bashPreview
	skillPreview *skillPreview
	agentPreview *agentPreview
	width        int
	selectedIdx  int // Current menu selection (0=Yes, 1=Yes all, 2=No)
}

// New creates a new Model instance
func New() *Model {
	return &Model{
		selectedIdx: 0, // Default to "Yes"
	}
}

func (p *Model) setRequest(req *perm.PermissionRequest, width int) {
	p.active = true
	p.request = req
	p.width = width
	p.selectedIdx = 0 // Reset to "Yes"

	if req.DiffMeta != nil {
		p.diffPreview = newDiffPreview(req.DiffMeta, req.FilePath)
	} else {
		p.diffPreview = nil
	}

	if req.BashMeta != nil {
		p.bashPreview = newBashPreview(req.BashMeta)
	} else {
		p.bashPreview = nil
	}

	if req.SkillMeta != nil {
		p.skillPreview = newSkillPreview(req.SkillMeta)
	} else {
		p.skillPreview = nil
	}

	if req.AgentMeta != nil {
		p.agentPreview = newAgentPreview(req.AgentMeta)
	} else {
		p.agentPreview = nil
	}
}

// Show displays the permission prompt with the given request.
func (p *Model) Show(req *perm.PermissionRequest, width, height int) {
	p.setRequest(req, width)
}

// Hide hides the permission prompt
func (p *Model) Hide() {
	p.active = false
	p.request = nil
	p.diffPreview = nil
	p.bashPreview = nil
	p.skillPreview = nil
	p.agentPreview = nil
}

// IsActive returns whether the prompt is visible
func (p *Model) IsActive() bool {
	return p.active
}

// TogglePreview toggles the expand state of diff/bash previews.
func (p *Model) TogglePreview() {
	if p.diffPreview != nil {
		p.diffPreview.toggleExpand()
	}
	if p.bashPreview != nil {
		p.bashPreview.toggleExpand()
	}
}

// GetRequest returns the current permission request
func (p *Model) GetRequest() *perm.PermissionRequest {
	return p.request
}

// Message types for permission responses
type (
	// RequestMsg is sent when a tool needs permission
	RequestMsg struct {
		Request  *perm.PermissionRequest
		ToolCall any // The original tool call
	}

	// ResponseMsg is sent when the user responds to a permission request
	ResponseMsg struct {
		Approved bool
		AllowAll bool // True if user selected "allow all during session"
		Persist  bool // True if user selected "always allow" (persist to settings)
		Request  *perm.PermissionRequest
	}
)

// HandleKeypress handles keyboard input for the permission prompt.
// Returns (cmd, response): cmd for UI updates, response when user makes a decision.
func (p *Model) HandleKeypress(msg tea.KeyMsg) (tea.Cmd, *ResponseMsg) {
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

// respondFull creates a response with all options and hides the prompt.
func (p *Model) respondFull(approved, allowAll, persist bool) (tea.Cmd, *ResponseMsg) {
	req := p.request
	p.Hide()
	return nil, &ResponseMsg{Approved: approved, AllowAll: allowAll, Persist: persist, Request: req}
}

// confirmSelection confirms the currently selected menu option.
func (p *Model) confirmSelection() (tea.Cmd, *ResponseMsg) {
	switch p.selectedIdx {
	case 0: // Yes
		return p.respondFull(true, false, false)
	case 1: // Allow all during session
		return p.respondFull(true, true, false)
	case 2: // Always allow (persist)
		return p.respondFull(true, false, true)
	case 3: // No
		return p.respondFull(false, false, false)
	}
	return nil, nil
}

// Permission prompt styles - use functions to get current theme dynamically
// This ensures styles update when theme changes (same pattern as question prompt)
func getSeparatorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Separator)
}

func getQuestionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
}

func getSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success).Bold(true)
}

func getUnselectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
}

func getHintStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted).Italic(true)
}

func getFooterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
}

func getTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)
}

// renderInline renders the permission prompt inline with Claude Code style
func (p *Model) renderInline() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder
	contentWidth := p.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Note: separator line is added by View() in app.go, not here

	// Title with Accent color (e.g., "Bash command", "Edit file")
	title := p.getTitle()
	sb.WriteString(" ")
	sb.WriteString(getTitleStyle().Render(title))
	sb.WriteString("\n\n")

	// Diff preview, Bash preview, Skill preview, Agent preview, or content preview (no dotted separators)
	if p.diffPreview != nil {
		sb.WriteString(p.diffPreview.render(contentWidth))
	} else if p.bashPreview != nil {
		sb.WriteString(p.bashPreview.render(contentWidth))
	} else if p.skillPreview != nil {
		sb.WriteString(p.skillPreview.Render(contentWidth))
	} else if p.agentPreview != nil {
		sb.WriteString(p.agentPreview.Render(contentWidth))
	}
	sb.WriteString("\n")

	// Question
	sb.WriteString(" ")
	sb.WriteString(getQuestionStyle().Render("Do you want to proceed?"))
	sb.WriteString("\n")

	// Menu options
	sb.WriteString(p.renderMenu())
	sb.WriteString("\n")

	// Simplified footer
	footer := " Esc to cancel"
	hasExpandableContent := (p.diffPreview != nil && len(p.diffPreview.diffMeta.Lines) > defaultMaxVisibleLines) ||
		(p.bashPreview != nil && p.bashPreview.needsExpand())
	if hasExpandableContent {
		footer += " · Ctrl+O expand"
	}
	sb.WriteString(getFooterStyle().Render(footer))
	sb.WriteString("\n")

	// Bottom separator line
	solidSep := strings.Repeat("─", contentWidth)
	sb.WriteString(getSeparatorStyle().Render(solidSep))

	return sb.String()
}

// getTitle returns a simple action title (used with Accent color).
// When a team agent triggers the permission, the agent name is shown as a prefix.
func (p *Model) getTitle() string {
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

// getAllSessionLabel returns the "allow all" label for this tool
func (p *Model) getAllSessionLabel() string {
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

// getAlwaysAllowLabel returns the "always allow" label, showing the suggested rule if available.
func (p *Model) getAlwaysAllowLabel() string {
	if p.request != nil && len(p.request.SuggestedRules) > 0 {
		return "Always allow: " + p.request.SuggestedRules[0]
	}
	return "Always allow"
}

// renderMenu renders the selection menu
func (p *Model) renderMenu() string {
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
			sb.WriteString(getSelectedStyle().Render(fmt.Sprintf(" ❯ %d. %s", i+1, opt.label)))
		} else {
			sb.WriteString(getUnselectedStyle().Render(fmt.Sprintf("   %d. %s", i+1, opt.label)))
		}
		if opt.hint != "" {
			sb.WriteString(" ")
			sb.WriteString(getHintStyle().Render(opt.hint))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Render renders the permission prompt (calls renderInline)
func (p *Model) Render() string {
	return p.renderInline()
}
