package tui

import (
	"fmt"
	"os"
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
	planPath    string         // Path to the plan file (displayed in footer)
	inlineInput textarea.Model // Inline input for option 4
}

// NewPlanPrompt creates a new PlanPrompt
func NewPlanPrompt() *PlanPrompt {
	ta := textarea.New()
	ta.Placeholder = "Modify the plan here..."
	ta.CharLimit = 0 // No limit
	ta.ShowLineNumbers = true

	// Inline input for option 4 (single line)
	inlineTA := textarea.New()
	inlineTA.Placeholder = ""
	inlineTA.CharLimit = 0
	inlineTA.ShowLineNumbers = false
	inlineTA.SetHeight(1)

	return &PlanPrompt{
		editor:      ta,
		inlineInput: inlineTA,
	}
}

// Show displays the plan prompt with the given request
func (p *PlanPrompt) Show(req *tool.PlanRequest, planPath string, width, height int) {
	p.active = true
	p.request = req
	p.planPath = planPath
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

	// Initialize inline input
	p.inlineInput.SetValue("")
	p.inlineInput.SetWidth(width - 20)
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
	p.planPath = ""
	p.inlineInput.Blur()
	p.inlineInput.SetValue("")
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

	// If in edit mode (full editor), handle editor keys
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

	// If option 4 is selected (inline input mode), handle inline input keys
	if p.selectedIdx == 3 {
		switch msg.Type {
		case tea.KeyEnter:
			// Submit inline input
			return p.submitInlineInput()
		case tea.KeyEsc:
			// Exit inline input, deselect option 4
			p.inlineInput.Blur()
			p.inlineInput.SetValue("")
			p.selectedIdx = 0
			return nil
		case tea.KeyUp, tea.KeyCtrlP:
			// Move to option 3
			p.inlineInput.Blur()
			p.selectedIdx = 2
			return nil
		default:
			// Forward to inline input
			var cmd tea.Cmd
			p.inlineInput, cmd = p.inlineInput.Update(msg)
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
			// Focus inline input when moving to option 4
			if p.selectedIdx == 3 {
				p.inlineInput.Focus()
			}
		}
		return nil

	case tea.KeyEnter:
		return p.confirmSelection()

	case tea.KeyShiftTab:
		// Quick "clear + auto" option
		return p.selectOption(0)

	case tea.KeyEsc, tea.KeyCtrlC:
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
		// Focus inline input
		p.selectedIdx = 3
		p.inlineInput.Focus()
		return nil
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
		// Option 4: Focus inline input instead of full editor
		p.selectedIdx = 3
		p.inlineInput.Focus()
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

// submitInlineInput submits the inline feedback for plan modification
func (p *PlanPrompt) submitInlineInput() tea.Cmd {
	req := p.request
	feedback := strings.TrimSpace(p.inlineInput.Value())
	if feedback == "" {
		return nil // Don't submit empty feedback
	}
	p.Hide()

	// Create a modified plan that includes the feedback as instructions
	modifiedPlan := req.Plan + "\n\n---\n\n**User Feedback:**\n" + feedback

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
	return lipgloss.NewStyle().Foreground(CurrentTheme.Primary).Bold(true)
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

// shortenPath shortens a path by replacing home directory with ~
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// RenderContent renders the plan content for the chat viewport (above separator)
// This returns the rendered markdown directly (not wrapped in a viewport),
// allowing the main chat viewport to handle scrolling.
func (p *PlanPrompt) RenderContent() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder

	// Plan content or editor
	if p.editing {
		// Show editor without border
		sb.WriteString("\n ")
		sb.WriteString(getPlanTitleStyle().Render("📋 Implementation Plan"))
		sb.WriteString("\n\n")
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

		// Wrap plan content in a bordered box
		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(CurrentTheme.Accent).
			Padding(1, 2).
			Width(p.width - 4)

		// Add title inside border
		title := getPlanTitleStyle().Render("📋 Implementation Plan")
		boxContent := title + "\n\n" + content

		sb.WriteString("\n")
		sb.WriteString(borderStyle.Render(boxContent))
		sb.WriteString("\n")
	}

	return sb.String()
}

// RenderMenu renders the menu options for the input area (below separator)
func (p *PlanPrompt) RenderMenu() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder

	// Question with blank line after
	sb.WriteString("\n")
	sb.WriteString(" ")
	sb.WriteString(getPlanUnselectedStyle().Render("Would you like to proceed?"))
	sb.WriteString("\n\n")

	// Menu options
	if !p.editing {
		sb.WriteString(p.renderMenu())
	}
	sb.WriteString("\n")

	// Footer: simple hint + plan path
	footer := " Esc to reject"
	if p.planPath != "" {
		footer += " · " + shortenPath(p.planPath)
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
		{"Yes, clear context and auto-accept edits", "(shift+tab)"},
		{"Yes, auto-accept edits", ""},
		{"Yes, manually approve edits", ""},
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

	// Option 4: Inline input prompt
	if p.selectedIdx == 3 {
		sb.WriteString(getPlanSelectedStyle().Render(" ❯ 4. "))
		sb.WriteString(p.inlineInput.View())
	} else {
		sb.WriteString(getPlanHintStyle().Render("   4. Type here to tell Claude what to change"))
	}
	sb.WriteString("\n")

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
		p.inlineInput.SetWidth(width - 20)

		// Update markdown renderer
		p.mdRenderer = createMarkdownRenderer(width - 4)
		p.updateViewportContent()
	}
}
