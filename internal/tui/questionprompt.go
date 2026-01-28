package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/yanmxa/gencode/internal/tool"
)

// QuestionPrompt manages the question prompt UI for AskUserQuestion tool
type QuestionPrompt struct {
	active          bool
	request         *tool.QuestionRequest
	width           int
	currentQuestion int              // Current question index
	selectedOption  int              // Currently highlighted option
	selected        map[int][]int    // Question index -> selected option indices
	customInput     textinput.Model  // For "Other" option
	showingCustom   bool             // Whether custom input is visible
}

// NewQuestionPrompt creates a new QuestionPrompt
func NewQuestionPrompt() *QuestionPrompt {
	ti := textinput.New()
	ti.Placeholder = "Type your answer..."
	ti.CharLimit = 200
	ti.Width = 50

	return &QuestionPrompt{
		selected:    make(map[int][]int),
		customInput: ti,
	}
}

// Show displays the question prompt with the given request
func (p *QuestionPrompt) Show(req *tool.QuestionRequest, width int) {
	p.active = true
	p.request = req
	p.width = width
	p.currentQuestion = 0
	p.selectedOption = 0
	p.selected = make(map[int][]int)
	p.showingCustom = false
	p.customInput.Reset()
}

// Hide hides the question prompt
func (p *QuestionPrompt) Hide() {
	p.active = false
	p.request = nil
	p.showingCustom = false
}

// IsActive returns whether the prompt is visible
func (p *QuestionPrompt) IsActive() bool {
	return p.active
}

// GetRequest returns the current question request
func (p *QuestionPrompt) GetRequest() *tool.QuestionRequest {
	return p.request
}

// Message types for question responses
type (
	// QuestionRequestMsg is sent when AskUserQuestion tool is called
	QuestionRequestMsg struct {
		Request  *tool.QuestionRequest
		ToolCall interface{} // Original tool call
	}

	// QuestionResponseMsg is sent when user answers or cancels
	QuestionResponseMsg struct {
		Request   *tool.QuestionRequest
		Response  *tool.QuestionResponse
		Cancelled bool
	}
)

// HandleKeypress handles keyboard input for the question prompt
func (p *QuestionPrompt) HandleKeypress(msg tea.KeyMsg) tea.Cmd {
	if !p.active || p.request == nil {
		return nil
	}

	// If custom input is showing, handle text input
	if p.showingCustom {
		switch msg.Type {
		case tea.KeyEnter:
			// Submit custom input
			return p.submitCustomInput()
		case tea.KeyEsc:
			// Cancel custom input, go back to options
			p.showingCustom = false
			p.customInput.Blur()
			return nil
		default:
			// Forward to text input
			var cmd tea.Cmd
			p.customInput, cmd = p.customInput.Update(msg)
			return cmd
		}
	}

	currentQ := p.request.Questions[p.currentQuestion]
	numOptions := len(currentQ.Options) + 1 // +1 for "Other"

	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		if p.selectedOption > 0 {
			p.selectedOption--
		}
		return nil

	case tea.KeyDown, tea.KeyCtrlN:
		if p.selectedOption < numOptions-1 {
			p.selectedOption++
		}
		return nil

	case tea.KeySpace:
		// Toggle selection in multi-select mode
		if currentQ.MultiSelect {
			p.toggleSelection()
		}
		return nil

	case tea.KeyEnter:
		// If "Other" is selected, show custom input
		if p.selectedOption == len(currentQ.Options) {
			p.showingCustom = true
			p.customInput.Focus()
			return nil
		}

		// For single-select, select and confirm
		if !currentQ.MultiSelect {
			p.selected[p.currentQuestion] = []int{p.selectedOption}
		} else if len(p.selected[p.currentQuestion]) == 0 {
			// Multi-select but nothing selected, select current
			p.selected[p.currentQuestion] = []int{p.selectedOption}
		}

		return p.confirmSelection()

	case tea.KeyEsc:
		// Cancel
		req := p.request
		p.Hide()
		return func() tea.Msg {
			return QuestionResponseMsg{
				Request:   req,
				Cancelled: true,
			}
		}
	}

	// Handle number key shortcuts (1-4)
	key := msg.String()
	if key >= "1" && key <= "4" {
		optionIdx := int(key[0] - '1')
		if optionIdx < numOptions {
			p.selectedOption = optionIdx

			// If selecting "Other" option, show custom input
			if optionIdx == len(currentQ.Options) {
				p.showingCustom = true
				p.customInput.Focus()
				return nil
			}

			// For single-select, confirm immediately; for multi-select, toggle
			if !currentQ.MultiSelect {
				p.selected[p.currentQuestion] = []int{optionIdx}
				return p.confirmSelection()
			}
			p.toggleSelection()
		}
	}

	return nil
}

// toggleSelection toggles the current option in multi-select mode
func (p *QuestionPrompt) toggleSelection() {
	current := p.selected[p.currentQuestion]
	found := false
	newSelection := []int{}

	for _, idx := range current {
		if idx == p.selectedOption {
			found = true
		} else {
			newSelection = append(newSelection, idx)
		}
	}

	if !found {
		newSelection = append(newSelection, p.selectedOption)
	}

	p.selected[p.currentQuestion] = newSelection
}

// submitCustomInput handles submitting custom "Other" input
func (p *QuestionPrompt) submitCustomInput() tea.Cmd {
	customText := strings.TrimSpace(p.customInput.Value())
	if customText == "" {
		// Don't submit empty custom input
		return nil
	}

	req := p.request
	p.Hide()

	// Build response with custom answer
	answers := make(map[int][]string)
	answers[p.currentQuestion] = []string{customText}

	return func() tea.Msg {
		return QuestionResponseMsg{
			Request: req,
			Response: &tool.QuestionResponse{
				RequestID: req.ID,
				Answers:   answers,
				Cancelled: false,
			},
		}
	}
}

