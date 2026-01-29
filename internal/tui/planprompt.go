package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool"
)

// PlanPrompt manages the plan approval UI
type PlanPrompt struct {
	active      bool
	request     *tool.PlanRequest
	width       int
	height      int
	selectedIdx int            // Current menu selection (0-3)
	viewport    viewport.Model // For scrolling plan content
	editing     bool           // Whether in edit mode
	editor      textarea.Model // For modifying plan
	mdRenderer  *glamour.TermRenderer
}

// NewPlanPrompt creates a new PlanPrompt
func NewPlanPrompt() *PlanPrompt {
	ta := textarea.New()
	ta.Placeholder = "Modify the plan here..."
	ta.CharLimit = 0 // No limit
	ta.ShowLineNumbers = true

	return &PlanPrompt{
		editor: ta,
	}
}

// Show displays the plan prompt with the given request
func (p *PlanPrompt) Show(req *tool.PlanRequest, width, height int) {
	p.active = true
	p.request = req
	p.width = width
	p.height = height
	p.selectedIdx = 0
	p.editing = false

	// Calculate viewport height (leave room for menu and footer)
	viewportHeight := height - 12
	if viewportHeight < 10 {
		viewportHeight = 10
	}

	// Initialize viewport
	p.viewport = viewport.New(width-4, viewportHeight)
	p.viewport.Style = lipgloss.NewStyle()

	// Initialize markdown renderer
	p.mdRenderer = createMarkdownRenderer(width - 4)

	// Render plan content
	p.updateViewportContent()

	// Initialize editor with plan content
	p.editor.SetValue(req.Plan)
	p.editor.SetWidth(width - 6)
	p.editor.SetHeight(viewportHeight - 2)
}

// updateViewportContent renders the plan content to the viewport
func (p *PlanPrompt) updateViewportContent() {
	if p.request == nil {
		return
	}

	content := p.request.Plan
	if p.mdRenderer != nil {
		if rendered, err := p.mdRenderer.Render(content); err == nil {
			content = strings.TrimSpace(rendered)
		}
	}
	p.viewport.SetContent(content)
}

// Hide hides the plan prompt
func (p *PlanPrompt) Hide() {
	p.active = false
	p.request = nil
	p.editing = false
}

// IsActive returns whether the prompt is visible
func (p *PlanPrompt) IsActive() bool {
	return p.active
}

// IsEditing returns whether the prompt is in edit mode
func (p *PlanPrompt) IsEditing() bool {
	return p.editing
}

// GetRequest returns the current plan request
func (p *PlanPrompt) GetRequest() *tool.PlanRequest {
	return p.request
}

// Message types for plan responses
type (
	// PlanRequestMsg is sent when ExitPlanMode tool is called
	PlanRequestMsg struct {
		Request *tool.PlanRequest
	}

	// PlanResponseMsg is sent when user responds to plan approval
	PlanResponseMsg struct {
		Request      *tool.PlanRequest
		Response     *tool.PlanResponse
		Approved     bool
		ApproveMode  string // "clear-auto" | "auto" | "manual" | "modify"
		ModifiedPlan string // Modified plan if edited
	}
)

// HandleKeypress handles keyboard input for the plan prompt
func (p *PlanPrompt) HandleKeypress(msg tea.KeyMsg) tea.Cmd {
	if !p.active {
		return nil
	}

	// If in edit mode, handle editor keys
	if p.editing {
		switch msg.Type {
		case tea.KeyCtrlS:
			// Save and submit modified plan
			return p.submitModifiedPlan()
		case tea.KeyEsc:
			// Exit edit mode without saving
			p.editing = false
			p.editor.Blur()
			return nil
		default:
			// Forward to editor
			var cmd tea.Cmd
			p.editor, cmd = p.editor.Update(msg)
			return cmd
		}
	}

	// Normal mode (menu navigation)
	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if p.selectedIdx > 0 {
			p.selectedIdx--
		}
		return nil

	case tea.KeyDown, tea.KeyCtrlN:
		if p.selectedIdx < 3 {
			p.selectedIdx++
		}
		return nil

	case tea.KeyEnter:
		return p.confirmSelection()

	case tea.KeyShiftTab:
		// Quick "clear + auto" option
		return p.selectOption(0)

	case tea.KeyEsc:
		// Reject plan
		req := p.request
		p.Hide()
		return func() tea.Msg {
			return PlanResponseMsg{
				Request:  req,
				Approved: false,
				Response: &tool.PlanResponse{
					RequestID: req.ID,
					Approved:  false,
				},
			}
		}

	case tea.KeyPgUp, tea.KeyCtrlU:
		// Let the main viewport handle scrolling (return nil to pass through)
		return nil

	case tea.KeyPgDown, tea.KeyCtrlD:
		// Let the main viewport handle scrolling (return nil to pass through)
		return nil
	}

	// Handle number key shortcuts
	switch msg.String() {
	case "1":
		return p.selectOption(0)
	case "2":
		return p.selectOption(1)
	case "3":
		return p.selectOption(2)
	case "4":
		return p.selectOption(3)
	}

	return nil
}

// confirmSelection confirms the currently selected option
func (p *PlanPrompt) confirmSelection() tea.Cmd {
	return p.selectOption(p.selectedIdx)
}

// selectOption handles selection of a specific option
func (p *PlanPrompt) selectOption(idx int) tea.Cmd {
	if idx == 3 {
		p.editing = true
		p.editor.Focus()
		return nil
	}

	approveModes := []string{"clear-auto", "auto", "manual"}
	if idx < 0 || idx >= len(approveModes) {
		return nil
	}

	req := p.request
	mode := approveModes[idx]
	p.Hide()

	return func() tea.Msg {
		return PlanResponseMsg{
			Request:     req,
			Approved:    true,
			ApproveMode: mode,
			Response: &tool.PlanResponse{
				RequestID:   req.ID,
				Approved:    true,
				ApproveMode: mode,
			},
		}
	}
}

