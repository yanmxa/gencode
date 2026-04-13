package mode

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// QuestionPrompt manages the question prompt UI for AskUserQuestion tool
type QuestionPrompt struct {
	active          bool
	request         *tool.QuestionRequest
	width           int
	currentQuestion int             // Current question index
	selectedOption map[int]int    // Per-question highlighted option index
	selected       map[int][]int  // Question index -> selected option indices
	customAnswers  map[int]string // Question index -> custom "Other" answer text
	customInput     textinput.Model // For "Other" option
	showingCustom   bool            // Whether custom input is visible
}

// NewQuestionPrompt creates a new QuestionPrompt
func NewQuestionPrompt() *QuestionPrompt {
	ti := textinput.New()
	ti.Placeholder = "Type your answer..."
	ti.CharLimit = 200
	ti.Width = 50

	return &QuestionPrompt{
		selectedOption: make(map[int]int),
		selected:       make(map[int][]int),
		customAnswers:  make(map[int]string),
		customInput:    ti,
	}
}

// Show displays the question prompt with the given request
func (p *QuestionPrompt) Show(req *tool.QuestionRequest, width int) {
	p.active = true
	p.request = req
	p.width = width
	p.currentQuestion = 0
	p.selectedOption = make(map[int]int)
	p.selected = make(map[int][]int)
	p.customAnswers = make(map[int]string)
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

// isAnswered returns whether a question has been answered (via selection or custom input).
func (p *QuestionPrompt) isAnswered(questionIdx int) bool {
	return len(p.selected[questionIdx]) > 0 || p.customAnswers[questionIdx] != ""
}

// restoreCustomInput restores the custom input field value for the current question.
func (p *QuestionPrompt) restoreCustomInput() {
	if text := p.customAnswers[p.currentQuestion]; text != "" {
		p.customInput.SetValue(text)
	}
}

// HandleKeypress handles keyboard input for the question prompt.
// Returns (cmd, response): cmd for UI updates, response when user makes a decision.
func (p *QuestionPrompt) HandleKeypress(msg tea.KeyMsg) (tea.Cmd, *QuestionResponseMsg) {
	if !p.active || p.request == nil {
		return nil, nil
	}

	if p.showingCustom {
		switch msg.Type {
		case tea.KeyEnter:
			return p.submitCustomInput()
		case tea.KeyEsc:
			p.showingCustom = false
			p.customInput.Blur()
			return nil, nil
		default:
			var cmd tea.Cmd
			p.customInput, cmd = p.customInput.Update(msg)
			return cmd, nil
		}
	}

	currentQ := p.request.Questions[p.currentQuestion]
	numOptions := len(currentQ.Options) + 1 // +1 for "Other"
	curOption := p.selectedOption[p.currentQuestion]

	switch msg.Type {
	case tea.KeyLeft:
		if len(p.request.Questions) > 1 && p.currentQuestion > 0 {
			p.currentQuestion--
			p.restoreCustomInput()
		}
		return nil, nil

	case tea.KeyRight:
		if len(p.request.Questions) > 1 && p.currentQuestion < len(p.request.Questions)-1 {
			p.currentQuestion++
			p.restoreCustomInput()
		}
		return nil, nil

	case tea.KeyTab:
		if len(p.request.Questions) > 1 {
			p.currentQuestion = (p.currentQuestion + 1) % len(p.request.Questions)
			p.restoreCustomInput()
		}
		return nil, nil

	case tea.KeyUp, tea.KeyCtrlP:
		if curOption > 0 {
			p.selectedOption[p.currentQuestion] = curOption - 1
		}
		return nil, nil

	case tea.KeyDown, tea.KeyCtrlN:
		if curOption < numOptions-1 {
			p.selectedOption[p.currentQuestion] = curOption + 1
		}
		return nil, nil

	case tea.KeySpace:
		if currentQ.MultiSelect {
			p.toggleSelection()
		}
		return nil, nil

	case tea.KeyEnter:
		if curOption == len(currentQ.Options) {
			p.showingCustom = true
			p.customInput.Focus()
			p.restoreCustomInput()
			return nil, nil
		}

		if !currentQ.MultiSelect {
			p.selected[p.currentQuestion] = []int{curOption}
		} else if len(p.selected[p.currentQuestion]) == 0 {
			p.selected[p.currentQuestion] = []int{curOption}
		}

		return p.tryFinishOrAdvance()

	case tea.KeyEsc, tea.KeyCtrlC:
		req := p.request
		p.Hide()
		return nil, &QuestionResponseMsg{
			Request:   req,
			Cancelled: true,
		}
	}

	key := msg.String()
	if key >= "1" && key <= "4" {
		optionIdx := int(key[0] - '1')
		if optionIdx < numOptions {
			p.selectedOption[p.currentQuestion] = optionIdx

			if optionIdx == len(currentQ.Options) {
				p.showingCustom = true
				p.customInput.Focus()
				p.restoreCustomInput()
				return nil, nil
			}

			if !currentQ.MultiSelect {
				p.selected[p.currentQuestion] = []int{optionIdx}
				return p.tryFinishOrAdvance()
			}
			p.toggleSelection()
		}
	}

	return nil, nil
}

// toggleSelection toggles the current option in multi-select mode
func (p *QuestionPrompt) toggleSelection() {
	current := p.selected[p.currentQuestion]
	curOption := p.selectedOption[p.currentQuestion]
	found := false
	newSelection := []int{}

	for _, idx := range current {
		if idx == curOption {
			found = true
		} else {
			newSelection = append(newSelection, idx)
		}
	}

	if !found {
		newSelection = append(newSelection, curOption)
	}

	p.selected[p.currentQuestion] = newSelection
}

// submitCustomInput handles submitting custom "Other" input.
func (p *QuestionPrompt) submitCustomInput() (tea.Cmd, *QuestionResponseMsg) {
	customText := strings.TrimSpace(p.customInput.Value())
	if customText == "" {
		return nil, nil
	}

	p.customAnswers[p.currentQuestion] = customText
	p.showingCustom = false
	p.customInput.Blur()
	p.customInput.Reset()

	return p.tryFinishOrAdvance()
}

// tryFinishOrAdvance checks if all questions are answered. If so, builds the response.
// Otherwise, advances to the next unanswered question.
func (p *QuestionPrompt) tryFinishOrAdvance() (tea.Cmd, *QuestionResponseMsg) {
	n := len(p.request.Questions)

	// Find next unanswered question, starting forward from current position
	nextUnanswered := -1
	for offset := 1; offset < n; offset++ {
		idx := (p.currentQuestion + offset) % n
		if !p.isAnswered(idx) {
			nextUnanswered = idx
			break
		}
	}

	// If we found an unanswered question, navigate to it
	if nextUnanswered >= 0 {
		p.currentQuestion = nextUnanswered
		return nil, nil
	}

	// Check if the current question itself is unanswered (single-question case)
	if !p.isAnswered(p.currentQuestion) {
		return nil, nil
	}

	req := p.request
	answers := make(map[int][]string)

	for qIdx := 0; qIdx < len(req.Questions); qIdx++ {
		if customText, ok := p.customAnswers[qIdx]; ok {
			answers[qIdx] = []string{customText}
			continue
		}
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

	return nil, &QuestionResponseMsg{
		Request: req,
		Response: &tool.QuestionResponse{
			RequestID: req.ID,
			Answers:   answers,
			Cancelled: false,
		},
	}
}

// Question prompt styles - use functions to get current theme dynamically
// This ensures styles update when theme changes
func getQuestionSeparatorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Separator)
}

func getQuestionHeaderStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.Primary).
		Bold(true)
}

func getQuestionTextStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Text)
}

func getQuestionSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success).Bold(true)
}

func getQuestionUnselectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
}

func getQuestionDescStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted).Italic(true)
}

func getQuestionFooterStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
}

func getQuestionTabActiveStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(theme.CurrentTheme.TextBright).
		Bold(true).
		Underline(true)
}

func getQuestionTabInactiveStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
}

func getQuestionTabAnsweredStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(theme.CurrentTheme.Success)
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
	solidSep := strings.Repeat("\u2500", contentWidth)
	sb.WriteString(getQuestionSeparatorStyle().Render(solidSep))
	sb.WriteString("\n")

	// Tab bar (only when multiple questions)
	if len(p.request.Questions) > 1 {
		sb.WriteString(" ")
		for i, q := range p.request.Questions {
			label := q.Header
			if label == "" {
				label = fmt.Sprintf("Q%d", i+1)
			}

			var tabStyle lipgloss.Style
			if i == p.currentQuestion {
				tabStyle = getQuestionTabActiveStyle()
			} else if p.isAnswered(i) {
				label = "\u2713 " + label
				tabStyle = getQuestionTabAnsweredStyle()
			} else {
				tabStyle = getQuestionTabInactiveStyle()
			}

			sb.WriteString(tabStyle.Render(label))
			if i < len(p.request.Questions)-1 {
				sb.WriteString(getQuestionSeparatorStyle().Render(" \u2502 "))
			}
		}
		sb.WriteString("\n")

		thinSep := strings.Repeat("\u2504", contentWidth)
		sb.WriteString(getQuestionSeparatorStyle().Render(thinSep))
		sb.WriteString("\n")
	}

	// Header badge
	header := getQuestionHeaderStyle().Render(currentQ.Header)
	sb.WriteString(" ")
	sb.WriteString(header)
	sb.WriteString("\n")

	// Question text
	sb.WriteString(" ")
	sb.WriteString(getQuestionTextStyle().Render(currentQ.Question))
	sb.WriteString("\n\n")

	// Options
	isMulti := currentQ.MultiSelect
	curOption := p.selectedOption[p.currentQuestion]
	selectedSet := make(map[int]bool)
	for _, idx := range p.selected[p.currentQuestion] {
		selectedSet[idx] = true
	}

	for i, opt := range currentQ.Options {
		isHighlighted := i == curOption
		isSelected := selectedSet[i]

		var prefix string
		switch {
		case isMulti && isSelected:
			prefix = "[\u2713]"
		case isMulti:
			prefix = "[ ]"
		case isSelected || isHighlighted:
			prefix = "(\u25CF)"
		default:
			prefix = "( )"
		}

		cursor := "   "
		if isHighlighted {
			cursor = " \u276F "
		}

		var optStyle lipgloss.Style
		if isHighlighted {
			optStyle = getQuestionSelectedStyle()
		} else {
			optStyle = getQuestionUnselectedStyle()
		}

		optLine := fmt.Sprintf("%s%s %d. %s", cursor, prefix, i+1, opt.Label)
		sb.WriteString(optStyle.Render(optLine))

		if opt.Description != "" {
			sb.WriteString(" - ")
			sb.WriteString(getQuestionDescStyle().Render(opt.Description))
		}
		sb.WriteString("\n")
	}

	// "Other" option
	otherIdx := len(currentQ.Options)
	isOtherHighlighted := curOption == otherIdx

	otherCursor := "   "
	if isOtherHighlighted {
		otherCursor = " \u276F "
	}

	otherStyle := getQuestionUnselectedStyle()
	if isOtherHighlighted {
		otherStyle = getQuestionSelectedStyle()
	}

	otherPrefix := "( )"
	if isMulti {
		otherPrefix = "[ ]"
	}

	sb.WriteString(otherStyle.Render(fmt.Sprintf("%s%s %d. Other", otherCursor, otherPrefix, otherIdx+1)))
	sb.WriteString(" - ")
	sb.WriteString(getQuestionDescStyle().Render("Type custom response"))
	sb.WriteString("\n")

	// Custom input (if showing)
	if p.showingCustom {
		sb.WriteString("\n")
		sb.WriteString("   ")
		sb.WriteString(p.customInput.View())
		sb.WriteString("\n")
	}

	// Show custom answer if already set for this question
	if !p.showingCustom {
		if customText, ok := p.customAnswers[p.currentQuestion]; ok {
			sb.WriteString("   ")
			sb.WriteString(getQuestionDescStyle().Render("answered: " + customText))
			sb.WriteString("\n")
		}
	}

	// Dotted separator
	dottedSep := strings.Repeat("\u254C", contentWidth)
	sb.WriteString(getQuestionSeparatorStyle().Render(dottedSep))
	sb.WriteString("\n")

	// Footer with hints
	var hints []string
	if len(p.request.Questions) > 1 {
		hints = append(hints, "\u2190/\u2192 switch question")
	}
	hints = append(hints, "\u2191/\u2193 navigate")
	if isMulti {
		hints = append(hints, "Space toggle")
	}
	hints = append(hints, "Enter confirm", "Esc cancel")

	footer := " " + strings.Join(hints, " \u00B7 ")
	sb.WriteString(getQuestionFooterStyle().Render(footer))

	return sb.String()
}
