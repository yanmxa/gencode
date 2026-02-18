package tui

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
)

type operationMode int

const (
	modeNormal operationMode = iota
	modeAutoAccept
	modePlan
)

func (m operationMode) Next() operationMode {
	return (m + 1) % 3
}

// Message roles
const (
	roleUser      = "user"
	roleAssistant = "assistant"
	roleNotice    = "notice" // UI-only notifications (not sent to LLM)
)

type chatMessage struct {
	role              string
	content           string
	thinking          string               // Reasoning content for thinking models
	images            []message.ImageData // Image attachments
	toolCalls         []message.ToolCall
	toolCallsExpanded bool
	toolResult        *message.ToolResult
	toolName          string
	expanded          bool
	renderedInline    bool // marks this toolResult was rendered inline with its tool call
	isSummary         bool // marks this as a compact summary message
	summaryCount      int  // original message count before compaction
}

type (
	streamChunkMsg struct {
		text             string
		thinking         string // Reasoning content for thinking models
		done             bool
		err              error
		toolCalls        []message.ToolCall
		stopReason       string
		buildingToolName string
		usage            *message.Usage
	}
	streamDoneMsg     struct{}
	streamContinueMsg struct {
		messages []message.Message
		modelID  string
	}
	// EditorFinishedMsg is sent when an external editor process completes
	EditorFinishedMsg struct {
		Err error
	}
)

type model struct {
	textarea     textarea.Model
	spinner      spinner.Model
	messages     []chatMessage
	llmProvider  provider.LLMProvider
	store        *provider.Store
	currentModel *provider.CurrentModelInfo
	streaming    bool
	streamChan   <-chan message.StreamChunk
	cancelFunc   context.CancelFunc
	width        int
	height       int
	ready        bool
	inputHistory []string
	historyIndex int
	tempInput    string
	cwd          string

	mdRenderer *glamour.TermRenderer

	currentToolID    string
	currentToolName  string
	currentToolInput string

	selectedToolIdx int
	lastCtrlOTime   time.Time

	suggestions SuggestionState

	selector       SelectorState
	memorySelector MemorySelectorState

	permissionPrompt *PermissionPrompt
	pendingToolCalls []message.ToolCall
	pendingToolIdx   int

	// Parallel tool execution tracking
	parallelMode        bool                        // True when executing tools in parallel
	parallelResults     map[int]message.ToolResult // Collected results by index
	parallelResultCount int                         // Number of results received

	settings           *config.Settings
	sessionPermissions *config.SessionPermissions

	questionPrompt  *QuestionPrompt
	pendingQuestion *tool.QuestionRequest

	enterPlanPrompt *EnterPlanPrompt

	planMode   bool
	planTask   string
	planPrompt *PlanPrompt
	planStore  *plan.Store

	operationMode operationMode

	buildingToolName string

	disabledTools    map[string]bool
	toolSelector     ToolSelectorState
	mcpSelector      MCPSelectorState
	pluginSelector   PluginSelectorState

	// Skill system
	skillSelector            SkillSelectorState
	pendingSkillInstructions string // Full skill content for next message
	pendingSkillArgs         string // User args for skill invocation

	// Agent management
	agentSelector AgentSelectorState

	// Task progress tracking
	activeTaskID string   // Currently executing Task ID (for progress display)
	taskProgress []string // Recent progress messages from Task

	// Token usage tracking (from most recent API response)
	lastInputTokens  int // Input tokens from last API call (represents current context size)
	lastOutputTokens int // Output tokens from last API call

	// Token limit fetching state
	fetchingTokenLimits bool // True when auto-fetching token limits

	// Compact state
	compacting      bool   // True when compacting conversation
	compactFocus    string // Optional focus for compact (e.g., "current task only")
	autoCompactNext bool   // True when auto-compact should trigger after current operation

	// Session persistence
	sessionStore           *session.Store
	currentSessionID       string
	sessionSelector        SessionSelectorState
	pendingSessionSelector bool // Open session selector after window ready

	// Memory file editing
	editingMemoryFile string // Path to memory file being edited

	// Hooks engine
	hookEngine *hooks.Engine

	// MCP registry
	mcpRegistry *mcp.Registry

	// Core loop for LLM orchestration
	loop *core.Loop

	// Inline rendering: tracks how many messages have been pushed to scrollback
	committedCount     int
	pendingClearScreen bool

	// Cached memory content (avoids re-reading from disk every turn)
	cachedMemory string

	// Pending images from clipboard paste
	pendingImages         []message.ImageData
	imageSelectMode       bool // True when in image selection mode
	selectedImageIdx      int  // Currently selected image index
}

