package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/permission"
	toolui "github.com/yanmxa/gencode/internal/tool/ui"
)

// Run starts the TUI application
func Run() error {
	p := tea.NewProgram(
		newModel(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

// Styles
var (
	mutedColor    = lipgloss.Color("#6B7280") // muted for placeholder
	accentColor   = lipgloss.Color("#F59E0B") // accent for spinner
	primaryColor  = lipgloss.Color("#60A5FA") // blue for user prompt
	aiColor       = lipgloss.Color("#A78BFA") // purple for AI

	userMsgStyle = lipgloss.NewStyle()

	assistantMsgStyle = lipgloss.NewStyle()

	inputPromptStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	aiPromptStyle = lipgloss.NewStyle().
			Foreground(aiColor).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Faint(true).
			Foreground(lipgloss.Color("240"))

	thinkingStyle = lipgloss.NewStyle().
			Foreground(accentColor)

	systemMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9CA3AF")).
			PaddingLeft(2)
)

type chatMessage struct {
	role              string
	content           string
	toolCalls         []provider.ToolCall        // For assistant messages with tool calls
	toolResult        *provider.ToolResult       // For tool result messages
	toolName          string                     // Tool name for tool result display
	expanded          bool                       // Whether tool result is expanded
	pendingPermission *permission.PermissionRequest // Pending permission request (inline display)
}

type (
	streamChunkMsg struct {
		text       string
		done       bool
		err        error
		toolCalls  []provider.ToolCall // Tool calls when done
		stopReason string              // Stop reason (end_turn, tool_use, etc.)
	}
	streamDoneMsg   struct{}
	toolExecutedMsg struct {
		results   []provider.ToolResult
		toolNames []string // Tool names for display
	}
	// todoUpdateMsg is sent when TodoWrite tool updates the todo list
	todoUpdateMsg struct {
		todos []toolui.TodoItem
	}
)

type model struct {
	viewport     viewport.Model
	textarea     textarea.Model
	spinner      spinner.Model
	messages     []chatMessage
	llmProvider  provider.LLMProvider
	store        *provider.Store
	currentModel *provider.CurrentModelInfo
	streaming    bool
	streamChan   <-chan provider.StreamChunk
	cancelFunc   context.CancelFunc
	width        int
	height       int
	ready        bool
	inputHistory []string
	historyIndex int
	tempInput    string
	cwd          string // Current working directory for tools

	// Markdown renderer
	mdRenderer *glamour.TermRenderer

	// Tool call tracking
	currentToolID    string // Current tool call ID being accumulated
	currentToolName  string // Current tool name
	currentToolInput string // Accumulated tool input JSON

	// Tool result expansion
	selectedToolIdx int // Index of selected tool result for expansion (-1 = none)

	// Command suggestions
	suggestions SuggestionState

	// Provider/Model selector
	selector SelectorState

	// Permission prompt
	permissionPrompt *PermissionPrompt
	pendingToolCalls []provider.ToolCall
	pendingToolIdx   int

	// Settings and session permissions
	settings           *config.Settings
	sessionPermissions *config.SessionPermissions

	// Todo panel (persistent display)
	todoPanel *TodoPanel

	// Question prompt (interactive)
	questionPrompt  *QuestionPrompt
	pendingQuestion *tool.QuestionRequest
}

// createMarkdownRenderer creates a glamour renderer with the specified width and compact styling
func createMarkdownRenderer(width int) *glamour.TermRenderer {
	// Leave some margin for padding
	wrapWidth := width - 4
	if wrapWidth < 40 {
		wrapWidth = 40
	}

	// Create compact style based on DarkStyleConfig with reduced margins
	compactStyle := styles.DarkStyleConfig

	// Helpers for pointers
	uintPtr := func(u uint) *uint { return &u }
	stringPtr := func(s string) *string { return &s }

	// Remove document margins and block prefix/suffix newlines
	compactStyle.Document = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color: compactStyle.Document.Color,
		},
		Margin: uintPtr(0),
	}

	// Remove paragraph margins
	compactStyle.Paragraph = ansi.StyleBlock{
		Margin: uintPtr(0),
	}

	// Remove heading extra newlines
	compactStyle.Heading = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color: stringPtr("39"),
			Bold:  func(b bool) *bool { return &b }(true),
		},
	}

	// Remove code block margins
	compactStyle.CodeBlock.Margin = uintPtr(0)

	// Compact horizontal rule
	compactStyle.HorizontalRule = ansi.StylePrimitive{
		Color:  stringPtr("240"),
		Format: "--------",
	}

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStyles(compactStyle),
		glamour.WithWordWrap(wrapWidth),
	)
	return renderer
}

