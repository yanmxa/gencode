package conv

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/tool"
)

// PlanPrompt manages the plan approval UI
type PlanPrompt struct {
	active      bool
	request     *tool.PlanRequest
	width       int
	height      int
	selectedIdx int
	editing     bool
	editor      textarea.Model
	planPath    string
	inlineInput textarea.Model
}

// NewPlanPrompt creates a new PlanPrompt
func NewPlanPrompt() *PlanPrompt {
	ta := textarea.New()
	ta.Placeholder = "Modify the plan here..."
	ta.CharLimit = 0
	ta.ShowLineNumbers = true

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

	editorHeight := height - 13
	if editorHeight < 5 {
		editorHeight = 5
	}
	p.editor.SetValue(req.Plan)
	p.editor.SetWidth(width - 6)
	p.editor.SetHeight(editorHeight - 2)

	p.inlineInput.SetValue("")
	p.inlineInput.SetWidth(width - 20)
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

// HandleKeypress handles keyboard input for the plan prompt.
func (p *PlanPrompt) HandleKeypress(msg tea.KeyMsg) (tea.Cmd, *PlanResponseMsg) {
	if !p.active {
		return nil, nil
	}

	if p.editing {
		switch msg.Type {
		case tea.KeyCtrlS:
			return p.submitModifiedPlan()
		case tea.KeyEsc:
			p.editing = false
			p.editor.Blur()
			return nil, nil
		default:
			var cmd tea.Cmd
			p.editor, cmd = p.editor.Update(msg)
			return cmd, nil
		}
	}

	if p.selectedIdx == 3 {
		switch msg.Type {
		case tea.KeyEnter:
			return p.submitInlineInput()
		case tea.KeyEsc:
			p.inlineInput.Blur()
			p.inlineInput.SetValue("")
			p.selectedIdx = 0
			return nil, nil
		case tea.KeyUp, tea.KeyCtrlP:
			p.inlineInput.Blur()
			p.selectedIdx = 2
			return nil, nil
		default:
			var cmd tea.Cmd
			p.inlineInput, cmd = p.inlineInput.Update(msg)
			return cmd, nil
		}
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
			if p.selectedIdx == 3 {
				p.inlineInput.Focus()
			}
		}
		return nil, nil

	case tea.KeyEnter:
		return p.selectOption(p.selectedIdx)

	case tea.KeyShiftTab:
		return p.selectOption(0)

	case tea.KeyEsc, tea.KeyCtrlC:
		req := p.request
		p.Hide()
		return nil, &PlanResponseMsg{
			Request:  req,
			Approved: false,
			Response: &tool.PlanResponse{
				RequestID: req.ID,
				Approved:  false,
			},
		}
	}

	switch msg.String() {
	case "1":
		return p.selectOption(0)
	case "2":
		return p.selectOption(1)
	case "3":
		return p.selectOption(2)
	case "4":
		p.selectedIdx = 3
		p.inlineInput.Focus()
		return nil, nil
	}

	return nil, nil
}

func (p *PlanPrompt) selectOption(idx int) (tea.Cmd, *PlanResponseMsg) {
	if idx == 3 {
		p.selectedIdx = 3
		p.inlineInput.Focus()
		return nil, nil
	}

	approveModes := []string{"clear-auto", "auto", "manual"}
	if idx < 0 || idx >= len(approveModes) {
		return nil, nil
	}

	req := p.request
	mode := approveModes[idx]
	p.Hide()

	return nil, &PlanResponseMsg{
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

func (p *PlanPrompt) submitModifiedPlan() (tea.Cmd, *PlanResponseMsg) {
	req := p.request
	modifiedPlan := p.editor.Value()
	p.Hide()

	return nil, &PlanResponseMsg{
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

func (p *PlanPrompt) submitInlineInput() (tea.Cmd, *PlanResponseMsg) {
	req := p.request
	feedback := strings.TrimSpace(p.inlineInput.Value())
	if feedback == "" {
		return nil, nil
	}
	p.Hide()

	modifiedPlan := req.Plan + "\n\n---\n\n**User Feedback:**\n" + feedback

	return nil, &PlanResponseMsg{
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

func getPlanSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Success).Bold(true)
}

func getPlanUnselectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.TextDim)
}

func getPlanHintStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted).Italic(true)
}

func getPlanFooterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
}

// RenderMenu renders the menu options for the input area (below separator)
func (p *PlanPrompt) RenderMenu() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(" ")
	sb.WriteString(getPlanUnselectedStyle().Render("Would you like to proceed?"))
	sb.WriteString("\n\n")

	if !p.editing {
		sb.WriteString(p.renderMenu())
	}
	sb.WriteString("\n")

	footer := " Esc to reject"
	if p.planPath != "" {
		footer += " · " + kit.ShortenPath(p.planPath)
	}
	sb.WriteString(getPlanFooterStyle().Render(footer))

	return sb.String()
}

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
			sb.WriteString(getPlanSelectedStyle().Render(fmt.Sprintf(" \u276F %d. %s", i+1, opt.label)))
		} else {
			sb.WriteString(getPlanUnselectedStyle().Render(fmt.Sprintf("   %d. %s", i+1, opt.label)))
		}
		if opt.hint != "" {
			sb.WriteString(" ")
			sb.WriteString(getPlanHintStyle().Render(opt.hint))
		}
		sb.WriteString("\n")
	}

	if p.selectedIdx == 3 {
		sb.WriteString(getPlanSelectedStyle().Render(" \u276F 4. "))
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
		editorHeight := height - 13
		if editorHeight < 5 {
			editorHeight = 5
		}
		p.editor.SetWidth(width - 6)
		p.editor.SetHeight(editorHeight - 2)
		p.inlineInput.SetWidth(width - 20)
	}
}