// submitModifiedPlan submits the edited plan
func (p *PlanPrompt) submitModifiedPlan() tea.Cmd {
	req := p.request
	modifiedPlan := p.editor.Value()
	p.Hide()

	return func() tea.Msg {
		return PlanResponseMsg{
			Request:      req,
			Approved:     true,
			ApproveMode:  "modify",
			ModifiedPlan: modifiedPlan,
			Response: &tool.PlanResponse{
				RequestID:    req.ID,
				Approved:     true,
				ApproveMode:  "modify",
				ModifiedPlan: modifiedPlan,
			},
		}
	}
}

// Plan prompt styles
func getPlanSeparatorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Separator)
}

func getPlanTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Accent).Bold(true)
}

func getPlanSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Success).Bold(true)
}

func getPlanUnselectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.TextDim)
}

func getPlanHintStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Muted).Italic(true)
}

func getPlanFooterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
}

// RenderContent renders the plan content for the chat viewport (above separator)
// This returns the rendered markdown directly (not wrapped in a viewport),
// allowing the main chat viewport to handle scrolling.
func (p *PlanPrompt) RenderContent() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder

	// Title
	sb.WriteString("\n ")
	sb.WriteString(getPlanTitleStyle().Render("📋 Implementation Plan"))
	sb.WriteString("\n\n")

	// Plan content or editor
	if p.editing {
		// Show editor
		sb.WriteString(" ")
		sb.WriteString(getPlanHintStyle().Render("Edit the plan below. Ctrl+S to save, Esc to cancel."))
		sb.WriteString("\n\n")
		sb.WriteString(p.editor.View())
	} else {
		// Render markdown directly (not via p.viewport) so the main viewport can scroll
		content := p.request.Plan
		if p.mdRenderer != nil {
			if rendered, err := p.mdRenderer.Render(content); err == nil {
				content = strings.TrimSpace(rendered)
			}
		}
		sb.WriteString(content)
	}

	return sb.String()
}

// RenderMenu renders the menu options for the input area (below separator)
func (p *PlanPrompt) RenderMenu() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder

	// Question
	sb.WriteString(" ")
	sb.WriteString(getPlanUnselectedStyle().Render("How would you like to proceed?"))
	sb.WriteString("\n")

	// Menu options
	if !p.editing {
		sb.WriteString(p.renderMenu())
	}

	// Footer
	footer := " Esc to reject"
	if !p.editing {
		footer += " · ↑/↓ navigate · PgUp/PgDn scroll"
	}
	sb.WriteString(getPlanFooterStyle().Render(footer))

	return sb.String()
}

// Render renders the plan prompt (legacy - full render)
func (p *PlanPrompt) Render() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder
	contentWidth := p.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Title
	sb.WriteString(" ")
	sb.WriteString(getPlanTitleStyle().Render("📋 Implementation Plan"))
	sb.WriteString("\n\n")

	// Plan content (viewport) or editor
	if p.editing {
		// Show editor
		sb.WriteString(" ")
		sb.WriteString(getPlanHintStyle().Render("Edit the plan below. Ctrl+S to save, Esc to cancel."))
		sb.WriteString("\n\n")
		sb.WriteString(p.editor.View())
	} else {
		// Show rendered plan
		sb.WriteString(p.viewport.View())
	}
	sb.WriteString("\n\n")

	// Question
	sb.WriteString(" ")
	sb.WriteString(getPlanUnselectedStyle().Render("How would you like to proceed?"))
	sb.WriteString("\n")

	// Menu options
	if !p.editing {
		sb.WriteString(p.renderMenu())
	}
	sb.WriteString("\n")

	// Footer
	footer := " Esc to reject"
	if !p.editing {
		footer += " · ↑/↓ navigate · PgUp/PgDn scroll"
	}
	sb.WriteString(getPlanFooterStyle().Render(footer))
	sb.WriteString("\n")

	// Bottom separator
	solidSep := strings.Repeat("─", contentWidth)
	sb.WriteString(getPlanSeparatorStyle().Render(solidSep))

	return sb.String()
}

// renderMenu renders the selection menu
func (p *PlanPrompt) renderMenu() string {
	var sb strings.Builder

	options := []struct {
		label string
		hint  string
	}{
		{"Clear context, auto-accept edits", "(shift+tab)"},
		{"Keep context, auto-accept edits", ""},
		{"Keep context, manually approve each edit", ""},
		{"Modify this plan", ""},
	}

	for i, opt := range options {
		if i == p.selectedIdx {
			sb.WriteString(getPlanSelectedStyle().Render(fmt.Sprintf(" ❯ %d. %s", i+1, opt.label)))
		} else {
			sb.WriteString(getPlanUnselectedStyle().Render(fmt.Sprintf("   %d. %s", i+1, opt.label)))
		}
		if opt.hint != "" {
			sb.WriteString(" ")
			sb.WriteString(getPlanHintStyle().Render(opt.hint))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// SetSize updates the prompt dimensions
func (p *PlanPrompt) SetSize(width, height int) {
	p.width = width
	p.height = height

	if p.active {
		viewportHeight := height - 12
		if viewportHeight < 10 {
			viewportHeight = 10
		}
		p.viewport.Width = width - 4
		p.viewport.Height = viewportHeight
		p.editor.SetWidth(width - 6)
		p.editor.SetHeight(viewportHeight - 2)

		// Update markdown renderer
		p.mdRenderer = createMarkdownRenderer(width - 4)
		p.updateViewportContent()
	}
}