func newModel() model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.Focus()
	ta.Prompt = ""                // No prompt per line, we render it manually
	ta.CharLimit = 0              // No character limit (0 = unlimited)
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle().Foreground(mutedColor)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(mutedColor)
	ta.KeyMap.InsertNewline.SetEnabled(true)

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    80 * time.Millisecond,
	}
	sp.Style = thinkingStyle

	// Load store and try to get connected provider
	store, _ := provider.NewStore()
	var llmProvider provider.LLMProvider
	var currentModel *provider.CurrentModelInfo

	if store != nil {
		currentModel = store.GetCurrentModel()
		ctx := context.Background()

		// Priority 1: Use the provider that matches the current model
		if currentModel != nil {
			p, err := provider.GetProvider(ctx, currentModel.Provider, currentModel.AuthMethod)
			if err == nil {
				llmProvider = p
			}
		}

		// Priority 2: Fall back to first available provider
		if llmProvider == nil {
			for providerName, conn := range store.GetConnections() {
				p, err := provider.GetProvider(ctx, provider.Provider(providerName), conn.AuthMethod)
				if err == nil {
					llmProvider = p
					break
				}
			}
		}
	}

	// Get current working directory
	cwd, _ := os.Getwd()

	// Initialize markdown renderer with default width (will be updated on WindowSizeMsg)
	mdRenderer := createMarkdownRenderer(80)

	// Load settings from multi-level config files
	settings, _ := config.Load()
	if settings == nil {
		settings = config.Default()
	}

	return model{
		textarea:           ta,
		spinner:            sp,
		messages:           []chatMessage{},
		llmProvider:        llmProvider,
		store:              store,
		currentModel:       currentModel,
		inputHistory:       []string{},
		historyIndex:       -1,
		cwd:                cwd,
		mdRenderer:         mdRenderer,
		suggestions:        NewSuggestionState(),
		selector:           NewSelectorState(),
		permissionPrompt:   NewPermissionPrompt(),
		settings:           settings,
		sessionPermissions: config.NewSessionPermissions(),
		todoPanel:          NewTodoPanel(),
		questionPrompt:     NewQuestionPrompt(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m *model) updateTextareaHeight() {
	content := m.textarea.Value()
	lines := strings.Count(content, "\n") + 1

	maxHeight := 6
	minHeight := 1

	newHeight := lines
	if newHeight < minHeight {
		newHeight = minHeight
	}
	if newHeight > maxHeight {
		newHeight = maxHeight
	}

	m.textarea.SetHeight(newHeight)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	// Handle inline provider connection result (stays in selector)
	case ProviderConnectResultMsg:
		m.selector.HandleConnectResult(msg)
		return m, nil

	// Handle provider selection result (legacy, exits selector)
	case ProviderSelectedMsg:
		ctx := context.Background()
		result, err := m.selector.ConnectProvider(ctx, msg.Provider, msg.AuthMethod)
		if err != nil {
			m.messages = append(m.messages, chatMessage{role: "system", content: "Error: " + err.Error()})
		} else {
			m.messages = append(m.messages, chatMessage{role: "system", content: result})
			// Update provider in memory
			if p, err := provider.GetProvider(ctx, msg.Provider, msg.AuthMethod); err == nil {
				m.llmProvider = p
			}
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case SelectorCancelledMsg:
		// Just close the selector, no message needed
		return m, nil

	// Handle model selection result
	case ModelSelectedMsg:
		result, err := m.selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
		if err != nil {
			m.messages = append(m.messages, chatMessage{role: "system", content: "Error: " + err.Error()})
		} else {
			m.messages = append(m.messages, chatMessage{role: "system", content: result})
			// Update current model in memory
			m.currentModel = &provider.CurrentModelInfo{
				ModelID:    msg.ModelID,
				Provider:   provider.Provider(msg.ProviderName),
				AuthMethod: msg.AuthMethod,
			}
			// Update provider to match the new model
			ctx := context.Background()
			if p, err := provider.GetProvider(ctx, provider.Provider(msg.ProviderName), msg.AuthMethod); err == nil {
				m.llmProvider = p
			}
		}
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	// Handle permission request from tool
	case PermissionRequestMsg:
		// Add a message with pending permission for inline display
		m.messages = append(m.messages, chatMessage{
			role:              "permission",
			toolName:          msg.Request.ToolName,
			pendingPermission: msg.Request,
		})
		m.permissionPrompt.Show(msg.Request, m.width, m.height)
		// Update viewport to show permission prompt in chat
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	// Handle permission response (user approved/denied)
	case PermissionResponseMsg:
		// Find and update the pending permission message
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].pendingPermission != nil {
				// Clear the pending permission (no longer pending)
				m.messages[i].pendingPermission = nil
				break
			}
		}

		if msg.Approved {
			// If user selected "allow all", update session permissions
			if msg.AllowAll && m.sessionPermissions != nil && msg.Request != nil {
				toolName := msg.Request.ToolName
				switch toolName {
				case "Edit":
					m.sessionPermissions.AllowAllEdits = true
				case "Write":
					m.sessionPermissions.AllowAllWrites = true
				case "Bash":
					m.sessionPermissions.AllowAllBash = true
				default:
					m.sessionPermissions.AllowTool(toolName)
				}
			}
			// Execute the approved tool
			m.viewport.SetContent(m.renderMessages())
			return m, executeApprovedTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd)
		} else {
			// Tool was denied - stop processing, don't continue to LLM
			tc := m.pendingToolCalls[m.pendingToolIdx]
			// Find the permission message and convert it to a tool result (for display only)
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].role == "permission" && m.messages[i].toolName == tc.Name {
					m.messages[i].role = "user"
					m.messages[i].toolResult = &provider.ToolResult{
						ToolCallID: tc.ID,
						Content:    "User denied permission",
						IsError:    true,
					}
					break
				}
			}
			// Clear all pending tools - stop the flow completely
			m.pendingToolCalls = nil
			m.pendingToolIdx = 0
			m.streaming = false
			m.viewport.SetContent(m.renderMessages())
			// Don't call processNextTool or continueWithToolResults - wait for user input
			return m, nil
		}

	// Handle TodoWrite tool result (with todo update)
	case todoResultMsg:
		r := msg.result
		m.messages = append(m.messages, chatMessage{
			role:       "user",
			toolResult: &r,
			toolName:   msg.toolName,
		})
		// Update the todo panel
		m.todoPanel.Update(msg.todos)
		m.todoPanel.SetWidth(m.width)
		m.pendingToolIdx++
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)

	// Handle single tool result
	case toolResultMsg:
		r := msg.result
		// Check if there's a pending permission message to update
		updated := false
		for i := len(m.messages) - 1; i >= 0; i-- {
			if m.messages[i].role == "permission" && m.messages[i].toolName == msg.toolName {
				// Convert permission message to tool result
				m.messages[i].role = "user"
				m.messages[i].toolResult = &r
				m.messages[i].pendingPermission = nil
				updated = true
				break
			}
		}
		// If no permission message found, add as new message
		if !updated {
			m.messages = append(m.messages, chatMessage{
				role:       "user",
				toolResult: &r,
				toolName:   msg.toolName,
			})
		}
		m.pendingToolIdx++
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)

	// Handle start of tool execution
	case startToolExecutionMsg:
		m.pendingToolCalls = msg.toolCalls
		m.pendingToolIdx = 0
		return m, processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)

	// Handle all tools completed
	case allToolsCompletedMsg:
		m.pendingToolCalls = nil
		m.pendingToolIdx = 0
		return m, m.continueWithToolResults()

	// Handle question request from AskUserQuestion tool
	case QuestionRequestMsg:
		m.pendingQuestion = msg.Request
		m.questionPrompt.Show(msg.Request, m.width)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	// Handle question response (user answered or cancelled)
	case QuestionResponseMsg:
		if msg.Cancelled {
			// User cancelled - create error result
			tc := m.pendingToolCalls[m.pendingToolIdx]
			m.messages = append(m.messages, chatMessage{
				role:     "user",
				toolName: tc.Name,
				toolResult: &provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "User cancelled the question prompt",
					IsError:    true,
				},
			})
			m.pendingToolCalls = nil
			m.pendingToolIdx = 0
			m.pendingQuestion = nil
			m.streaming = false
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}

		// Execute the tool with the response
		tc := m.pendingToolCalls[m.pendingToolIdx]
		m.pendingQuestion = nil
		return m, executeInteractiveTool(tc, msg.Response, m.cwd)

	// Handle todo update (from TodoWrite tool result)
	case todoUpdateMsg:
		m.todoPanel.Update(msg.todos)
		m.todoPanel.SetWidth(m.width)
		return m, nil

	case tea.KeyMsg:
		// Handle question prompt first (highest priority)
		if m.questionPrompt.IsActive() {
			cmd := m.questionPrompt.HandleKeypress(msg)
			return m, cmd
		}

		// Handle permission prompt
		if m.permissionPrompt.IsActive() {
			cmd := m.permissionPrompt.HandleKeypress(msg)
			return m, cmd
		}

		// Handle selector mode
		if m.selector.IsActive() {
			cmd := m.selector.HandleKeypress(msg)
			return m, cmd
		}

		// Handle suggestions navigation
		if m.suggestions.IsVisible() {
			switch msg.Type {
			case tea.KeyUp, tea.KeyCtrlP:
				m.suggestions.MoveUp()
				return m, nil
			case tea.KeyDown, tea.KeyCtrlN:
				m.suggestions.MoveDown()
				return m, nil
			case tea.KeyTab, tea.KeyEnter:
				// Complete the selected command
				if selected := m.suggestions.GetSelected(); selected != "" {
					m.textarea.SetValue(selected + " ")
					m.textarea.CursorEnd()
					m.suggestions.Hide()
				}
				return m, nil
			case tea.KeyEsc:
				m.suggestions.Hide()
				return m, nil
			}
		}

		// Handle Ctrl+O to toggle expansion
		if msg.Type == tea.KeyCtrlO {
			// If permission prompt is active, toggle preview expansion (diff or bash)
			if m.permissionPrompt != nil && m.permissionPrompt.IsActive() {
				if m.permissionPrompt.diffPreview != nil {
					m.permissionPrompt.diffPreview.ToggleExpand()
				}
				if m.permissionPrompt.bashPreview != nil {
					m.permissionPrompt.bashPreview.ToggleExpand()
				}
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				return m, nil
			}
			// Otherwise, find the last tool result message and toggle its expansion
			for i := len(m.messages) - 1; i >= 0; i-- {
				if m.messages[i].toolResult != nil {
					m.messages[i].expanded = !m.messages[i].expanded
					m.viewport.SetContent(m.renderMessages())
					return m, nil
				}
			}
			return m, nil
		}

		switch msg.Type {
		case tea.KeyCtrlC:
			if m.textarea.Value() != "" {
				m.textarea.Reset()
				m.textarea.SetHeight(1)
				m.historyIndex = -1
				return m, nil
			}
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, tea.Quit

		case tea.KeyEsc:
			// Hide suggestions if visible
			if m.suggestions.IsVisible() {
				m.suggestions.Hide()
				return m, nil
			}
			if m.streaming && m.cancelFunc != nil {
				m.cancelFunc()
				m.streaming = false
				m.streamChan = nil
				m.cancelFunc = nil
				if len(m.messages) > 0 {
					idx := len(m.messages) - 1
					if m.messages[idx].content == "" {
						m.messages[idx].content = "[Interrupted]"
					} else {
						m.messages[idx].content += " [Interrupted]"
					}
				}
				m.viewport.SetContent(m.renderMessages())
				return m, nil
			}
			return m, nil

		case tea.KeyUp:
			if m.textarea.Line() == 0 {
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

		case tea.KeyDown:
			lines := strings.Count(m.textarea.Value(), "\n")
			if m.textarea.Line() == lines {
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

		case tea.KeyEnter:
			// Alt+Enter for newline
			if msg.Alt {
				m.textarea.InsertString("\n")
				m.updateTextareaHeight()
				return m, nil
			}

			if m.streaming {
				return m, nil
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}

			if strings.ToLower(input) == "exit" {
				if m.cancelFunc != nil {
					m.cancelFunc()
				}
				return m, tea.Quit
			}

			m.inputHistory = append(m.inputHistory, input)
			m.historyIndex = -1
			m.tempInput = ""

			// Check for slash commands
			if result, isCmd := ExecuteCommand(context.Background(), &m, input); isCmd {
				m.textarea.Reset()
				m.textarea.SetHeight(1)
				// Only add messages if result is not empty (e.g., /clear returns empty)
				if result != "" {
					m.messages = append(m.messages, chatMessage{role: "user", content: input})
					m.messages = append(m.messages, chatMessage{role: "system", content: result})
				}
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				return m, nil
			}

			m.messages = append(m.messages, chatMessage{role: "user", content: input})
			m.textarea.Reset()
			m.textarea.SetHeight(1)

			// Check if provider is connected
			if m.llmProvider == nil {
				m.messages = append(m.messages, chatMessage{role: "system", content: "No provider connected. Use /provider to connect."})
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				return m, nil
			}

			m.streaming = true

			ctx, cancel := context.WithCancel(context.Background())
			m.cancelFunc = cancel

			// Convert messages BEFORE adding the empty assistant placeholder
			// to avoid sending empty text blocks to the API
			providerMsgs := m.convertMessagesToProvider()

			// Add empty assistant message for UI streaming display
			m.messages = append(m.messages, chatMessage{role: "assistant", content: ""})
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
			modelID := m.getModelID()

			// Build system prompt
			sysPrompt := system.Prompt(system.Config{
				Provider: m.llmProvider.Name(),
				Model:    modelID,
				Cwd:      m.cwd,
				IsGit:    isGitRepo(m.cwd),
			})

			m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
				Model:        modelID,
				Messages:     providerMsgs,
				MaxTokens:    8192,
				Tools:        tool.GetToolSchemas(),
				SystemPrompt: sysPrompt,
			})
			return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// 2 separator lines + input area (up to 6 lines) + some padding
		inputH := 8
		separatorH := 2
		chatH := msg.Height - inputH - separatorH

		if !m.ready {
			m.viewport = viewport.New(msg.Width, chatH)
			m.viewport.SetContent(m.renderWelcome())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = chatH
		}
		m.textarea.SetWidth(msg.Width - 4 - 2) // -2 for "❯ " prompt

		// Update markdown renderer with new width
		m.mdRenderer = createMarkdownRenderer(msg.Width)

	case streamChunkMsg:
		if msg.done {
			// Check if we have tool calls to execute
			if len(msg.toolCalls) > 0 {
				// Save tool calls to the current assistant message
				if len(m.messages) > 0 {
					idx := len(m.messages) - 1
					m.messages[idx].toolCalls = msg.toolCalls
				}
				m.viewport.SetContent(m.renderMessages())

				// Execute tools and continue conversation
				return m, m.executeTools(msg.toolCalls)
			}

			// Normal completion - no tool calls
			m.streaming = false
			m.streamChan = nil
			m.cancelFunc = nil
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}
		if msg.err != nil {
			if len(m.messages) > 0 {
				idx := len(m.messages) - 1
				m.messages[idx].content += "\n[Error: " + msg.err.Error() + "]"
			}
			m.streaming = false
			m.streamChan = nil
			m.cancelFunc = nil
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}
		if len(m.messages) > 0 && msg.text != "" {
			idx := len(m.messages) - 1
			m.messages[idx].content += msg.text
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()
		}
		return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)

	case toolExecutedMsg:
		// Add tool results to messages with tool names
		for i, result := range msg.results {
			toolName := ""
			if i < len(msg.toolNames) {
				toolName = msg.toolNames[i]
			}
			r := result // Copy to avoid pointer issues
			m.messages = append(m.messages, chatMessage{
				role:       "user",
				toolResult: &r,
				toolName:   toolName,
				expanded:   false,
			})
		}

		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		// Continue the conversation with tool results
		return m, m.continueWithToolResults()

	case streamContinueMsg:
		// Continue streaming after tool execution
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelFunc = cancel

		// Add new assistant message placeholder
		m.messages = append(m.messages, chatMessage{role: "assistant", content: ""})
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		// Build system prompt
		sysPrompt := system.Prompt(system.Config{
			Provider: m.llmProvider.Name(),
			Model:    msg.modelID,
			Cwd:      m.cwd,
			IsGit:    isGitRepo(m.cwd),
		})

		// Start streaming with tools
		m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
			Model:        msg.modelID,
			Messages:     msg.messages,
			MaxTokens:    8192,
			Tools:        tool.GetToolSchemas(),
			SystemPrompt: sysPrompt,
		})
		return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)

	case streamDoneMsg:
		m.streaming = false
		m.streamChan = nil
		m.cancelFunc = nil
		m.viewport.SetContent(m.renderMessages())

	case spinner.TickMsg:
		// Update spinner and re-render viewport to show animation
		if m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.viewport.SetContent(m.renderMessages())
			return m, cmd
		}
	}

	var cmd tea.Cmd
	prevValue := m.textarea.Value()
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	if m.textarea.Value() != prevValue {
		m.updateTextareaHeight()
		// Update command suggestions based on input
		m.suggestions.UpdateSuggestions(m.textarea.Value())
	}

	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.streaming {
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// executeTools executes the given tool calls and returns a command
// For permission-aware tools, it initiates the permission flow
func (m model) executeTools(toolCalls []provider.ToolCall) tea.Cmd {
	return func() tea.Msg {
		return startToolExecutionMsg{toolCalls: toolCalls}
	}
}

// startToolExecutionMsg initiates tool execution
type startToolExecutionMsg struct {
	toolCalls []provider.ToolCall
}

// processNextTool processes the next tool in the pending queue
// It checks permissions based on settings and session permissions before execution.
func processNextTool(toolCalls []provider.ToolCall, idx int, cwd string, settings *config.Settings, sessionPerms *config.SessionPermissions) tea.Cmd {
	// Check if we've processed all tools
	if idx >= len(toolCalls) {
		// All tools processed - signal completion
		return func() tea.Msg {
			return allToolsCompletedMsg{}
		}
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx := context.Background()

		// Parse the tool input JSON
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Error parsing tool input: " + err.Error(),
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

		// Check if tool requires permission
		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Unknown tool: " + tc.Name,
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

		// Check permissions from settings and session
		if settings != nil {
			permResult := settings.CheckPermission(tc.Name, params, sessionPerms)
			switch permResult {
			case config.PermissionAllow:
				// Auto-allowed by settings or session - execute directly
				result := tool.Execute(ctx, tc.Name, params, cwd)
				return toolResultMsg{
					result: provider.ToolResult{
						ToolCallID: tc.ID,
						Content:    result.FormatForLLM(),
						IsError:    !result.Success,
					},
					toolName: tc.Name,
				}

			case config.PermissionDeny:
				// Auto-denied by settings
				return toolResultMsg{
					result: provider.ToolResult{
						ToolCallID: tc.ID,
						Content:    "Permission denied by settings",
						IsError:    true,
					},
					toolName: tc.Name,
				}

			case config.PermissionAsk:
				// Fall through to permission request below
			}
		}

		// Check if it's an interactive tool (like AskUserQuestion)
		if it, ok := t.(tool.InteractiveTool); ok && it.RequiresInteraction() {
			// Prepare interaction request
			req, err := it.PrepareInteraction(ctx, params, cwd)
			if err != nil {
				return toolResultMsg{
					result: provider.ToolResult{
						ToolCallID: tc.ID,
						Content:    "Error: " + err.Error(),
						IsError:    true,
					},
					toolName: tc.Name,
				}
			}
			// Return question request for TUI to handle
			if qr, ok := req.(*tool.QuestionRequest); ok {
				return QuestionRequestMsg{Request: qr}
			}
		}

		// Check if it's a permission-aware tool
		if pat, ok := t.(tool.PermissionAwareTool); ok && pat.RequiresPermission() {
			// Prepare permission request
			req, err := pat.PreparePermission(ctx, params, cwd)
			if err != nil {
				return toolResultMsg{
					result: provider.ToolResult{
						ToolCallID: tc.ID,
						Content:    "Error: " + err.Error(),
						IsError:    true,
					},
					toolName: tc.Name,
				}
			}
			// Request permission
			return PermissionRequestMsg{Request: req}
		}

		// Execute regular tool directly
		result := tool.Execute(ctx, tc.Name, params, cwd)

		// Special handling for TodoWrite - need to send todo update
		if tc.Name == "TodoWrite" && result.Success && len(result.TodoItems) > 0 {
			return todoResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    result.FormatForLLM(),
					IsError:    !result.Success,
				},
				toolName: tc.Name,
				todos:    result.TodoItems,
			}
		}

		return toolResultMsg{
			result: provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    result.FormatForLLM(),
				IsError:    !result.Success,
			},
			toolName: tc.Name,
		}
	}
}