// RunOptions contains options for running the TUI
type RunOptions struct {
	PluginDir string // Directory to load plugins from
}

func Run() error {
	return RunWithOptions(RunOptions{})
}

func RunWithOptions(opts RunOptions) error {
	m := newModel()

	// Load plugins from specified directory
	if opts.PluginDir != "" {
		ctx := context.Background()
		if err := plugin.DefaultRegistry.LoadFromPath(ctx, opts.PluginDir); err != nil {
			return fmt.Errorf("failed to load plugins from %s: %w", opts.PluginDir, err)
		}
	}

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

func RunWithPlanMode(task string) error {
	m := newModel()
	m.planMode = true
	m.planTask = task
	m.operationMode = modePlan

	store, err := plan.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize plan store: %w", err)
	}
	m.planStore = store

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

// RunWithContinue runs TUI and resumes the most recent session
func RunWithContinue() error {
	m := newModel()

	// Initialize session store
	sessionStore, err := session.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	m.sessionStore = sessionStore

	// Load the latest session for the current working directory
	cwd, _ := os.Getwd()
	sess, err := sessionStore.GetLatestByCwd(cwd)
	if err != nil {
		return fmt.Errorf("no previous session to continue: %w", err)
	}

	// Restore messages from session
	m.messages = convertFromStoredMessages(sess.Messages)
	m.currentSessionID = sess.Metadata.ID

	// Restore tasks from session
	if len(sess.Tasks) > 0 {
		tool.DefaultTodoStore.Import(sess.Tasks)
	}

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

// RunWithResume runs TUI with the session selector to choose a session to resume
func RunWithResume() error {
	m := newModel()

	// Initialize session store
	sessionStore, err := session.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	m.sessionStore = sessionStore

	// Set flag to open session selector after window is ready
	m.pendingSessionSelector = true

	p := tea.NewProgram(m)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	return nil
}

func newModel() model {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.Focus()
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.SetWidth(defaultWidth)
	ta.SetHeight(minTextareaHeight)
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

	store, _ := provider.NewStore()
	var llmProvider provider.LLMProvider
	var currentModel *provider.CurrentModelInfo

	if store != nil {
		currentModel = store.GetCurrentModel()
		ctx := context.Background()

		if currentModel != nil {
			p, err := provider.GetProvider(ctx, currentModel.Provider, currentModel.AuthMethod)
			if err == nil {
				llmProvider = p
			}
		}

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

	cwd, _ := os.Getwd()

	// Initialize plugin registry (must be before skills and agents)
	ctx := context.Background()
	if err := plugin.DefaultRegistry.Load(ctx, cwd); err != nil {
		log.Logger().Warn("Failed to load plugins", zap.Error(err))
	}

	// Initialize skill registry
	if err := skill.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize skill registry", zap.Error(err))
	}

	// Initialize agent system (loads custom agents and state stores)
	agent.Init(cwd)

	// Initialize MCP registry
	var mcpRegistry *mcp.Registry
	if err := mcp.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize MCP registry", zap.Error(err))
	} else {
		mcpRegistry = mcp.DefaultRegistry
	}

	mdRenderer := createMarkdownRenderer(defaultWidth)

	settings, _ := config.Load()
	if settings == nil {
		settings = config.Default()
	}

	// Initialize hooks engine
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	hookEngine := hooks.NewEngine(settings, sessionID, cwd, "")

	// Configure Task tool if provider is available
	if llmProvider != nil {
		modelID := ""
		if currentModel != nil {
			modelID = currentModel.ModelID
		}
		configureTaskTool(llmProvider, cwd, modelID, hookEngine)
	}

	// Initialize suggestions with cwd for @ file completion
	suggestions := NewSuggestionState()
	suggestions.SetCwd(cwd)

	loop := &core.Loop{}

	return model{
		textarea:           ta,
		spinner:            sp,
		messages:           []chatMessage{},
		llmProvider:        llmProvider,
		store:              store,
		currentModel:       currentModel,
		inputHistory:       loadInputHistory(cwd),
		historyIndex:       -1,
		cwd:                cwd,
		mdRenderer:         mdRenderer,
		suggestions:        suggestions,
		selector:           NewSelectorState(),
		memorySelector:     NewMemorySelectorState(),
		permissionPrompt:   NewPermissionPrompt(),
		settings:           settings,
		sessionPermissions: config.NewSessionPermissions(),
		questionPrompt:     NewQuestionPrompt(),
		enterPlanPrompt:    NewEnterPlanPrompt(),
		planPrompt:         NewPlanPrompt(),
		operationMode:      modeNormal,
		disabledTools:      config.GetDisabledTools(),
		toolSelector:       NewToolSelectorState(),
		mcpSelector:        NewMCPSelectorState(),
		pluginSelector:     NewPluginSelectorState(),
		skillSelector:      NewSkillSelectorState(),
		agentSelector:      NewAgentSelectorState(),
		sessionSelector:    NewSessionSelectorState(),
		hookEngine:         hookEngine,
		mcpRegistry:        mcpRegistry,
		loop:               loop,
	}
}

func (m model) Init() tea.Cmd {
	// Execute SessionStart hook asynchronously
	if m.hookEngine != nil {
		source := "startup"
		if m.currentSessionID != "" {
			source = "resume"
		}
		m.hookEngine.ExecuteAsync(hooks.SessionStart, hooks.HookInput{
			Source: source,
			Model:  m.getModelID(),
		})
	}
	return tea.Batch(textarea.Blink, m.spinner.Tick, autoConnectMCPServers())
}

func (m *model) updateTextareaHeight() {
	content := m.textarea.Value()
	lines := strings.Count(content, "\n") + 1

	newHeight := lines
	if newHeight < minTextareaHeight {
		newHeight = minTextareaHeight
	}
	if newHeight > maxTextareaHeight {
		newHeight = maxTextareaHeight
	}

	m.textarea.SetHeight(newHeight)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case ProviderConnectResultMsg:
		return m.handleProviderConnectResult(msg)

	case ProviderSelectedMsg:
		return m.handleProviderSelected(msg)

	// Selector state changes - handled by selector components, no action needed
	case SelectorCancelledMsg,
		ToolToggleMsg, ToolSelectorCancelledMsg,
		SkillCycleMsg, SkillSelectorCancelledMsg,
		AgentToggleMsg, AgentSelectorCancelledMsg:
		return m, nil

	case MCPConnectMsg:
		// Clear disabled flag and mark as connecting
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetDisabled(msg.ServerName, false)
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return m, startMCPConnect(msg.ServerName)

	case MCPConnectResultMsg:
		// Clear connecting state and store error if failed
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, false)
			if !msg.Success && msg.Error != nil {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, msg.Error.Error())
			} else {
				mcp.DefaultRegistry.SetConnectError(msg.ServerName, "")
			}
		}
		m.mcpSelector.HandleConnectResult(msg)
		// Only show notice when selector is not active (status visible in selector UI)
		if !m.mcpSelector.IsActive() && !msg.Success {
			content := fmt.Sprintf("Failed to connect to '%s': %v", msg.ServerName, msg.Error)
			m.messages = append(m.messages, chatMessage{role: roleNotice, content: content})
			return m, tea.Batch(m.commitMessages()...)
		}
		return m, nil

	case MCPDisconnectMsg:
		m.mcpSelector.HandleDisconnect(msg.ServerName)
		return m, nil

	case MCPReconnectMsg:
		m.mcpSelector.HandleReconnect(msg.ServerName)
		if mcp.DefaultRegistry != nil {
			mcp.DefaultRegistry.SetConnecting(msg.ServerName, true)
		}
		return m, startMCPConnect(msg.ServerName)

	case MCPRemoveMsg:
		m.mcpSelector.HandleRemove(msg.ServerName)
		return m, nil

	case MCPAddRequestMsg:
		m.textarea.SetValue("/mcp add ")
		return m, nil

	// Plugin selector events
	case PluginEnableMsg:
		m.pluginSelector.HandleEnable(msg.PluginName)
		return m, nil

	case PluginDisableMsg:
		m.pluginSelector.HandleDisable(msg.PluginName)
		return m, nil

	case PluginUninstallMsg:
		m.pluginSelector.HandleUninstall(msg.PluginName)
		return m, nil

	case PluginInstallMsg:
		// Install plugin asynchronously
		return m, m.installPlugin(msg)

	case PluginInstallResultMsg:
		m.pluginSelector.HandleInstallResult(msg)
		// Reload agents to pick up newly installed plugin agents
		if msg.Success {
			agent.Init(m.cwd)
		}
		return m, nil

	case MarketplaceRemoveMsg:
		m.pluginSelector.HandleMarketplaceRemove(msg.ID)
		return m, nil

	case MarketplaceSyncResultMsg:
		m.pluginSelector.HandleMarketplaceSync(msg)
		return m, nil

	// Additional selector cancelled events
	case MCPSelectorCancelledMsg, SessionSelectorCancelledMsg, MemorySelectorCancelledMsg, PluginSelectorCancelledMsg:
		return m, nil

	case SessionSelectedMsg:
		return m.handleSessionSelected(msg)

	case MemorySelectedMsg:
		return m.handleMemorySelected(msg)

	case SkillInvokeMsg:
		// A skill was invoked from the selector - trigger skill execution
		if sk, ok := skill.DefaultRegistry.Get(msg.SkillName); ok {
			executeSkillCommand(&m, sk, "")
			return m.handleSkillInvocation()
		}
		return m, nil

	case ModelSelectedMsg:
		return m.handleModelSelected(msg)

	case PermissionRequestMsg:
		return m.handlePermissionRequest(msg)

	case PermissionResponseMsg:
		return m.handlePermissionResponse(msg)

	case toolResultMsg:
		return m.handleToolResult(msg)

	case TaskProgressMsg:
		return m.handleTaskProgress(msg)

	case startToolExecutionMsg:
		return m.handleStartToolExecution(msg)

	case allToolsCompletedMsg:
		return m.handleAllToolsCompleted()

	case QuestionRequestMsg:
		return m.handleQuestionRequest(msg)

	case QuestionResponseMsg:
		return m.handleQuestionResponse(msg)

	case PlanRequestMsg:
		return m.handlePlanRequest(msg)

	case PlanResponseMsg:
		return m.handlePlanResponse(msg)

	case EnterPlanRequestMsg:
		return m.handleEnterPlanRequest(msg)

	case EnterPlanResponseMsg:
		return m.handleEnterPlanResponse(msg)

	case TokenLimitResultMsg:
		return m.handleTokenLimitResult(msg)

	case CompactResultMsg:
		return m.handleCompactResult(msg)

	case EditorFinishedMsg:
		return m.handleEditorFinished(msg)

	case tea.KeyMsg:
		result, cmd := m.handleKeypress(msg)
		if cmd != nil || result != nil {
			if result != nil {
				return result, cmd
			}
			return m, cmd
		}

	case tea.WindowSizeMsg:
		return m.handleWindowResize(msg)

	case streamChunkMsg:
		return m.handleStreamChunk(msg)

	case streamContinueMsg:
		return m.handleStreamContinue(msg)

	case streamDoneMsg:
		m.streaming = false
		m.streamChan = nil
		m.cancelFunc = nil
		cmds = append(cmds, m.commitMessages()...)

	case spinner.TickMsg:
		return m.handleSpinnerTick(msg)
	}

	var cmd tea.Cmd
	prevValue := m.textarea.Value()
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)

	if m.textarea.Value() != prevValue {
		m.updateTextareaHeight()
		m.suggestions.UpdateSuggestions(m.textarea.Value())
	}

	if m.streaming || m.fetchingTokenLimits || m.compacting {
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	if !m.ready {
		return "\n  Loading..."
	}

	if m.selector.IsActive() {
		return m.selector.Render()
	}
	if m.toolSelector.IsActive() {
		return m.toolSelector.Render()
	}
	if m.skillSelector.IsActive() {
		return m.skillSelector.Render()
	}
	if m.agentSelector.IsActive() {
		return m.agentSelector.Render()
	}
	if m.mcpSelector.IsActive() {
		return m.mcpSelector.Render()
	}
	if m.pluginSelector.IsActive() {
		return m.pluginSelector.Render()
	}
	if m.sessionSelector.IsActive() {
		return m.sessionSelector.Render()
	}
	if m.memorySelector.IsActive() {
		return m.memorySelector.Render()
	}

	separator := separatorStyle.Render(strings.Repeat("─", m.width))

	// Helper: render todo list prefix for interactive prompts
	todoPrefix := ""
	if todoView := m.renderTodoList(); todoView != "" {
		todoPrefix = strings.TrimSuffix(todoView, "\n") + "\n"
	}

	// Plan prompt: show plan content + menu in the managed region
	if m.planPrompt != nil && m.planPrompt.IsActive() {
		planContent := m.planPrompt.RenderContent()
		planMenu := m.planPrompt.RenderMenu()
		return fmt.Sprintf("%s\n%s%s\n%s\n%s", planContent, todoPrefix, separator, planMenu, separator)
	}

	// Interactive prompts: show todo list above the prompt
	if m.permissionPrompt.IsActive() {
		return fmt.Sprintf("%s%s\n%s", todoPrefix, separator, m.permissionPrompt.Render())
	}

	if m.questionPrompt.IsActive() {
		return fmt.Sprintf("%s%s\n%s", todoPrefix, separator, m.questionPrompt.Render())
	}

	if m.enterPlanPrompt.IsActive() {
		return fmt.Sprintf("%s%s\n%s", todoPrefix, separator, m.enterPlanPrompt.Render())
	}

	// Active content: streaming message, tool spinner
	activeContent := m.renderActiveContent()

	prompt := inputPromptStyle.Render("❯ ")
	pendingImagesView := m.renderPendingImages()
	inputView := prompt + m.textarea.View()

	// Build the managed region
	var parts []string

	if activeContent != "" {
		parts = append(parts, activeContent)
	}

	// Task list: show between active content and input
	if todoView := m.renderTodoList(); todoView != "" {
		parts = append(parts, strings.TrimSuffix(todoView, "\n"))
	}

	if pendingImagesView != "" {
		parts = append(parts, strings.TrimSuffix(pendingImagesView, "\n"))
	}

	// Show spinner when fetching token limits
	if m.fetchingTokenLimits {
		spinnerView := thinkingStyle.Render(m.spinner.View() + " Fetching token limits...")
		parts = append(parts, spinnerView)
	}

	// Show spinner when compacting conversation
	if m.compacting {
		spinnerView := thinkingStyle.Render(m.spinner.View() + " Compacting conversation...")
		parts = append(parts, spinnerView)
	}

	chatSection := strings.Join(parts, "\n")

	statusLine := m.renderModeStatus()
	suggestions := m.suggestions.Render(m.width)

	// Build the final view
	var view strings.Builder
	if chatSection != "" {
		view.WriteString(chatSection)
		view.WriteString("\n")
	} else if m.committedCount > 0 {
		// Add spacing between scrollback content and the input separator
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
	if statusLine != "" {
		view.WriteString("\n")
		view.WriteString(statusLine)
	}

	return view.String()
}

// commitMessages pushes uncommitted completed messages to terminal scrollback via tea.Println.
// Messages that are currently being streamed (last assistant message while streaming) are skipped.
// Assistant messages with tool calls are not committed until all their tool results are present,
// ensuring that tool results are rendered inline with their tool calls.
func (m *model) commitMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(true)
}

