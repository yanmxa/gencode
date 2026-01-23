package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/myan/gencode/internal/tool/permission"
)

// PermissionPrompt manages the permission request UI with Claude Code style.
type PermissionPrompt struct {
	active       bool
	request      *permission.PermissionRequest
	diffPreview  *DiffPreview
	bashPreview  *BashPreview
	width        int
	selectedIdx  int // Current menu selection (0=Yes, 1=Yes all, 2=No)
}

// NewPermissionPrompt creates a new PermissionPrompt instance
func NewPermissionPrompt() *PermissionPrompt {
	return &PermissionPrompt{
		selectedIdx: 0, // Default to "Yes"
	}
}

// Show displays the permission prompt with the given request
func (p *PermissionPrompt) Show(req *permission.PermissionRequest, width, height int) {
	p.active = true
	p.request = req
	p.width = width
	p.selectedIdx = 0 // Reset to "Yes"

	if req.DiffMeta != nil {
		p.diffPreview = NewDiffPreview(req.DiffMeta)
	} else {
		p.diffPreview = nil
	}

	if req.BashMeta != nil {
		p.bashPreview = NewBashPreview(req.BashMeta)
	} else {
		p.bashPreview = nil
	}
}

// Hide hides the permission prompt
func (p *PermissionPrompt) Hide() {
	p.active = false
	p.request = nil
	p.diffPreview = nil
	p.bashPreview = nil
}

// IsActive returns whether the prompt is visible
func (p *PermissionPrompt) IsActive() bool {
	return p.active
}

// GetRequest returns the current permission request
func (p *PermissionPrompt) GetRequest() *permission.PermissionRequest {
	return p.request
}

// Message types for permission responses
type (
	// PermissionRequestMsg is sent when a tool needs permission
	PermissionRequestMsg struct {
		Request  *permission.PermissionRequest
		ToolCall interface{} // The original tool call
	}

	// PermissionResponseMsg is sent when the user responds to a permission request
	PermissionResponseMsg struct {
		Approved   bool
		AllowAll   bool // True if user selected "allow all during session"
		Request    *permission.PermissionRequest
	}
)

// HandleKeypress handles keyboard input for the permission prompt
func (p *PermissionPrompt) HandleKeypress(msg tea.KeyMsg) tea.Cmd {
	if !p.active {
		return nil
	}

	switch msg.Type {
	case tea.KeyUp, tea.KeyCtrlP:
		// Move selection up
		if p.selectedIdx > 0 {
			p.selectedIdx--
		}
		return nil

	case tea.KeyDown, tea.KeyCtrlN:
		// Move selection down
		if p.selectedIdx < 2 {
			p.selectedIdx++
		}
		return nil

	case tea.KeyEnter:
		// Confirm selection
		return p.confirmSelection()

	case tea.KeyShiftTab:
		// Quick "allow all" option
		req := p.request
		p.Hide()
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: true, AllowAll: true, Request: req}
		}

	case tea.KeyCtrlO:
		// Toggle diff or bash preview expansion
		if p.diffPreview != nil {
			p.diffPreview.ToggleExpand()
		}
		if p.bashPreview != nil {
			p.bashPreview.ToggleExpand()
		}
		return nil

	case tea.KeyEsc:
		// Cancel/Deny
		req := p.request
		p.Hide()
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: false, Request: req}
		}
	}

	// Handle number key shortcuts
	switch msg.String() {
	case "1", "y", "Y":
		// Approve single
		req := p.request
		p.Hide()
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: true, Request: req}
		}

	case "2":
		// Approve all for session
		req := p.request
		p.Hide()
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: true, AllowAll: true, Request: req}
		}

	case "3", "n", "N":
		// Deny
		req := p.request
		p.Hide()
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: false, Request: req}
		}
	}

	return nil
}

// confirmSelection confirms the currently selected menu option
func (p *PermissionPrompt) confirmSelection() tea.Cmd {
	req := p.request
	p.Hide()

	switch p.selectedIdx {
	case 0: // Yes
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: true, Request: req}
		}
	case 1: // Yes, allow all
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: true, AllowAll: true, Request: req}
		}
	case 2: // No
		return func() tea.Msg {
			return PermissionResponseMsg{Approved: false, Request: req}
		}
	}
	return nil
}

// Styles for permission prompt
var (
	// Separator styles
	solidSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4B5563")) // gray-600

	dottedSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280")) // gray-500

	// Description style
	promptDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D1D5DB")) // gray-300

	// Question style
	promptQuestionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF")) // gray-400

	// Menu styles
	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#10B981")). // green
				Bold(true)

	menuUnselectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280")) // gray-500

	menuHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4B5563")).
			Italic(true)

	// Footer style
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))
)