// confirmSelection confirms the current selection and moves to next question or finishes
func (p *QuestionPrompt) confirmSelection() tea.Cmd {
	// Move to next question if there are more
	if p.currentQuestion < len(p.request.Questions)-1 {
		p.currentQuestion++
		p.selectedOption = 0
		return nil
	}

	// All questions answered, build response
	req := p.request
	answers := make(map[int][]string)

	for qIdx := 0; qIdx < len(req.Questions); qIdx++ {
		q := req.Questions[qIdx]
		selectedIndices := p.selected[qIdx]

		labels := []string{}
		for _, optIdx := range selectedIndices {
			if optIdx < len(q.Options) {
				labels = append(labels, q.Options[optIdx].Label)
			}
		}
		answers[qIdx] = labels
	}

	p.Hide()

	return func() tea.Msg {
		return QuestionResponseMsg{
			Request: req,
			Response: &tool.QuestionResponse{
				RequestID: req.ID,
				Answers:   answers,
				Cancelled: false,
			},
		}
	}
}

// Question prompt styles (initialized dynamically based on theme)
var (
	questionSeparatorStyle lipgloss.Style
	questionHeaderStyle    lipgloss.Style
	questionTextStyle      lipgloss.Style
	optionSelectedStyle    lipgloss.Style
	optionUnselectedStyle  lipgloss.Style
	optionDescStyle        lipgloss.Style
	questionFooterStyle    lipgloss.Style
)

func init() {
	// Initialize question prompt styles based on current theme
	questionSeparatorStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Separator)

	questionHeaderStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Primary).
		Bold(true).
		Padding(0, 1).
		Background(CurrentTheme.Background)

	questionTextStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Text)

	optionSelectedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Success).
		Bold(true)

	optionUnselectedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDim)

	optionDescStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted).
		Italic(true)

	questionFooterStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)
}

// Render renders the question prompt
func (p *QuestionPrompt) Render() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder
	contentWidth := p.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	currentQ := p.request.Questions[p.currentQuestion]

	// Solid separator
	solidSep := strings.Repeat("─", contentWidth)
	sb.WriteString(questionSeparatorStyle.Render(solidSep))
	sb.WriteString("\n")

	// Header badge
	header := questionHeaderStyle.Render(currentQ.Header)
	sb.WriteString(" ")
	sb.WriteString(header)
	sb.WriteString("\n")

	// Question text
	sb.WriteString(" ")
	sb.WriteString(questionTextStyle.Render(currentQ.Question))
	sb.WriteString("\n\n")

	// Options
	isMulti := currentQ.MultiSelect
	selectedSet := make(map[int]bool)
	for _, idx := range p.selected[p.currentQuestion] {
		selectedSet[idx] = true
	}

	for i, opt := range currentQ.Options {
		isHighlighted := i == p.selectedOption
		isSelected := selectedSet[i]

		// Choose prefix based on selection mode and state
		var prefix string
		switch {
		case isMulti && isSelected:
			prefix = "[✓]"
		case isMulti:
			prefix = "[ ]"
		case isSelected || isHighlighted:
			prefix = "(●)"
		default:
			prefix = "( )"
		}

		// Cursor indicator
		cursor := "   "
		if isHighlighted {
			cursor = " ❯ "
		}

		// Option line
		var optStyle lipgloss.Style
		if isHighlighted {
			optStyle = optionSelectedStyle
		} else {
			optStyle = optionUnselectedStyle
		}

		optLine := fmt.Sprintf("%s%s %d. %s", cursor, prefix, i+1, opt.Label)
		sb.WriteString(optStyle.Render(optLine))

		// Description
		if opt.Description != "" {
			sb.WriteString(" - ")
			sb.WriteString(optionDescStyle.Render(opt.Description))
		}
		sb.WriteString("\n")
	}

	// "Other" option
	otherIdx := len(currentQ.Options)
	isOtherHighlighted := p.selectedOption == otherIdx

	otherCursor := "   "
	if isOtherHighlighted {
		otherCursor = " ❯ "
	}

	otherStyle := optionUnselectedStyle
	if isOtherHighlighted {
		otherStyle = optionSelectedStyle
	}

	otherPrefix := "( )"
	if isMulti {
		otherPrefix = "[ ]"
	}

	sb.WriteString(otherStyle.Render(fmt.Sprintf("%s%s %d. Other", otherCursor, otherPrefix, otherIdx+1)))
	sb.WriteString(" - ")
	sb.WriteString(optionDescStyle.Render("Type custom response"))
	sb.WriteString("\n")

	// Custom input (if showing)
	if p.showingCustom {
		sb.WriteString("\n")
		sb.WriteString("   ")
		sb.WriteString(p.customInput.View())
		sb.WriteString("\n")
	}

	// Dotted separator
	dottedSep := strings.Repeat("╌", contentWidth)
	sb.WriteString(questionSeparatorStyle.Render(dottedSep))
	sb.WriteString("\n")

	// Footer with hints
	var hints []string
	hints = append(hints, "↑/↓ navigate")
	if isMulti {
		hints = append(hints, "Space toggle")
	}
	hints = append(hints, "Enter confirm", "Esc cancel")

	footer := " " + strings.Join(hints, " · ")
	sb.WriteString(questionFooterStyle.Render(footer))

	return sb.String()
}