// commitAllMessages pushes all messages to scrollback (used for session resume)
func (m *model) commitAllMessages() []tea.Cmd {
	return m.commitMessagesWithCheck(false)
}

// commitMessagesWithCheck is the shared implementation for committing messages.
// When checkReady is true, it waits for streaming to complete and tool results to arrive.
func (m *model) commitMessagesWithCheck(checkReady bool) []tea.Cmd {
	var cmds []tea.Cmd
	lastIdx := len(m.messages) - 1

	for i := m.committedCount; i < len(m.messages); i++ {
		msg := m.messages[i]

		if checkReady {
			// Don't commit the in-progress streaming message
			if i == lastIdx && msg.role == roleAssistant && m.streaming {
				break
			}
			// Wait for all tool results before committing an assistant message with tool calls
			if msg.role == roleAssistant && len(msg.toolCalls) > 0 && !m.hasAllToolResults(i) {
				break
			}
		}

		if rendered := m.renderSingleMessage(i); rendered != "" {
			cmds = append(cmds, tea.Println(rendered))
		}
		m.committedCount = i + 1
	}
	return cmds
}

// hasAllToolResults checks if all tool results for the assistant message at idx are present.
func (m *model) hasAllToolResults(idx int) bool {
	toolCalls := m.messages[idx].toolCalls
	if len(toolCalls) == 0 {
		return true
	}
	endIdx := idx + 1 + len(toolCalls)
	if endIdx > len(m.messages) {
		return false
	}
	for j := idx + 1; j < endIdx; j++ {
		if m.messages[j].toolResult == nil {
			return false
		}
	}
	return true
}