// allToolsCompletedMsg signals that all pending tools have been executed
type allToolsCompletedMsg struct{}

// executeApprovedTool executes the current pending tool after user approval
func executeApprovedTool(toolCalls []provider.ToolCall, idx int, cwd string) tea.Cmd {
	if idx >= len(toolCalls) {
		return nil
	}

	tc := toolCalls[idx]

	return func() tea.Msg {
		ctx := context.Background()

		// Parse the tool input JSON
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Error parsing tool input: " + err.Error(),
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

		t, _ := tool.Get(tc.Name)
		pat := t.(tool.PermissionAwareTool)

		// Execute approved tool
		result := pat.ExecuteApproved(ctx, params, cwd)

		return toolResultMsg{
			result: provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    result.FormatForLLM(),
				IsError:    !result.Success,
			},
			toolName: tc.Name,
		}
	}
}

// executeInteractiveTool executes an interactive tool with the user's response
func executeInteractiveTool(tc provider.ToolCall, response *tool.QuestionResponse, cwd string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Parse the tool input JSON
		var params map[string]any
		if err := json.Unmarshal([]byte(tc.Input), &params); err != nil {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Error parsing tool input: " + err.Error(),
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Unknown tool: " + tc.Name,
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

		it, ok := t.(tool.InteractiveTool)
		if !ok {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Tool is not interactive: " + tc.Name,
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

		// Execute with the user's response
		result := it.ExecuteWithResponse(ctx, params, response, cwd)

		return toolResultMsg{
			result: provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    result.FormatForLLM(),
				IsError:    !result.Success,
			},
			toolName: tc.Name,
		}
	}
}

