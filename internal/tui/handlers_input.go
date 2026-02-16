package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/message"
)

func (m *model) handleKeypress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.planPrompt != nil && m.planPrompt.IsActive() {
		cmd := m.planPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.questionPrompt.IsActive() {
		cmd := m.questionPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.permissionPrompt.IsActive() {
		cmd := m.permissionPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.enterPlanPrompt.IsActive() {
		cmd := m.enterPlanPrompt.HandleKeypress(msg)
		return m, cmd
	}

	if m.selector.IsActive() {
		cmd := m.selector.HandleKeypress(msg)
		return m, cmd
	}

	if m.toolSelector.IsActive() {
		cmd := m.toolSelector.HandleKeypress(msg)
		return m, cmd
	}

	if m.skillSelector.IsActive() {
		cmd := m.skillSelector.HandleKeypress(msg)
		return m, cmd
	}

	if m.agentSelector.IsActive() {
		cmd := m.agentSelector.HandleKeypress(msg)
		return m, cmd
	}

	if m.mcpSelector.IsActive() {
		cmd := m.mcpSelector.HandleKeypress(msg)
		return m, cmd
	}

	if m.sessionSelector.IsActive() {
		cmd := m.sessionSelector.HandleKeypress(msg)
		return m, cmd
	}

	if m.memorySelector.IsActive() {
		cmd := m.memorySelector.HandleKeypress(msg)
		return m, cmd
	}

	// Image selection mode handling
	if m.imageSelectMode && len(m.pendingImages) > 0 {
		switch msg.Type {
		case tea.KeyLeft:
			if m.selectedImageIdx > 0 {
				m.selectedImageIdx--
			}
			return m, nil
		case tea.KeyRight:
			if m.selectedImageIdx < len(m.pendingImages)-1 {
				m.selectedImageIdx++
			}
			return m, nil
		case tea.KeyDelete, tea.KeyBackspace:
			m.removePendingImage(m.selectedImageIdx)
			return m, nil
		case tea.KeyEsc:
			m.imageSelectMode = false
			return m, nil
		}
	}

	if m.suggestions.IsVisible() {
		switch msg.Type {
		case tea.KeyUp, tea.KeyCtrlP:
			m.suggestions.MoveUp()
			return m, nil
		case tea.KeyDown, tea.KeyCtrlN:
			m.suggestions.MoveDown()
			return m, nil
		case tea.KeyTab, tea.KeyEnter:
			if selected := m.suggestions.GetSelected(); selected != "" {
				if m.suggestions.GetSuggestionType() == SuggestionTypeFile {
					// For @ file suggestions, replace the @query with @selected
					currentValue := m.textarea.Value()
					// Find the last @ and replace from there
					if atIdx := strings.LastIndex(currentValue, "@"); atIdx >= 0 {
						newValue := currentValue[:atIdx] + "@" + selected
						m.textarea.SetValue(newValue)
						m.textarea.CursorEnd()
					}
				} else {
					// For command suggestions
					m.textarea.SetValue(selected + " ")
					m.textarea.CursorEnd()
				}
				m.suggestions.Hide()
			}
			return m, nil
		case tea.KeyEsc:
			m.suggestions.Hide()
			return m, nil
		}
	}

	if msg.Type == tea.KeyShiftTab {
		if !m.streaming && !m.permissionPrompt.IsActive() &&
			!m.questionPrompt.IsActive() &&
			(m.planPrompt == nil || !m.planPrompt.IsActive()) &&
			!m.selector.IsActive() && !m.suggestions.IsVisible() {
			m.cycleOperationMode()
			return m, nil
		}
	}

	if msg.Type == tea.KeyCtrlO {
		return m.handleCtrlO()
	}

	switch msg.Type {
	case tea.KeyCtrlX:
		// Remove pending image: selected one in select mode, or last one otherwise
		if len(m.pendingImages) > 0 {
			if m.imageSelectMode {
				m.removePendingImage(m.selectedImageIdx)
			} else {
				m.removePendingImage(len(m.pendingImages) - 1)
			}
			return m, nil
		}
		// No pending images, let textarea handle it
		return nil, nil

	case tea.KeyCtrlV, tea.KeyCtrlY:
		// Ctrl+V / Ctrl+Y: Paste image from clipboard
		return m.pasteImageFromClipboard()

	case tea.KeyCtrlC:
		if m.textarea.Value() != "" {
			m.textarea.Reset()
			m.textarea.SetHeight(minTextareaHeight)
			m.historyIndex = -1
			return m, nil
		}
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		return m, tea.Quit

	case tea.KeyEsc:
		if m.suggestions.IsVisible() {
			m.suggestions.Hide()
			return m, nil
		}
		if m.streaming && m.cancelFunc != nil {
			return m.handleStreamCancel()
		}
		return m, nil

	case tea.KeyUp:
		if m.textarea.Line() == 0 {
			// Enter image select mode if there are pending images
			if len(m.pendingImages) > 0 && !m.imageSelectMode {
				m.imageSelectMode = true
				m.selectedImageIdx = len(m.pendingImages) - 1 // Select last image
				return m, nil
			}
			return m.handleHistoryUp()
		}

	case tea.KeyDown:
		lines := strings.Count(m.textarea.Value(), "\n")
		if m.textarea.Line() == lines {
			return m.handleHistoryDown()
		}

	case tea.KeyEnter:
		if msg.Alt {
			m.textarea.InsertString("\n")
			m.updateTextareaHeight()
			return m, nil
		}
		return m.handleSubmit()
	}

	// Return nil, nil to let textarea handle the input
	return nil, nil
}

func (m *model) handleCtrlO() (tea.Model, tea.Cmd) {
	// Handle permission prompt preview toggle
	if m.permissionPrompt != nil && m.permissionPrompt.IsActive() {
		m.togglePermissionPreview()
		return m, nil
	}

	now := time.Now()
	if now.Sub(m.lastCtrlOTime) < doubleTapThreshold {
		// Double-tap: toggle all uncommitted expandable items
		anyExpanded := false
		for i := m.committedCount; i < len(m.messages); i++ {
			msg := m.messages[i]
			if (msg.toolResult != nil && msg.expanded) ||
				(len(msg.toolCalls) > 0 && msg.toolCallsExpanded) ||
				(msg.isSummary && msg.expanded) {
				anyExpanded = true
				break
			}
		}
		for i := m.committedCount; i < len(m.messages); i++ {
			if m.messages[i].toolResult != nil {
				m.messages[i].expanded = !anyExpanded
			}
			if len(m.messages[i].toolCalls) > 0 {
				m.messages[i].toolCallsExpanded = !anyExpanded
			}
			if m.messages[i].isSummary {
				m.messages[i].expanded = !anyExpanded
			}
		}
		m.lastCtrlOTime = time.Time{}
		return m, nil
	}

	// Single tap: toggle most recent expandable item
	m.lastCtrlOTime = now
	m.toggleMostRecentExpandable()
	return m, nil
}

// toggleMostRecentExpandable toggles the expansion state of the most recent expandable message.
func (m *model) toggleMostRecentExpandable() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := &m.messages[i]
		switch {
		case msg.isSummary:
			msg.expanded = !msg.expanded
			return
		case msg.toolResult != nil:
			msg.expanded = !msg.expanded
			return
		case len(msg.toolCalls) > 0:
			msg.toolCallsExpanded = !msg.toolCallsExpanded
			return
		}
	}
}

