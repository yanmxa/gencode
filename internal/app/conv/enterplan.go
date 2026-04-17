package conv

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/tool"
)

// EnterPlanPrompt manages the enter plan mode confirmation UI
type EnterPlanPrompt struct {
	active      bool
	request     *tool.EnterPlanRequest
	width       int
	selectedIdx int
}

// NewEnterPlanPrompt creates a new EnterPlanPrompt
func NewEnterPlanPrompt() *EnterPlanPrompt {
	return &EnterPlanPrompt{}
}

// Show displays the enter plan prompt
func (p *EnterPlanPrompt) Show(req *tool.EnterPlanRequest, width int) {
	p.active = true
	p.request = req
	p.width = width
	p.selectedIdx = 0
}

// Hide hides the prompt
func (p *EnterPlanPrompt) Hide() {
	p.active = false
	p.request = nil
}

// IsActive returns whether the prompt is visible
func (p *EnterPlanPrompt) IsActive() bool {
	return p.active
}

// HandleKeypress handles keyboard input.
func (p *EnterPlanPrompt) HandleKeypress(msg tea.KeyMsg) (tea.Cmd, *EnterPlanResponseMsg) {
	if !p.active {
		return nil, nil
	}

	switch msg.Type {
	case tea.KeyLeft, tea.KeyRight, tea.KeyTab:
		p.selectedIdx = 1 - p.selectedIdx
		return nil, nil
	case tea.KeyEnter:
		return p.respond(p.selectedIdx == 0)
	case tea.KeyEsc, tea.KeyCtrlC:
		return p.respond(false)
	}

	switch msg.String() {
	case "y", "Y":
		return p.respond(true)
	case "n", "N":
		return p.respond(false)
	}

	return nil, nil
}

func (p *EnterPlanPrompt) respond(approved bool) (tea.Cmd, *EnterPlanResponseMsg) {
	req := p.request
	p.Hide()
	return nil, &EnterPlanResponseMsg{
		Request:  req,
		Approved: approved,
		Response: &tool.EnterPlanResponse{
			RequestID: req.ID,
			Approved:  approved,
		},
	}
}

// Render renders the prompt
func (p *EnterPlanPrompt) Render() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder

	titleStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Primary).Bold(true)
	sb.WriteString("\n ")
	sb.WriteString(titleStyle.Render("\U0001F4CB Enter Plan Mode?"))
	sb.WriteString("\n\n")

	descStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
	desc := "The assistant wants to enter plan mode to explore the codebase and create an implementation plan before making changes."
	if p.request.Message != "" {
		desc = p.request.Message
	}
	sb.WriteString(" ")
	sb.WriteString(descStyle.Render(desc))
	sb.WriteString("\n\n")

	selectedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)

	sb.WriteString(" ")
	if p.selectedIdx == 0 {
		sb.WriteString(selectedStyle.Render("\u276F [Yes]"))
		sb.WriteString("  ")
		sb.WriteString(unselectedStyle.Render("  No"))
	} else {
		sb.WriteString(unselectedStyle.Render("  Yes"))
		sb.WriteString("  ")
		sb.WriteString(selectedStyle.Render("\u276F [No]"))
	}
	sb.WriteString("\n\n")

	hintStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted).Italic(true)
	sb.WriteString(" ")
	sb.WriteString(hintStyle.Render("\u2190/\u2192 or Tab to switch \u00B7 Enter to confirm \u00B7 y/n for quick select \u00B7 Esc to decline"))
	sb.WriteString("\n")

	return sb.String()
}