func (m model) continueWithToolResults() tea.Cmd {
	return func() tea.Msg {
		return streamContinueMsg{
			messages: m.convertMessagesToProvider(),
			modelID:  m.getModelID(),
		}
	}
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
		case message.ChunkTypeText:
			return streamChunkMsg{text: chunk.Text}
		case message.ChunkTypeThinking:
			return streamChunkMsg{thinking: chunk.Text}
		case message.ChunkTypeDone:
			var usage *message.Usage
			if chunk.Response != nil {
				usage = &chunk.Response.Usage
			}
			if chunk.Response != nil && len(chunk.Response.ToolCalls) > 0 {
				return streamChunkMsg{
					done:       true,
					toolCalls:  chunk.Response.ToolCalls,
					stopReason: chunk.Response.StopReason,
					usage:      usage,
				}
			}
			return streamChunkMsg{done: true, usage: usage}
		case message.ChunkTypeError:
			return streamChunkMsg{err: chunk.Error}
		case message.ChunkTypeToolStart:
			return streamChunkMsg{text: "", buildingToolName: chunk.ToolName}
		case message.ChunkTypeToolInput:
			return streamChunkMsg{text: ""}
		default:
			return streamChunkMsg{text: ""}
		}
	}
}

func isGitRepo(dir string) bool {
	_, err := os.Stat(dir + "/.git")
	return err == nil
}