func (m *model) handleStreamCancel() (tea.Model, tea.Cmd) {
	m.cancelFunc()
	m.streaming = false
	m.streamChan = nil
	m.cancelFunc = nil
	m.buildingToolName = ""

	// Cancel pending tool calls
	m.cancelPendingToolCalls()

	// Mark the last assistant message as interrupted
	m.markLastAssistantInterrupted()

	// Commit all messages to scrollback
	return m, tea.Batch(m.commitMessages()...)
}

// cancelPendingToolCalls adds cancellation messages for pending tool calls.
func (m *model) cancelPendingToolCalls() {
	var toolCalls []message.ToolCall

	if m.pendingToolCalls != nil {
		toolCalls = m.pendingToolCalls[m.pendingToolIdx:]
		m.pendingToolCalls = nil
		m.pendingToolIdx = 0
	} else if len(m.messages) > 0 {
		lastMsg := m.messages[len(m.messages)-1]
		if lastMsg.role == roleAssistant {
			toolCalls = lastMsg.toolCalls
		}
	}

	for _, tc := range toolCalls {
		m.messages = append(m.messages, chatMessage{
			role:     roleUser,
			toolName: tc.Name,
			toolResult: &message.ToolResult{
				ToolCallID: tc.ID,
				Content:    "Tool execution cancelled by user",
				IsError:    true,
			},
		})
	}
}

