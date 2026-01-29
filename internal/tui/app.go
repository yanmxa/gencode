package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
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
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
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

// RunWithPlanMode starts the TUI application in plan mode
func RunWithPlanMode(task string) error {
	m := newModel()
	m.planMode = true
	m.planTask = task
	m.operationMode = config.ModePlan

	// Initialize plan store
	store, err := plan.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.planStore = store

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

// Styles
// Styles are initialized dynamically based on theme
var (
	userMsgStyle      lipgloss.Style
	assistantMsgStyle lipgloss.Style
	inputPromptStyle  lipgloss.Style
	aiPromptStyle     lipgloss.Style
	separatorStyle    lipgloss.Style
	thinkingStyle     lipgloss.Style
	systemMsgStyle    lipgloss.Style
)

func init() {
	// Initialize styles based on current theme
	userMsgStyle = lipgloss.NewStyle()
	assistantMsgStyle = lipgloss.NewStyle()

	inputPromptStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Primary).
		Bold(true)

	aiPromptStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.AI).
		Bold(true)

	separatorStyle = lipgloss.NewStyle().
		Faint(true).
		Foreground(CurrentTheme.Separator)

	thinkingStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Accent)

	systemMsgStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDim).
		PaddingLeft(2)
}

type chatMessage struct {
	role              string
	content           string
	toolCalls         []provider.ToolCall  // For assistant messages with tool calls
	toolCallsExpanded bool                 // Whether tool calls are expanded (show full args)
	toolResult        *provider.ToolResult // For tool result messages
	toolName          string               // Tool name for tool result display
	expanded          bool                 // Whether tool result is expanded
	todos             []toolui.TodoItem    // Snapshot of todos (for TodoWrite results)
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
	selectedToolIdx int       // Index of selected tool result for expansion (-1 = none)
	lastCtrlOTime   time.Time // Last Ctrl+O press time for double-tap detection

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

	// Plan mode
	planMode   bool        // Whether in plan mode
	planTask   string      // The task description for plan mode
	planPrompt *PlanPrompt // Plan approval UI
	planStore  *plan.Store // Plan file storage

	// Operation mode for mode cycling (Normal -> AutoAccept -> Plan -> Normal)
	operationMode config.OperationMode
}

