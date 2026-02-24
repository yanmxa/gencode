// Bubble Tea View: composes the terminal UI from active content, input area, and status bar.
package tui

import (
	"fmt"
	"strings"
)

func (m model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	// Render full-screen selectors if any are active
	if selectorView := m.renderActiveSelector(); selectorView != "" {
		return selectorView
	}

	separator := separatorStyle.Render(strings.Repeat("─", m.width))

	todoPrefix := ""
	if todoView := m.renderTodoList(); todoView != "" {
		todoPrefix = strings.TrimSuffix(todoView, "\n") + "\n"
	}

	if m.planPrompt != nil && m.planPrompt.IsActive() {
		planMenu := m.planPrompt.RenderMenu()
		return fmt.Sprintf("%s%s\n%s\n%s", todoPrefix, separator, planMenu, separator)
	}

	if m.permissionPrompt.IsActive() {
		return fmt.Sprintf("%s%s\n%s", todoPrefix, separator, m.permissionPrompt.Render())
	}

	if m.questionPrompt.IsActive() {
		return fmt.Sprintf("%s%s\n%s", todoPrefix, separator, m.questionPrompt.Render())
	}

	if m.enterPlanPrompt.IsActive() {
		return fmt.Sprintf("%s%s\n%s", todoPrefix, separator, m.enterPlanPrompt.Render())
	}

	activeContent := m.renderActiveContent()

	prompt := inputPromptStyle.Render("❯ ")
	pendingImagesView := m.renderPendingImages()
	inputView := prompt + m.textarea.View()

	var parts []string

	if activeContent != "" {
		parts = append(parts, activeContent)
	}

	if todoView := m.renderTodoList(); todoView != "" {
		parts = append(parts, strings.TrimSuffix(todoView, "\n"))
	}

	if pendingImagesView != "" {
		parts = append(parts, strings.TrimSuffix(pendingImagesView, "\n"))
	}

	if m.fetchingTokenLimits {
		spinnerView := thinkingStyle.Render(m.spinner.View() + " Fetching token limits...")
		parts = append(parts, spinnerView)
	}

	if m.compacting {
		spinnerView := thinkingStyle.Render(m.spinner.View() + " Compacting conversation...")
		parts = append(parts, spinnerView)
	}

	chatSection := strings.Join(parts, "\n")

	statusLine := m.renderModeStatus()
	suggestions := m.suggestions.Render(m.width)

	var view strings.Builder
	if chatSection != "" {
		view.WriteString(chatSection)
		view.WriteString("\n")
	} else if m.committedCount > 0 {
		view.WriteString("\n")
	}
	view.WriteString(separator)
	view.WriteString("\n")
	view.WriteString(inputView)
	if suggestions != "" {
		view.WriteString("\n")
		view.WriteString(suggestions)
	}
	view.WriteString("\n")
	view.WriteString(separator)
	view.WriteString("\n")
	if statusLine != "" {
		view.WriteString(statusLine)
	} else {
		view.WriteString(" ")
	}

	return view.String()
}

// renderActiveSelector returns the view for any active full-screen selector, or empty string if none.
func (m model) renderActiveSelector() string {
	switch {
	case m.selector.IsActive():
		return m.selector.Render()
	case m.toolSelector.IsActive():
		return m.toolSelector.Render()
	case m.skillSelector.IsActive():
		return m.skillSelector.Render()
	case m.agentSelector.IsActive():
		return m.agentSelector.Render()
	case m.mcpSelector.IsActive():
		return m.mcpSelector.Render()
	case m.pluginSelector.IsActive():
		return m.pluginSelector.Render()
	case m.sessionSelector.IsActive():
		return m.sessionSelector.Render()
	case m.memorySelector.IsActive():
		return m.memorySelector.Render()
	default:
		return ""
	}
}
