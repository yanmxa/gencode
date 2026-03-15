package approval

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	appskill "github.com/yanmxa/gencode/internal/app/skill"
	"github.com/yanmxa/gencode/internal/tool/permission"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// Model manages the permission request UI with Claude Code style.
type Model struct {
	active       bool
	request      *permission.PermissionRequest
	diffPreview  *DiffPreview
	bashPreview  *BashPreview
	skillPreview *appskill.Preview
	agentPreview *appagent.Preview
	width        int
	selectedIdx  int // Current menu selection (0=Yes, 1=Yes all, 2=No)
}

// New creates a new Model instance
func New() *Model {
	return &Model{
		selectedIdx: 0, // Default to "Yes"
	}
}

// Show displays the permission prompt with the given request
func (p *Model) Show(req *permission.PermissionRequest, width, height int) {
	p.active = true
	p.request = req
	p.width = width
	p.selectedIdx = 0 // Reset to "Yes"

	if req.DiffMeta != nil {
		p.diffPreview = NewDiffPreview(req.DiffMeta, req.FilePath)
	} else {
		p.diffPreview = nil
	}

	if req.BashMeta != nil {
		p.bashPreview = NewBashPreview(req.BashMeta)
	} else {
		p.bashPreview = nil
	}

	if req.SkillMeta != nil {
		p.skillPreview = appskill.NewPreview(req.SkillMeta)
	} else {
		p.skillPreview = nil
	}

	if req.AgentMeta != nil {
		p.agentPreview = appagent.NewPreview(req.AgentMeta)
	} else {
		p.agentPreview = nil
	}
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
		p.diffPreview.ToggleExpand()
	}
	if p.bashPreview != nil {
		p.bashPreview.ToggleExpand()
	}
}

// GetRequest returns the current permission request
func (p *Model) GetRequest() *permission.PermissionRequest {
	return p.request
}

// Message types for permission responses
type (
	// RequestMsg is sent when a tool needs permission
	RequestMsg struct {
		Request  *permission.PermissionRequest
		ToolCall interface{} // The original tool call
	}

	// ResponseMsg is sent when the user responds to a permission request
	ResponseMsg struct {
		Approved bool
		AllowAll bool // True if user selected "allow all during session"
		Request  *permission.PermissionRequest
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
		if p.selectedIdx < 2 {
			p.selectedIdx++
		}
		return nil, nil

	case tea.KeyEnter:
		return p.confirmSelection()

	case tea.KeyShiftTab:
		return p.respond(true, true)

	case tea.KeyCtrlO:
		if p.diffPreview != nil {
			p.diffPreview.ToggleExpand()
		}
		if p.bashPreview != nil {
			p.bashPreview.ToggleExpand()
		}
		return nil, nil

	case tea.KeyEsc, tea.KeyCtrlC:
		return p.respond(false, false)
	}

	switch msg.String() {
	case "1", "y", "Y":
		return p.respond(true, false)
	case "2":
		return p.respond(true, true)
	case "3", "n", "N":
		return p.respond(false, false)
	}

	return nil, nil
}

// respond creates a response and hides the prompt.
func (p *Model) respond(approved, allowAll bool) (tea.Cmd, *ResponseMsg) {
	req := p.request
	p.Hide()
	return nil, &ResponseMsg{Approved: approved, AllowAll: allowAll, Request: req}
}

// confirmSelection confirms the currently selected menu option.
func (p *Model) confirmSelection() (tea.Cmd, *ResponseMsg) {
	switch p.selectedIdx {
	case 0:
		return p.respond(true, false)
	case 1:
		return p.respond(true, true)
	case 2:
		return p.respond(false, false)
	}
	return nil, nil
}

// Permission prompt styles - use functions to get current theme dynamically
// This ensures styles update when theme changes (same pattern as question prompt)
func getSeparatorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Separator)
}

func getQuestionStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
}

func getSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success).Bold(true)
}

func getUnselectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
}

func getHintStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted).Italic(true)
}

func getFooterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
}

func getTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Primary).Bold(true)
}

// RenderInline renders the permission prompt inline with Claude Code style
func (p *Model) RenderInline() string {
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
		sb.WriteString(p.diffPreview.Render(contentWidth))
	} else if p.bashPreview != nil {
		sb.WriteString(p.bashPreview.Render(contentWidth))
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
	hasExpandableContent := (p.diffPreview != nil && len(p.diffPreview.DiffMeta().Lines) > DefaultMaxVisibleLines) ||
		(p.bashPreview != nil && p.bashPreview.NeedsExpand())
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
	case "Skill":
		title = "Load skill"
	case "Agent":
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
	case "Skill":
		return "Yes, allow all skills during this session"
	case "Agent":
		return "Yes, allow all agents during this session"
	default:
		return "Yes, allow all during this session"
	}
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

// Render renders the permission prompt (calls RenderInline)
func (p *Model) Render() string {
	return p.RenderInline()
}