// RenderInline renders the permission prompt inline with Claude Code style
func (p *PermissionPrompt) RenderInline() string {
	if !p.active || p.request == nil {
		return ""
	}

	var sb strings.Builder
	contentWidth := p.width - 2
	if contentWidth < 40 {
		contentWidth = 40
	}

	// No header needed - tool call header (⚡Write(file.txt)) is already shown above
	// Start directly with separator
	solidSep := strings.Repeat("─", contentWidth)
	sb.WriteString(solidSeparatorStyle.Render(solidSep))
	sb.WriteString("\n")

	// Description with filename (e.g., "Create new file: world.txt")
	description := p.getDescription()
	sb.WriteString(" ")
	sb.WriteString(promptDescStyle.Render(description))
	sb.WriteString("\n")

	// Dotted separator before content
	dottedSep := strings.Repeat("╌", contentWidth)
	sb.WriteString(dottedSeparatorStyle.Render(dottedSep))
	sb.WriteString("\n")

	// Diff preview, Bash preview, or content preview
	if p.diffPreview != nil {
		sb.WriteString(p.diffPreview.Render(contentWidth))
	} else if p.bashPreview != nil {
		sb.WriteString(p.bashPreview.Render(contentWidth))
	}

	// Dotted separator after content
	sb.WriteString(dottedSeparatorStyle.Render(dottedSep))
	sb.WriteString("\n")

	// Question
	question := p.getQuestion()
	sb.WriteString(" ")
	sb.WriteString(promptQuestionStyle.Render(question))
	sb.WriteString("\n")

	// Menu options
	sb.WriteString(p.renderMenu())
	sb.WriteString("\n")

	// Footer
	footer := " Esc to cancel"
	hasExpandableContent := (p.diffPreview != nil && len(p.diffPreview.diffMeta.Lines) > DefaultMaxVisibleLines) ||
		(p.bashPreview != nil && p.bashPreview.NeedsExpand())
	if hasExpandableContent {
		footer += " · Ctrl+O to expand/collapse"
	}
	sb.WriteString(footerStyle.Render(footer))

	return sb.String()
}

// getDescription returns the action description with filename
func (p *PermissionPrompt) getDescription() string {
	// Extract filename from full path
	filename := p.request.FilePath
	if idx := strings.LastIndex(filename, "/"); idx >= 0 {
		filename = filename[idx+1:]
	}

	switch p.request.ToolName {
	case "Edit":
		return "Edit file: " + filename
	case "Write":
		if p.request.DiffMeta != nil && p.request.DiffMeta.IsNewFile {
			return "Create new file: " + filename
		}
		return "Overwrite file: " + filename
	case "Bash":
		return "Execute command"
	default:
		if filename != "" {
			return p.request.Description + ": " + filename
		}
		return p.request.Description
	}
}

// getQuestion returns the confirmation question
func (p *PermissionPrompt) getQuestion() string {
	switch p.request.ToolName {
	case "Edit":
		return "Allow this edit?"
	case "Write":
		if p.request.DiffMeta != nil && p.request.DiffMeta.IsNewFile {
			return "Allow creating this file?"
		}
		return "Allow overwriting this file?"
	case "Bash":
		return "Allow running this command?"
	default:
		return "Allow this action?"
	}
}

// getAllSessionLabel returns the "allow all" label for this tool
func (p *PermissionPrompt) getAllSessionLabel() string {
	switch p.request.ToolName {
	case "Edit":
		return "Yes, allow all edits during this session"
	case "Write":
		return "Yes, allow all writes during this session"
	case "Bash":
		return "Yes, allow all commands during this session"
	default:
		return "Yes, allow all during this session"
	}
}

// renderMenu renders the selection menu
func (p *PermissionPrompt) renderMenu() string {
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
			sb.WriteString(menuSelectedStyle.Render(fmt.Sprintf(" ❯ %d. %s", i+1, opt.label)))
		} else {
			sb.WriteString(menuUnselectedStyle.Render(fmt.Sprintf("   %d. %s", i+1, opt.label)))
		}
		if opt.hint != "" {
			sb.WriteString(" ")
			sb.WriteString(menuHintStyle.Render(opt.hint))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// Render renders the permission prompt (calls RenderInline)
func (p *PermissionPrompt) Render() string {
	return p.RenderInline()
}