// createMarkdownRenderer creates a glamour renderer with the specified width
func createMarkdownRenderer(width int) *glamour.TermRenderer {
	wrapWidth := width - 4
	if wrapWidth < 40 {
		wrapWidth = 40
	}

	// Auto-detect terminal background and select appropriate style
	var compactStyle ansi.StyleConfig
	if lipgloss.HasDarkBackground() {
		compactStyle = styles.DarkStyleConfig
	} else {
		compactStyle = styles.LightStyleConfig
	}

	// Apply compact margins
	uintPtr := func(u uint) *uint { return &u }
	compactStyle.Document.Margin = uintPtr(0)
	compactStyle.Paragraph.Margin = uintPtr(0)
	compactStyle.CodeBlock.Margin = uintPtr(0)

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
	ta.Prompt = ""   // No prompt per line, we render it manually
	ta.CharLimit = 0 // No character limit (0 = unlimited)
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(CurrentTheme.Muted)
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
		planPrompt:         NewPlanPrompt(),
		operationMode:      config.ModeNormal,
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
		// Show permission prompt as a fixed footer (like question prompt)
		// Don't add a message - the prompt is rendered in View()
		m.permissionPrompt.Show(msg.Request, m.width, m.height)
		return m, nil

	// Handle permission response (user approved/denied)
	case PermissionResponseMsg:
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
			return m, executeApprovedTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd)
		} else {
			// Tool was denied - add error result and stop processing
			tc := m.pendingToolCalls[m.pendingToolIdx]
			m.messages = append(m.messages, chatMessage{
				role:     "user",
				toolName: tc.Name,
				toolResult: &provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "User denied permission",
					IsError:    true,
				},
			})
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
			todos:      msg.todos, // Store snapshot for inline rendering
		})
		// Update the todo panel (for tracking current state)
		m.todoPanel.Update(msg.todos)
		m.todoPanel.SetWidth(m.width)
		m.pendingToolIdx++
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, processNextTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd, m.settings, m.sessionPermissions)

	// Handle single tool result
	case toolResultMsg:
		r := msg.result
		m.messages = append(m.messages, chatMessage{
			role:       "user",
			toolResult: &r,
			toolName:   msg.toolName,
		})
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

	// Handle plan request from ExitPlanMode tool
	case PlanRequestMsg:
		m.planPrompt.Show(msg.Request, m.width, m.height)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	// Handle plan response (user approved/rejected/modified)
	case PlanResponseMsg:
		if !msg.Approved {
			// Plan was rejected - add error result and stop
			tc := m.pendingToolCalls[m.pendingToolIdx]
			m.messages = append(m.messages, chatMessage{
				role:     "user",
				toolName: tc.Name,
				toolResult: &provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Plan was rejected by the user. Please ask for clarification or modify your approach.",
					IsError:    true,
				},
			})
			m.pendingToolCalls = nil
			m.pendingToolIdx = 0
			m.streaming = false
			m.planMode = false
			m.operationMode = config.ModeNormal
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}

		// Plan was approved - handle based on mode
		tc := m.pendingToolCalls[m.pendingToolIdx]

		// Save plan to file
		if m.planStore == nil {
			m.planStore, _ = plan.NewStore()
		}
		if m.planStore != nil {
			planContent := msg.ModifiedPlan
			if planContent == "" && msg.Request != nil {
				planContent = msg.Request.Plan
			}
			savedPlan := &plan.Plan{
				Task:    m.planTask,
				Status:  plan.StatusApproved,
				Content: planContent,
			}
			m.planStore.Save(savedPlan)
		}

		// Handle different approval modes
		switch msg.ApproveMode {
		case "clear-auto":
			// Clear context and auto-accept edits
			m.messages = []chatMessage{}
			m.sessionPermissions.AllowAllEdits = true
			m.sessionPermissions.AllowAllWrites = true
			m.operationMode = config.ModeAutoAccept
			m.planMode = false

			// Clear pending tools to avoid sending orphan tool_result
			m.pendingToolCalls = nil
			m.pendingToolIdx = 0

			// Start fresh conversation with plan as context
			planContent := msg.ModifiedPlan
			if planContent == "" && msg.Request != nil {
				planContent = msg.Request.Plan
			}
			userMsg := fmt.Sprintf("Please implement the following plan:\n\n%s", planContent)
			m.messages = append(m.messages, chatMessage{role: "user", content: userMsg})

			// Start streaming
			m.streaming = true
			ctx, cancel := context.WithCancel(context.Background())
			m.cancelFunc = cancel
			providerMsgs := m.convertMessagesToProvider()
			m.messages = append(m.messages, chatMessage{role: "assistant", content: ""})
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()

			modelID := m.getModelID()
			sysPrompt := system.Prompt(system.Config{
				Provider: m.llmProvider.Name(),
				Model:    modelID,
				Cwd:      m.cwd,
				IsGit:    isGitRepo(m.cwd),
				PlanMode: false,
			})
			tools := m.getToolsForMode()

			m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
				Model:        modelID,
				Messages:     providerMsgs,
				MaxTokens:    8192,
				Tools:        tools,
				SystemPrompt: sysPrompt,
			})
			return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)

		case "auto":
			// Keep context, auto-accept edits
			m.sessionPermissions.AllowAllEdits = true
			m.sessionPermissions.AllowAllWrites = true
			m.operationMode = config.ModeAutoAccept
		case "manual":
			// Keep context, manual approval (default behavior)
			m.operationMode = config.ModeNormal
		case "modify":
			// Plan was modified - use modified content
			m.operationMode = config.ModeNormal
		}

		// Exit plan mode
		m.planMode = false

		// Execute the tool with the response
		return m, executePlanTool(tc, msg.Response, m.cwd)

	case tea.KeyMsg:
		// Handle plan prompt first (highest priority)
		if m.planPrompt != nil && m.planPrompt.IsActive() {
			cmd := m.planPrompt.HandleKeypress(msg)
			return m, cmd
		}

		// Handle question prompt
		if m.questionPrompt.IsActive() {
			cmd := m.questionPrompt.HandleKeypress(msg)
			return m, cmd
		}

		// Handle permission prompt
		if m.permissionPrompt.IsActive() {
			cmd := m.permissionPrompt.HandleKeypress(msg)
			// No viewport update needed - View() renders the prompt as a footer
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

		// Global Shift+Tab for mode cycling
		if msg.Type == tea.KeyShiftTab {
			if !m.streaming && !m.permissionPrompt.IsActive() &&
				!m.questionPrompt.IsActive() &&
				(m.planPrompt == nil || !m.planPrompt.IsActive()) &&
				!m.selector.IsActive() && !m.suggestions.IsVisible() {
				m.cycleOperationMode()
				m.viewport.SetContent(m.renderMessages())
				return m, nil
			}
		}

		// Handle Ctrl+O to toggle expansion
		// Double-tap Ctrl+O (within 500ms) to toggle ALL tool results
		// Single Ctrl+O toggles only the last tool result
		if msg.Type == tea.KeyCtrlO {
			// If permission prompt is active, toggle preview expansion (diff or bash)
			if m.permissionPrompt != nil && m.permissionPrompt.IsActive() {
				if m.permissionPrompt.diffPreview != nil {
					m.permissionPrompt.diffPreview.ToggleExpand()
				}
				if m.permissionPrompt.bashPreview != nil {
					m.permissionPrompt.bashPreview.ToggleExpand()
				}
				// No viewport update needed - View() renders the prompt as a footer
				return m, nil
			}

			now := time.Now()
			// Check for double-tap (within 500ms)
			if now.Sub(m.lastCtrlOTime) < 500*time.Millisecond {
				// Double-tap: toggle ALL tool results and tool calls
				anyExpanded := false
				for _, msg := range m.messages {
					if (msg.toolResult != nil && msg.expanded) || (len(msg.toolCalls) > 0 && msg.toolCallsExpanded) {
						anyExpanded = true
						break
					}
				}
				for i := range m.messages {
					if m.messages[i].toolResult != nil {
						m.messages[i].expanded = !anyExpanded
					}
					if len(m.messages[i].toolCalls) > 0 {
						m.messages[i].toolCallsExpanded = !anyExpanded
					}
				}
				m.lastCtrlOTime = time.Time{} // Reset to prevent triple-tap
				m.viewport.SetContent(m.renderMessages())
				return m, nil
			}

			// Single tap: toggle last tool result or tool calls
			m.lastCtrlOTime = now
			for i := len(m.messages) - 1; i >= 0; i-- {
				// First check for tool results
				if m.messages[i].toolResult != nil {
					m.messages[i].expanded = !m.messages[i].expanded
					m.viewport.SetContent(m.renderMessages())
					return m, nil
				}
				// Then check for tool calls (assistant messages with pending tools)
				if len(m.messages[i].toolCalls) > 0 {
					m.messages[i].toolCallsExpanded = !m.messages[i].toolCallsExpanded
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

			// Clear todo panel when starting new conversation
			m.todoPanel.Clear()

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

			// Build system prompt and select tools based on mode
			sysPrompt := system.Prompt(system.Config{
				Provider: m.llmProvider.Name(),
				Model:    modelID,
				Cwd:      m.cwd,
				IsGit:    isGitRepo(m.cwd),
				PlanMode: m.planMode,
			})

			tools := m.getToolsForMode()

			m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
				Model:        modelID,
				Messages:     providerMsgs,
				MaxTokens:    8192,
				Tools:        tools,
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

		// Build system prompt and select tools based on mode
		sysPrompt := system.Prompt(system.Config{
			Provider: m.llmProvider.Name(),
			Model:    msg.modelID,
			Cwd:      m.cwd,
			IsGit:    isGitRepo(m.cwd),
			PlanMode: m.planMode,
		})

		tools := m.getToolsForMode()

		// Start streaming with tools
		m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
			Model:        msg.modelID,
			Messages:     msg.messages,
			MaxTokens:    8192,
			Tools:        tools,
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
			// Only re-render if spinner is actually visible:
			// - Not executing tools (pendingToolCalls is nil)
			// - Last message is assistant with no content and no tool calls
			// This prevents flickering of static tool call displays like ⚡Bash(...)
			if m.pendingToolCalls == nil && len(m.messages) > 0 {
				lastMsg := m.messages[len(m.messages)-1]
				if lastMsg.role == "assistant" && lastMsg.content == "" && len(lastMsg.toolCalls) == 0 {
					m.viewport.SetContent(m.renderMessages())
				}
			}
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

		// Check if it's an interactive tool (like AskUserQuestion or ExitPlanMode)
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
			// Return plan request for TUI to handle (ExitPlanMode)
			if pr, ok := req.(*tool.PlanRequest); ok {
				return PlanRequestMsg{Request: pr}
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

		t, ok := tool.Get(tc.Name)
		if !ok {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Internal error: unknown tool: " + tc.Name,
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

		pat, ok := t.(tool.PermissionAwareTool)
		if !ok {
			return toolResultMsg{
				result: provider.ToolResult{
					ToolCallID: tc.ID,
					Content:    "Internal error: tool does not implement PermissionAwareTool: " + tc.Name,
					IsError:    true,
				},
				toolName: tc.Name,
			}
		}

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

// executePlanTool executes the ExitPlanMode tool with the user's response
func executePlanTool(tc provider.ToolCall, response *tool.PlanResponse, cwd string) tea.Cmd {
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

// renderModeStatus renders the current mode indicator
func (m model) renderModeStatus() string {
	var icon, label string
	var color lipgloss.Color

	if m.operationMode == config.ModeAutoAccept {
		icon = "⏵⏵"
		label = " accept edits on"
		color = CurrentTheme.Success
	} else if m.operationMode == config.ModePlan {
		icon = "⏸"
		label = " plan mode on"
		color = CurrentTheme.Warning
	} else {
		return ""
	}

	styledIcon := lipgloss.NewStyle().Foreground(color).Render(icon)
	styledLabel := lipgloss.NewStyle().Foreground(color).Render(label)
	hint := lipgloss.NewStyle().Foreground(CurrentTheme.Muted).Render("  shift+tab to toggle")

	return "  " + styledIcon + styledLabel + hint
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

	// Plan prompt (if active, show content in chat area and menu below separator)
	if m.planPrompt != nil && m.planPrompt.IsActive() {
		planContent := m.planPrompt.RenderContent()
		planMenu := m.planPrompt.RenderMenu()
		return fmt.Sprintf("%s\n%s\n%s\n%s\n%s", chat, planContent, separator, planMenu, separator)
	}

	// Permission prompt (if active, replaces input area)
	if m.permissionPrompt.IsActive() {
		return fmt.Sprintf("%s\n%s\n%s", chat, separator, m.permissionPrompt.Render())
	}

	// Question prompt (if active, replaces input area)
	if m.questionPrompt.IsActive() {
		return fmt.Sprintf("%s\n%s\n%s", chat, separator, m.questionPrompt.Render())
	}

	// Render input area with prompt on first line only
	prompt := inputPromptStyle.Render("❯ ")
	inputView := prompt + m.textarea.View()

	// Mode status line (displayed below bottom separator)
	statusLine := m.renderModeStatus()

	// Render suggestions if visible
	suggestions := m.suggestions.Render(m.width)
	if suggestions != "" {
		if statusLine != "" {
			return fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
				chat, separator, inputView, suggestions, separator, statusLine)
		}
		return fmt.Sprintf("%s\n%s\n%s\n%s\n%s", chat, separator, inputView, suggestions, separator)
	}

	if statusLine != "" {
		return fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
			chat, separator, inputView, separator, statusLine)
	}

	return fmt.Sprintf("%s\n%s\n%s\n%s", chat, separator, inputView, separator)
}

func (m model) renderWelcome() string {
	// Gradient colors for the logo (use theme colors)
	gradient := []lipgloss.Color{
		CurrentTheme.Primary,
		CurrentTheme.AI,
		CurrentTheme.Accent,
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
		Foreground(CurrentTheme.Muted)
	hintStyle := lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDisabled)

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
	sb.WriteString("   " + hintStyle.Render("Enter to send · Esc to stop · Shift+Tab mode · Ctrl+C exit") + "\n")

	return sb.String()
}

// Tool display styles (initialized in initToolStyles)
var (
	toolCallStyle           lipgloss.Style
	toolResultStyle         lipgloss.Style
	toolResultExpandedStyle lipgloss.Style
	todoPendingStyle        lipgloss.Style
	todoInProgressStyle     lipgloss.Style
	todoCompletedStyle      lipgloss.Style
)

func init() {
	// Initialize tool styles based on current theme
	toolCallStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Accent)

	toolResultStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	toolResultExpandedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDim).
		PaddingLeft(4)

	// Todo styles for inline rendering
	todoPendingStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Muted)

	todoInProgressStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.Warning).
		Bold(true)

	todoCompletedStyle = lipgloss.NewStyle().
		Foreground(CurrentTheme.TextDisabled).
		Strikethrough(true)
}

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

				// Render TodoWrite results inline with todos snapshot
				if toolName == "TodoWrite" && len(msg.todos) > 0 {
					sb.WriteString(renderTodosInline(msg.todos))
					continue
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

				// Show expanded content if expanded or if it's an error (errors always shown)
				if msg.expanded || msg.toolResult.IsError {
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
			// Permission prompt is rendered separately in View() as a fixed footer
			// Skip rendering here to avoid calling Show() which resets selectedIdx
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
				// Add blank line after content before tool calls (only if there was content)
				if msg.content != "" {
					sb.WriteString("\n")
				}

				// First, look ahead for TodoWrite results and render todos before other tools
				for j := i + 1; j < len(m.messages); j++ {
					nextMsg := m.messages[j]
					// Stop if we hit a non-tool-result message
					if nextMsg.toolResult == nil {
						break
					}
					// Found TodoWrite result with todos - render it first
					if nextMsg.toolName == "TodoWrite" && len(nextMsg.todos) > 0 {
						sb.WriteString(renderTodosInline(nextMsg.todos))
						break
					}
				}

				// Then render other tool calls (skip TodoWrite)
				for _, tc := range msg.toolCalls {
					if tc.Name == "TodoWrite" {
						continue
					}
					if msg.toolCallsExpanded {
						// Expanded: show full tool input JSON formatted
						toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s", tc.Name))
						sb.WriteString(toolLine + "\n")
						// Pretty print the JSON input
						var params map[string]any
						if err := json.Unmarshal([]byte(tc.Input), &params); err == nil {
							for k, v := range params {
								if s, ok := v.(string); ok {
									// Wrap long strings
									if len(s) > 80 {
										sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("%s:", k)) + "\n")
										sb.WriteString(toolResultExpandedStyle.Render(s) + "\n")
									} else {
										sb.WriteString(toolResultExpandedStyle.Render(fmt.Sprintf("%s: %s", k, s)) + "\n")
									}
								}
							}
						}
					} else {
						// Collapsed: show short summary
						args := extractToolArgs(tc.Input)
						toolLine := toolCallStyle.Render(fmt.Sprintf("⚡%s(%s)", tc.Name, args))
						sb.WriteString(toolLine + "\n")
					}
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

	// Fallback: sort keys for deterministic iteration
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if s, ok := params[k].(string); ok {
			if len(s) > 60 {
				return s[:60] + "..."
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

// getToolsForMode returns the appropriate tool set based on plan mode
func (m model) getToolsForMode() []provider.Tool {
	if m.planMode {
		return tool.GetPlanModeToolSchemas()
	}
	return tool.GetToolSchemas()
}

// cycleOperationMode cycles through Normal -> AutoAccept -> Plan -> Normal
func (m *model) cycleOperationMode() {
	m.operationMode = m.operationMode.Next()

	// Reset all permissions
	m.sessionPermissions.AllowAllEdits = false
	m.sessionPermissions.AllowAllWrites = false
	m.sessionPermissions.AllowAllBash = false

	// Configure based on mode
	if m.operationMode == config.ModeAutoAccept {
		m.sessionPermissions.AllowAllEdits = true
		m.sessionPermissions.AllowAllWrites = true
	}

	m.planMode = (m.operationMode == config.ModePlan)
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

// renderTodosInline renders a snapshot of todos inline in the message flow
// Format:
//
//	📋 Tasks [1/4]
//	  completed task (strikethrough)
//	  in progress task (orange highlight)
//	  pending task (gray)
//
// Order: completed → in_progress → pending
func renderTodosInline(todos []toolui.TodoItem) string {
	if len(todos) == 0 {
		return ""
	}

	var sb strings.Builder

	// Count tasks for progress display
	pending, inProgress, completed := 0, 0, 0
	for _, todo := range todos {
		switch todo.Status {
		case "pending":
			pending++
		case "in_progress":
			inProgress++
		case "completed":
			completed++
		}
	}
	total := pending + inProgress + completed

	// Header line with clipboard icon: 📋 Tasks [2/5]
	header := toolResultStyle.Render(fmt.Sprintf("  📋 Tasks [%d/%d]", completed, total))
	sb.WriteString(header + "\n")

	// 2-space indent to align with header
	indent := "  "

	// Render in order: completed → in_progress → pending
	for _, todo := range todos {
		if todo.Status == "completed" {
			sb.WriteString(indent + todoCompletedStyle.Render(todo.Content) + "\n")
		}
	}
	for _, todo := range todos {
		if todo.Status == "in_progress" {
			sb.WriteString(indent + todoInProgressStyle.Render(todo.ActiveForm) + "\n")
		}
	}
	for _, todo := range todos {
		if todo.Status == "pending" {
			sb.WriteString(indent + todoPendingStyle.Render(todo.Content) + "\n")
		}
	}

	return sb.String()
}
