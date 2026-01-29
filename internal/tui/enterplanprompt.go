package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool"
)

// EnterPlanPrompt manages the enter plan mode confirmation UI
type EnterPlanPrompt struct {
	active      bool
	request     *tool.EnterPlanRequest
	width       int
	selectedIdx int // 0 = Yes, 1 = No
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
	p.selectedIdx = 0 // Default to "Yes"
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

// GetRequest returns the current request
func (p *EnterPlanPrompt) GetRequest() *tool.EnterPlanRequest {
	return p.request
}

// Message types for enter plan mode
type (
	// EnterPlanRequestMsg is sent when EnterPlanMode tool is called
	EnterPlanRequestMsg struct {
		Request *tool.EnterPlanRequest
	}

	// EnterPlanResponseMsg is sent when user responds
	EnterPlanResponseMsg struct {
		Request  *tool.EnterPlanRequest
		Response *tool.EnterPlanResponse
		Approved bool
	}
)

// HandleKeypress handles keyboard input
func (p *EnterPlanPrompt) HandleKeypress(msg tea.KeyMsg) tea.Cmd {
	if !p.active {
		return nil
	}

	switch msg.Type {
	case tea.KeyLeft, tea.KeyRight, tea.KeyTab:
		// Toggle between Yes and No
		p.selectedIdx = 1 - p.selectedIdx
		return nil

	case tea.KeyEnter:
		return p.confirmSelection()

	case tea.KeyEsc:
		// Decline
		return p.selectOption(false)

	case tea.KeyCtrlC:
		return p.selectOption(false)
	}

	// Handle 'y' and 'n' shortcuts
	switch msg.String() {
	case "y", "Y":
		return p.selectOption(true)
	case "n", "N":
		return p.selectOption(false)
	}

	return nil
}

// confirmSelection confirms the currently selected option
func (p *EnterPlanPrompt) confirmSelection() tea.Cmd {
	return p.selectOption(p.selectedIdx == 0)
}

// selectOption handles selection
func (p *EnterPlanPrompt) selectOption(approved bool) tea.Cmd {
	req := p.request
	p.Hide()

	return func() tea.Msg {
		return EnterPlanResponseMsg{
			Request:  req,
			Approved: approved,
			Response: &tool.EnterPlanResponse{
				RequestID: req.ID,
				Approved:  approved,
			},
		}
	}
}

// Render renders the prompt
func (p *EnterPlanPrompt) Render() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Bold(true)
	sb.WriteString("\n ")
	sb.WriteString(titleStyle.Render("📋 Enter Plan Mode?"))
	sb.WriteString("\n\n")

	// Description
	descStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)
	desc := "The assistant wants to enter plan mode to explore the codebase and create an implementation plan before making changes."
	if p.request.Message != "" {
		desc = p.request.Message
	}
	sb.WriteString(" ")
	sb.WriteString(descStyle.Render(desc))
	sb.WriteString("\n\n")

	// Options
	selectedStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Success).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)

	sb.WriteString(" ")
	if p.selectedIdx == 0 {
		sb.WriteString(selectedStyle.Render("❯ [Yes]"))
		sb.WriteString("  ")
		sb.WriteString(unselectedStyle.Render("  No"))
	} else {
		sb.WriteString(unselectedStyle.Render("  Yes"))
		sb.WriteString("  ")
		sb.WriteString(selectedStyle.Render("❯ [No]"))
	}
	sb.WriteString("\n\n")

	// Footer hint
	hintStyle := lipgloss.NewStyle().Foreground(CurrentTheme.Muted).Italic(true)
	sb.WriteString(" ")
	sb.WriteString(hintStyle.Render("←/→ or Tab to switch · Enter to confirm · y/n for quick select · Esc to decline"))
	sb.WriteString("\n")

	return sb.String()
}

// SetWidth updates the prompt width
func (p *EnterPlanPrompt) SetWidth(width int) {
	p.width = width
}