// toolResultMsg is sent when a single tool completes execution
type toolResultMsg struct {
	result   provider.ToolResult
	toolName string
}

// todoResultMsg is sent when TodoWrite tool completes (includes todo items for panel)
type todoResultMsg struct {
	result   provider.ToolResult
	toolName string
	todos    []toolui.TodoItem
}

// continueWithToolResults continues the conversation after tool execution
func (m model) continueWithToolResults() tea.Cmd {
	return func() tea.Msg {
		return streamContinueMsg{
			messages: m.convertMessagesToProvider(),
			modelID:  m.getModelID(),
		}
	}
}

// streamContinueMsg is sent when we need to continue streaming after tool execution
type streamContinueMsg struct {
	messages []provider.Message
	modelID  string
}

func (m model) waitForChunk() tea.Cmd {
	return func() tea.Msg {
		if m.streamChan == nil {
			return streamDoneMsg{}
		}
		chunk, ok := <-m.streamChan
		if !ok {
			return streamChunkMsg{done: true}
		}

		switch chunk.Type {
		case provider.ChunkTypeText:
			return streamChunkMsg{text: chunk.Text}
		case provider.ChunkTypeDone:
			// Check if there are tool calls in the response
			if chunk.Response != nil && len(chunk.Response.ToolCalls) > 0 {
				return streamChunkMsg{
					done:       true,
					toolCalls:  chunk.Response.ToolCalls,
					stopReason: chunk.Response.StopReason,
				}
			}
			return streamChunkMsg{done: true}
		case provider.ChunkTypeError:
			return streamChunkMsg{err: chunk.Error}
		case provider.ChunkTypeToolStart:
			// Tool call started - we'll handle this when done
			return streamChunkMsg{text: ""}
		case provider.ChunkTypeToolInput:
			// Tool input chunk - we'll handle this when done
			return streamChunkMsg{text: ""}
		default:
			return streamChunkMsg{text: ""}
		}
	}
}