// markLastAssistantInterrupted marks the last assistant message as interrupted if it has no tool calls.
func (m *model) markLastAssistantInterrupted() {
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := &m.messages[i]
		if msg.role != roleAssistant {
			continue
		}
		if len(msg.toolCalls) == 0 {
			if msg.content == "" {
				msg.content = "[Interrupted]"
			} else {
				msg.content += " [Interrupted]"
			}
		}
		return
	}
}

func (m *model) handleHistoryUp() (tea.Model, tea.Cmd) {
	if len(m.inputHistory) == 0 {
		return m, nil
	}
	if m.historyIndex == -1 {
		m.tempInput = m.textarea.Value()
		m.historyIndex = len(m.inputHistory) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}
	m.textarea.SetValue(m.inputHistory[m.historyIndex])
	m.textarea.CursorEnd()
	m.updateTextareaHeight()
	return m, nil
}

func (m *model) handleHistoryDown() (tea.Model, tea.Cmd) {
	if m.historyIndex == -1 {
		return m, nil
	}
	if m.historyIndex < len(m.inputHistory)-1 {
		m.historyIndex++
		m.textarea.SetValue(m.inputHistory[m.historyIndex])
	} else {
		m.historyIndex = -1
		m.textarea.SetValue(m.tempInput)
	}
	m.textarea.CursorEnd()
	m.updateTextareaHeight()
	return m, nil
}