func (m model) convertMessagesToProvider() []message.Message {
	providerMsgs := make([]message.Message, 0, len(m.messages))
	for _, msg := range m.messages {
		// Skip UI-only messages
		if msg.role == roleNotice {
			continue
		}

		providerMsg := message.Message{
			Role:      message.Role(msg.role),
			Content:   msg.content,
			Images:    msg.images,
			ToolCalls: msg.toolCalls,
			Thinking:  msg.thinking,
		}

		if msg.toolResult != nil {
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

// configureLoop updates the loop's configuration from the current model state.
func (m *model) configureLoop(extra []string) {
	var mcpToolsGetter func() []provider.Tool
	if m.mcpRegistry != nil {
		mcpToolsGetter = m.mcpRegistry.GetToolSchemas
	}

	// Cache memory content to avoid re-reading files from disk every turn
	if m.cachedMemory == "" {
		m.cachedMemory = system.LoadMemory(m.cwd)
	}

	m.loop.Client = &client.Client{
		Provider:  m.llmProvider,
		Model:     m.getModelID(),
		MaxTokens: m.getMaxTokens(),
	}
	m.loop.System = &system.System{
		Client:   m.loop.Client,
		Cwd:      m.cwd,
		IsGit:    isGitRepo(m.cwd),
		PlanMode: m.planMode,
		Extra:    extra,
		Memory:   m.cachedMemory,
	}
	m.loop.Tool = &tool.Set{
		Disabled: m.disabledTools,
		PlanMode: m.planMode,
		MCP:      mcpToolsGetter,
	}
	m.loop.Permission = nil // TUI uses its own permission system
	m.loop.Hooks = m.hookEngine
}

func (m model) getModelID() string {
	if m.currentModel != nil {
		return m.currentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

// buildExtraContext returns additional context for the system prompt including
// available skills and agents metadata.
func (m model) buildExtraContext() []string {
	var extra []string
	if skill.DefaultRegistry != nil {
		if metadata := skill.DefaultRegistry.GetAvailableSkillsPrompt(); metadata != "" {
			extra = append(extra, metadata)
		}
	}
	if agent.DefaultRegistry != nil {
		if metadata := agent.DefaultRegistry.GetAgentPromptForLLM(); metadata != "" {
			extra = append(extra, metadata)
		}
	}
	return extra
}

func (m *model) cycleOperationMode() {
	m.operationMode = m.operationMode.Next()

	m.sessionPermissions.AllowAllEdits = false
	m.sessionPermissions.AllowAllWrites = false
	m.sessionPermissions.AllowAllBash = false
	m.sessionPermissions.AllowAllSkills = false

	if m.operationMode == modeAutoAccept {
		m.sessionPermissions.AllowAllEdits = true
		m.sessionPermissions.AllowAllWrites = true
		for _, pattern := range config.CommonAllowPatterns {
			m.sessionPermissions.AllowPattern(pattern)
		}
	}

	m.planMode = (m.operationMode == modePlan)

	// Update hook engine permission mode
	if m.hookEngine != nil {
		var mode string
		switch m.operationMode {
		case modeAutoAccept:
			mode = "auto"
		case modePlan:
			mode = "plan"
		default:
			mode = "normal"
		}
		m.hookEngine.SetPermissionMode(mode)
	}

}

const maxHistoryEntries = 500

func historyFilePath(cwd string) string {
	return filepath.Join(cwd, ".gen", "history")
}

func loadInputHistory(cwd string) []string {
	path := historyFilePath(cwd)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var history []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Unescape newlines
		entry := strings.ReplaceAll(line, "\\n", "\n")
		entry = strings.ReplaceAll(entry, "\\\\", "\\")
		if entry != "" {
			history = append(history, entry)
		}
	}
	// Keep only the last maxHistoryEntries
	if len(history) > maxHistoryEntries {
		history = history[len(history)-maxHistoryEntries:]
	}
	return history
}

func saveInputHistory(cwd string, history []string) {
	path := historyFilePath(cwd)
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	// Keep only the last maxHistoryEntries
	entries := history
	if len(entries) > maxHistoryEntries {
		entries = entries[len(entries)-maxHistoryEntries:]
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, entry := range entries {
		// Escape backslashes first, then newlines
		escaped := strings.ReplaceAll(entry, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		fmt.Fprintln(w, escaped)
	}
	w.Flush()
}

// installPlugin handles plugin installation asynchronously
func (m model) installPlugin(msg PluginInstallMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		installer := plugin.NewInstaller(plugin.DefaultRegistry, m.cwd)
		if err := installer.LoadMarketplaces(); err != nil {
			return PluginInstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		pluginRef := msg.PluginName
		if msg.Marketplace != "" {
			pluginRef = msg.PluginName + "@" + msg.Marketplace
		}

		if err := installer.Install(ctx, pluginRef, msg.Scope); err != nil {
			return PluginInstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		return PluginInstallResultMsg{PluginName: msg.PluginName, Success: true}
	}
}