func (m model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	// If selector is active, show it as an overlay
	if m.selector.IsActive() {
		return m.selector.Render()
	}

	chat := m.viewport.View()

	// Separator line
	separator := separatorStyle.Render(strings.Repeat("─", m.width))

	// Todo panel (persistent display above input)
	todoView := ""
	if m.todoPanel.IsVisible() {
		todoView = m.todoPanel.Render() + "\n"
	}

	// Question prompt (if active, replaces input area)
	if m.questionPrompt.IsActive() {
		return fmt.Sprintf("%s\n%s\n%s%s", chat, separator, todoView, m.questionPrompt.Render())
	}

	// Render input area with prompt on first line only
	prompt := inputPromptStyle.Render("❯ ")
	inputView := prompt + m.textarea.View()

	// Render suggestions if visible
	suggestions := m.suggestions.Render(m.width)
	if suggestions != "" {
		return fmt.Sprintf("%s\n%s\n%s%s\n%s\n%s", chat, separator, todoView, inputView, suggestions, separator)
	}

	return fmt.Sprintf("%s\n%s\n%s%s\n%s", chat, separator, todoView, inputView, separator)
}

func (m model) renderWelcome() string {
	// Gradient colors for the logo
	gradient := []lipgloss.Color{
		lipgloss.Color("#60A5FA"), // blue
		lipgloss.Color("#818CF8"), // indigo
		lipgloss.Color("#A78BFA"), // violet
	}

	logoLines := []string{
		"   ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄",
		"   █                             █",
		"   █   ╋╋╋╋╋   ╋╋╋╋   ╋   ╋      █",
		"   █   ╋       ╋      ╋╋  ╋      █",
		"   █   ╋  ╋╋╋  ╋╋╋╋   ╋ ╋ ╋      █",
		"   █   ╋    ╋  ╋      ╋  ╋╋      █",
		"   █   ╋╋╋╋╋   ╋╋╋╋   ╋   ╋      █",
		"   █                             █",
		"   ▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀",
	}

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280"))
	hintStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#4B5563"))

	var sb strings.Builder
	sb.WriteString("\n")

	// Render logo with gradient
	for i, line := range logoLines {
		colorIdx := i % len(gradient)
		style := lipgloss.NewStyle().Foreground(gradient[colorIdx])
		sb.WriteString(style.Render(line) + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString("   " + subtitleStyle.Render("AI-powered coding assistant") + "\n")
	sb.WriteString("\n")
	sb.WriteString("   " + hintStyle.Render("Enter to send · Esc to stop · Ctrl+C to exit") + "\n")

	return sb.String()
}

// Styles for tool display
var (
	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B"))

	toolResultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))

	toolResultExpandedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9CA3AF")).
				PaddingLeft(4)
)