func (m *model) handleSubmit() (tea.Model, tea.Cmd) {
	if m.streaming {
		return m, nil
	}
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" && len(m.pendingImages) == 0 {
		return m, nil
	}

	if strings.ToLower(input) == "exit" {
		if m.cancelFunc != nil {
			m.cancelFunc()
		}
		return m, tea.Quit
	}

	// Execute UserPromptSubmit hook before processing
	if blocked, reason := m.checkPromptHook(input); blocked {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "Prompt blocked: " + reason})
		m.textarea.Reset()
		m.textarea.SetHeight(minTextareaHeight)
		return m, tea.Batch(m.commitMessages()...)
	}

	if input != "" {
		m.inputHistory = append(m.inputHistory, input)
		m.historyIndex = -1
		m.tempInput = ""
		saveInputHistory(m.cwd, m.inputHistory)
	}

	if result, isCmd := ExecuteCommand(context.Background(), m, input); isCmd {
		m.textarea.Reset()
		m.textarea.SetHeight(minTextareaHeight)

		// Handle clear screen command: clear both visible screen and scrollback.
		if m.pendingClearScreen {
			m.pendingClearScreen = false
			// Write escape sequences directly to /dev/tty to bypass BT's renderer.
			// \033[2J clears visible screen, \033[3J clears scrollback, \033[H moves cursor home.
			if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
				tty.WriteString("\033[2J\033[3J\033[H")
				tty.Close()
			}
			// For tmux, clear its own scrollback buffer since \033[3J doesn't affect it.
			if os.Getenv("TMUX") != "" {
				exec.Command("tmux", "clear-history").Run()
			}
			return m, tea.ClearScreen
		}

		// Check if async token limit fetch was started (don't add to messages to avoid polluting main loop)
		if m.fetchingTokenLimits {
			return m, tea.Batch(m.spinner.Tick, startTokenLimitFetch(m))
		}

		// Check if async compact was started
		if m.compacting {
			return m, tea.Batch(m.spinner.Tick, startCompact(m))
		}

		// Check if external editor was requested
		if m.editingMemoryFile != "" {
			return m, startExternalEditor(m.editingMemoryFile)
		}

		// Auto-reconnect disconnected MCP servers when selector opens
		if m.mcpSelector.IsActive() {
			cmds := m.commitMessages()
			if reconnectCmd := m.mcpSelector.autoReconnect(); reconnectCmd != nil {
				cmds = append(cmds, reconnectCmd)
			}
			return m, tea.Batch(cmds...)
		}

		if result != "" {
			m.messages = append(m.messages, chatMessage{role: roleUser, content: input})
			m.messages = append(m.messages, chatMessage{role: roleNotice, content: result})
			return m, tea.Batch(m.commitMessages()...)
		}
		// Check if this was a skill command (empty result with pending args)
		if m.pendingSkillArgs != "" {
			return m.handleSkillInvocation()
		}
		return m, tea.Batch(m.commitMessages()...)
	}

	// Process @image.png references
	content, fileImages, err := m.processImageReferences(input)
	if err != nil {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "Image error: " + err.Error()})
		return m, tea.Batch(m.commitMessages()...)
	}

	// Combine pending clipboard images with file reference images
	allImages := append(m.pendingImages, fileImages...)
	m.pendingImages = nil // Clear pending images

	m.messages = append(m.messages, chatMessage{role: roleUser, content: content, images: allImages})
	m.textarea.Reset()
	m.textarea.SetHeight(minTextareaHeight)

	if m.llmProvider == nil {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "No provider connected. Use /provider to connect."})
		return m, tea.Batch(m.commitMessages()...)
	}

	return m, m.startLLMStream(m.buildExtraContext())
}

func (m *model) handleWindowResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height

	// Update markdown renderer before rendering any content
	m.mdRenderer = createMarkdownRenderer(msg.Width)

	if !m.ready {
		m.ready = true

		var cmds []tea.Cmd

		// If resuming a session with messages, commit them to scrollback
		if len(m.messages) > 0 {
			cmds = append(cmds, m.commitAllMessages()...)
		} else {
			// Print welcome screen
			cmds = append(cmds, tea.Println(m.renderWelcome()))
		}

		// Open session selector if pending (for --resume flag)
		if m.pendingSessionSelector {
			m.pendingSessionSelector = false
			if m.sessionStore != nil {
				_ = m.sessionSelector.EnterSessionSelect(m.width, m.height, m.sessionStore, m.cwd)
			}
		}

		m.textarea.SetWidth(msg.Width - 4 - 2)
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil
	}

	m.textarea.SetWidth(msg.Width - 4 - 2)
	return m, nil
}

// startLLMStream sets up and starts an LLM streaming request with the given extra context.
// It appends an empty assistant message, sets up cancellation, and starts streaming.
func (m *model) startLLMStream(extra []string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel
	m.streaming = true

	// Configure loop with current state and set messages
	m.configureLoop(extra)
	m.loop.SetMessages(m.convertMessagesToProvider())

	// Commit any pending messages before starting stream
	commitCmds := m.commitMessages()

	m.messages = append(m.messages, chatMessage{role: roleAssistant, content: ""})

	m.streamChan = m.loop.Stream(ctx)

	allCmds := append(commitCmds, m.waitForChunk(), m.spinner.Tick)
	return tea.Batch(allCmds...)
}

