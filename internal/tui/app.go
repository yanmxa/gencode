package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
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


type chatMessage struct {
	role              string
	content           string
	toolCalls         []provider.ToolCall
	toolCallsExpanded bool
	toolResult        *provider.ToolResult
	toolName          string
	expanded          bool
	renderedInline    bool // marks this toolResult was rendered inline with its tool call
	isSummary         bool // marks this as a compact summary message
	summaryCount      int  // original message count before compaction
}

type (
	streamChunkMsg struct {
		text             string
		done             bool
		err              error
		toolCalls        []provider.ToolCall
		stopReason       string
		buildingToolName string
		usage            *provider.Usage
	}
	streamDoneMsg     struct{}
	streamContinueMsg struct {
		messages []provider.Message
		modelID  string
	}
	// EditorFinishedMsg is sent when an external editor process completes
	EditorFinishedMsg struct {
		Err error
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
	pendingToolCalls []provider.ToolCall
	pendingToolIdx   int

	// Parallel tool execution tracking
	parallelMode        bool                       // True when executing tools in parallel
	parallelResults     map[int]provider.ToolResult // Collected results by index
	parallelResultCount int                        // Number of results received

	settings           *config.Settings
	sessionPermissions *config.SessionPermissions

	questionPrompt *QuestionPrompt
	pendingQuestion *tool.QuestionRequest

	enterPlanPrompt *EnterPlanPrompt

	planMode   bool
	planTask   string
	planPrompt *PlanPrompt
	planStore  *plan.Store

	operationMode operationMode

	buildingToolName string

	disabledTools map[string]bool
	toolSelector  ToolSelectorState

	// Skill system
	skillSelector            SkillSelectorState
	pendingSkillInstructions string // Full skill content for next message
	pendingSkillArgs         string // User args for skill invocation

	// Agent management
	agentSelector AgentSelectorState

	// Task progress tracking
	activeTaskID   string   // Currently executing Task ID (for progress display)
	taskProgress   []string // Recent progress messages from Task

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
}

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

// RunWithContinue runs TUI and resumes the most recent session
func RunWithContinue() error {
	m := newModel()

	// Initialize session store
	sessionStore, err := session.NewStore()
	if err != nil {
		return fmt.Errorf("failed to initialize session store: %w", err)
	}
	m.sessionStore = sessionStore

	// Load the latest session
	sess, err := sessionStore.GetLatest()
	if err != nil {
		return fmt.Errorf("no previous session to continue: %w", err)
	}

	// Restore messages from session
	m.messages = convertFromStoredMessages(sess.Messages)
	m.currentSessionID = sess.Metadata.ID

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

func newViewport(width, height int) viewport.Model {
	return viewport.New(width, height)
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

	// Initialize skill registry
	if err := skill.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize skill registry", zap.Error(err))
	}

	// Initialize agent system (loads custom agents and state stores)
	agent.Init(cwd)

	// Configure Task tool if provider is available
	if llmProvider != nil {
		modelID := ""
		if currentModel != nil {
			modelID = currentModel.ModelID
		}
		configureTaskTool(llmProvider, cwd, modelID)
	}

	mdRenderer := createMarkdownRenderer(defaultWidth)

	settings, _ := config.Load()
	if settings == nil {
		settings = config.Default()
	}

	// Initialize hooks engine
	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	hookEngine := hooks.NewEngine(settings, sessionID, cwd, "")

	// Initialize suggestions with cwd for @ file completion
	suggestions := NewSuggestionState()
	suggestions.SetCwd(cwd)

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
		skillSelector:      NewSkillSelectorState(),
		agentSelector:      NewAgentSelectorState(),
		sessionSelector:    NewSessionSelectorState(),
		hookEngine:         hookEngine,
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
	return tea.Batch(textarea.Blink, m.spinner.Tick)
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

	case SelectorCancelledMsg:
		return m, nil

	case ToolToggleMsg:
		// Tool toggle already handled in toolSelector.Toggle()
		return m, nil

	case ToolSelectorCancelledMsg:
		return m, nil

	case SkillCycleMsg:
		// Skill state cycle already handled in skillSelector.CycleState()
		return m, nil

	case SkillSelectorCancelledMsg:
		return m, nil

	case AgentToggleMsg:
		// Agent toggle already handled in agentSelector.Toggle()
		return m, nil

	case AgentSelectorCancelledMsg:
		return m, nil

	case SessionSelectedMsg:
		return m.handleSessionSelected(msg)

	case SessionSelectorCancelledMsg:
		return m, nil

	case MemorySelectedMsg:
		return m.handleMemorySelected(msg)

	case MemorySelectorCancelledMsg:
		return m, nil

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
		m.viewport.SetContent(m.renderMessages())

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

	if m.ready {
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
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

	if m.sessionSelector.IsActive() {
		return m.sessionSelector.Render()
	}

	if m.memorySelector.IsActive() {
		return m.memorySelector.Render()
	}

	chat := m.viewport.View()

	separator := separatorStyle.Render(strings.Repeat("─", m.width))

	if m.planPrompt != nil && m.planPrompt.IsActive() {
		planMenu := m.planPrompt.RenderMenu()
		return fmt.Sprintf("%s\n%s\n%s\n%s", m.viewport.View(), separator, planMenu, separator)
	}

	if m.permissionPrompt.IsActive() {
		return fmt.Sprintf("%s\n%s\n%s", chat, separator, m.permissionPrompt.Render())
	}

	if m.questionPrompt.IsActive() {
		return fmt.Sprintf("%s\n%s\n%s", chat, separator, m.questionPrompt.Render())
	}

	if m.enterPlanPrompt.IsActive() {
		return fmt.Sprintf("%s\n%s\n%s", chat, separator, m.enterPlanPrompt.Render())
	}

	prompt := inputPromptStyle.Render("❯ ")
	inputView := prompt + m.textarea.View()

	// Show spinner in chat area when fetching token limits
	if m.fetchingTokenLimits {
		spinnerView := thinkingStyle.Render(m.spinner.View() + " Fetching token limits...")
		chatWithSpinner := chat + "\n" + spinnerView
		return fmt.Sprintf("%s\n%s\n%s\n%s", chatWithSpinner, separator, inputView, separator)
	}

	// Show spinner in chat area when compacting conversation
	if m.compacting {
		spinnerView := thinkingStyle.Render(m.spinner.View() + " Compacting conversation...")
		chatWithSpinner := chat + "\n" + spinnerView
		return fmt.Sprintf("%s\n%s\n%s\n%s", chatWithSpinner, separator, inputView, separator)
	}

	statusLine := m.renderModeStatus()

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
		case provider.ChunkTypeText:
			return streamChunkMsg{text: chunk.Text}
		case provider.ChunkTypeDone:
			var usage *provider.Usage
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
		case provider.ChunkTypeError:
			return streamChunkMsg{err: chunk.Error}
		case provider.ChunkTypeToolStart:
			return streamChunkMsg{text: "", buildingToolName: chunk.ToolName}
		case provider.ChunkTypeToolInput:
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

func (m model) convertMessagesToProvider() []provider.Message {
	providerMsgs := make([]provider.Message, 0, len(m.messages))
	for _, msg := range m.messages {
		if msg.role == "system" {
			continue
		}

		providerMsg := provider.Message{
			Role:      msg.role,
			Content:   msg.content,
			ToolCalls: msg.toolCalls,
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

func (m model) getModelID() string {
	if m.currentModel != nil {
		return m.currentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

func (m model) getToolsForMode() []provider.Tool {
	if m.planMode {
		return tool.GetPlanModeToolSchemasFiltered(m.disabledTools)
	}
	return tool.GetToolSchemasFiltered(m.disabledTools)
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

	// Recalculate viewport height when mode changes
	m.updateViewportHeight()
}

func (m *model) updateViewportHeight() {
	if m.width == 0 || m.height == 0 {
		return
	}
	inputH := 3
	separatorH := 2
	statusH := 0
	if m.operationMode != modeNormal {
		statusH = 1
	}
	chatH := m.height - inputH - separatorH - statusH
	if chatH < 1 {
		chatH = 1
	}
	m.viewport.Height = chatH
}