func (m model) renderMessages() string {
	if len(m.messages) == 0 {
		return m.renderWelcome()
	}

	var sb strings.Builder

	for i, msg := range m.messages {
		// Add newline before messages, but not for tool results (they follow tool calls closely)
		if msg.toolResult == nil {
			sb.WriteString("\n")
		}

		switch msg.role {
		case "user":
			// Check if this is a tool result
			if msg.toolResult != nil {
				// Get tool name display
				toolName := msg.toolName
				if toolName == "" {
					toolName = "Tool"
				}

				// Format result size based on tool type
				sizeInfo := formatToolResultSize(toolName, msg.toolResult.Content)

				// Format: ⎿ Read → 76 lines
				icon := "⎿"
				if msg.toolResult.IsError {
					icon = "✗"
				}
				summary := toolResultStyle.Render(fmt.Sprintf("  %s  %s → %s", icon, toolName, sizeInfo))
				sb.WriteString(summary + "\n")

				// Show expanded content if expanded (no truncation)
				if msg.expanded {
					lines := strings.Split(msg.toolResult.Content, "\n")
					for _, line := range lines {
						sb.WriteString(toolResultExpandedStyle.Render(line) + "\n")
					}
				}
			} else {
				prompt := inputPromptStyle.Render("❯ ")
				content := userMsgStyle.Render(msg.content)
				sb.WriteString(prompt + content + "\n")
			}
		case "system":
			// System messages (command output)
			content := systemMsgStyle.Render(msg.content)
			sb.WriteString(content + "\n")
		case "permission":
			// Pending permission request - render inline using new Claude Code style
			if msg.pendingPermission != nil {
				m.permissionPrompt.Show(msg.pendingPermission, m.width, m.height)
				sb.WriteString(m.permissionPrompt.RenderInline())
			}
		default: // assistant
			aiIcon := aiPromptStyle.Render("◆ ")
			aiIndent := "  " // Indent for continuation lines (same width as icon)
			var content string
			if msg.content == "" && len(msg.toolCalls) == 0 && m.streaming {
				// Show spinner animation while thinking
				content = thinkingStyle.Render(m.spinner.View() + " Thinking...")
				sb.WriteString(aiIcon + content + "\n")
			} else if m.streaming && i == len(m.messages)-1 && len(msg.toolCalls) == 0 {
				// Show cursor at end while streaming (no markdown render)
				content = assistantMsgStyle.Render(msg.content + "▌")
				// Indent continuation lines
				content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
				sb.WriteString(aiIcon + content + "\n")
			} else if m.mdRenderer != nil && msg.content != "" {
				// Render markdown for completed messages
				rendered, err := m.mdRenderer.Render(msg.content)
				if err == nil {
					// Remove all leading newlines and whitespace from rendered markdown
					content = strings.TrimLeft(rendered, " \t\n")
					// Remove trailing whitespace
					content = strings.TrimRight(content, " \t\n")
					// Collapse multiple blank lines into single newline
					blankLines := regexp.MustCompile(`\n\s*\n`)
					content = blankLines.ReplaceAllString(content, "\n")
				} else {
					content = msg.content
				}
				// Indent continuation lines
				content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
				sb.WriteString(aiIcon + content + "\n")
			} else if msg.content != "" {
				content = msg.content
				// Indent continuation lines
				content = strings.ReplaceAll(content, "\n", "\n"+aiIndent)
				sb.WriteString(aiIcon + content + "\n")
			}

			// Render tool calls if any
			if len(msg.toolCalls) > 0 {
				for _, tc := range msg.toolCalls {
					// Extract key info from input for display
					args := extractToolArgs(tc.Input)
					toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s(%s)", tc.Name, args))
					sb.WriteString(toolLine + "\n")
				}
			}
		}
	}

	return sb.String()
}