// handleSkillInvocation handles skill command invocation by sending the skill
// instructions and args to the LLM.
func (m *model) handleSkillInvocation() (tea.Model, tea.Cmd) {
	if m.llmProvider == nil {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "No provider connected. Use /provider to connect."})
		m.pendingSkillInstructions = ""
		m.pendingSkillArgs = ""
		return m, tea.Batch(m.commitMessages()...)
	}

	// Get the user message (skill args or skill name)
	userMessage := m.pendingSkillArgs
	if userMessage == "" {
		userMessage = "Execute the skill."
	}

	m.messages = append(m.messages, chatMessage{role: roleUser, content: userMessage})

	// Build extra context with skill instructions
	extra := m.buildExtraContext()
	if m.pendingSkillInstructions != "" {
		extra = append(extra, m.pendingSkillInstructions)
		m.pendingSkillInstructions = ""
	}
	m.pendingSkillArgs = ""

	return m, m.startLLMStream(extra)
}

// checkPromptHook runs UserPromptSubmit hook and returns (blocked, reason).
func (m *model) checkPromptHook(prompt string) (bool, string) {
	if m.hookEngine == nil {
		return false, ""
	}
	outcome := m.hookEngine.Execute(context.Background(), hooks.UserPromptSubmit, hooks.HookInput{Prompt: prompt})
	return outcome.ShouldBlock, outcome.BlockReason
}

// togglePermissionPreview toggles the expand state of permission prompt previews.
func (m *model) togglePermissionPreview() {
	if m.permissionPrompt.diffPreview != nil {
		m.permissionPrompt.diffPreview.ToggleExpand()
	}
	if m.permissionPrompt.bashPreview != nil {
		m.permissionPrompt.bashPreview.ToggleExpand()
	}
}

// removePendingImage removes the image at the given index from pendingImages
// and adjusts selectedImageIdx accordingly. Exits image select mode if no images remain.
func (m *model) removePendingImage(idx int) {
	if idx < 0 || idx >= len(m.pendingImages) {
		return
	}
	m.pendingImages = append(m.pendingImages[:idx], m.pendingImages[idx+1:]...)
	if m.selectedImageIdx >= len(m.pendingImages) && m.selectedImageIdx > 0 {
		m.selectedImageIdx--
	}
	if len(m.pendingImages) == 0 {
		m.imageSelectMode = false
	}
}

// imageRefPattern matches @path/to/image.ext references
var imageRefPattern = regexp.MustCompile(`@([^\s]+\.(png|jpg|jpeg|gif|webp))`)

// pasteImageFromClipboard handles pasting image from clipboard
func (m *model) pasteImageFromClipboard() (tea.Model, tea.Cmd) {
	imgData, err := image.ReadImageToProviderData()
	if err != nil {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "Image paste error: " + err.Error()})
		return m, tea.Batch(m.commitMessages()...)
	}
	if imgData == nil {
		// No image in clipboard, let textarea handle the key
		return nil, nil
	}
	m.pendingImages = append(m.pendingImages, *imgData)
	return m, nil
}

// processImageReferences extracts @image.png references from input
// Returns the cleaned text content and any loaded images.
// Only processes references where the file actually exists on disk;
// non-existent file references are left in the text as-is.
func (m *model) processImageReferences(input string) (string, []message.ImageData, error) {
	matches := imageRefPattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input, nil, nil
	}

	var images []message.ImageData
	var loadedRefs []string // track which @references were successfully loaded
	for _, match := range matches {
		path := match[1]
		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(m.cwd, absPath)
		}

		// Skip references to files that don't exist
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			continue
		}

		imgInfo, err := image.Load(absPath)
		if err != nil {
			return "", nil, err
		}
		images = append(images, imgInfo.ToProviderData())
		loadedRefs = append(loadedRefs, match[0]) // full match including @
	}

	// Only remove references that were successfully loaded
	content := input
	for _, ref := range loadedRefs {
		content = strings.ReplaceAll(content, ref, "")
	}
	content = strings.TrimSpace(content)

	return content, images, nil
}