// isGitRepo checks if the given directory is inside a git repository
func isGitRepo(dir string) bool {
	_, err := os.Stat(dir + "/.git")
	return err == nil
}

// extractToolArgs extracts a short summary of tool arguments for display
func extractToolArgs(input string) string {
	var params map[string]any
	if err := json.Unmarshal([]byte(input), &params); err != nil {
		return ""
	}

	// Priority: file_path > pattern > path
	if fp, ok := params["file_path"].(string); ok {
		return fp
	}
	if p, ok := params["pattern"].(string); ok {
		return p
	}
	if p, ok := params["path"].(string); ok {
		return p
	}

	// Fallback: first string value
	for _, v := range params {
		if s, ok := v.(string); ok {
			if len(s) > 30 {
				return s[:30] + "..."
			}
			return s
		}
	}
	return ""
}

// convertMessagesToProvider converts chatMessages to provider.Messages
// Skips system messages (command output) and includes all user/assistant messages
func (m model) convertMessagesToProvider() []provider.Message {
	providerMsgs := make([]provider.Message, 0, len(m.messages))
	for _, msg := range m.messages {
		if msg.role == "system" {
			continue // Skip system messages (command output)
		}

		providerMsg := provider.Message{
			Role:      msg.role,
			Content:   msg.content,
			ToolCalls: msg.toolCalls,
		}

		if msg.toolResult != nil {
			// Copy tool result and set tool name
			tr := *msg.toolResult
			if msg.toolName != "" {
				tr.ToolName = msg.toolName
			}
			providerMsg.ToolResult = &tr
		}

		providerMsgs = append(providerMsgs, providerMsg)
	}
	return providerMsgs
}

// getModelID returns the current model ID or default
func (m model) getModelID() string {
	if m.currentModel != nil {
		return m.currentModel.ModelID
	}
	return "claude-sonnet-4-20250514" // Default model
}

// formatToolResultSize formats the size information for a tool result
func formatToolResultSize(toolName, content string) string {
	switch toolName {
	case "WebFetch":
		size := len(content)
		if size >= 1024*1024 {
			return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
		}
		if size >= 1024 {
			return fmt.Sprintf("%.1f KB", float64(size)/1024)
		}
		return fmt.Sprintf("%d bytes", size)

	case "Write", "Edit":
		// Extract line count from status message like "Created /path (45 lines)"
		start := strings.Index(content, "(")
		if start == -1 {
			return "completed"
		}
		end := strings.Index(content[start:], ")")
		if end == -1 {
			return "completed"
		}
		return content[start+1 : start+end]

	default:
		// Show line count for other tools
		trimmed := strings.TrimSuffix(content, "\n")
		if trimmed == "" {
			return "0 lines"
		}
		lineCount := strings.Count(trimmed, "\n") + 1
		return fmt.Sprintf("%d lines", lineCount)
	}
}
