// TUI entry point, model constructor, and Bubble Tea Init.
package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/options"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	_ "github.com/yanmxa/gencode/internal/provider/anthropic"
	_ "github.com/yanmxa/gencode/internal/provider/google"
	_ "github.com/yanmxa/gencode/internal/provider/openai"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tui/history"
	"github.com/yanmxa/gencode/internal/tui/suggest"
	"github.com/yanmxa/gencode/internal/tui/theme"
)

// NewProgram creates a configured Bubble Tea program ready to run.
func NewProgram(opts options.RunOptions) (*tea.Program, error) {
	m := newModel()
	if err := m.initState(opts); err != nil {
		return nil, err
	}
	return tea.NewProgram(m), nil
}

// initState applies RunOptions flags to the model: plugin loading,
// plan mode setup, and session restoration.
func (m *model) initState(opts options.RunOptions) error {
	if opts.PluginDir != "" {
		ctx := context.Background()
		if err := plugin.DefaultRegistry.LoadFromPath(ctx, opts.PluginDir); err != nil {
			return fmt.Errorf("failed to load plugins from %s: %w", opts.PluginDir, err)
		}
	}

	if opts.PlanMode {
		m.planMode = true
		m.planTask = opts.Prompt
		m.operationMode = modePlan

		store, err := plan.NewStore()
		if err != nil {
			return fmt.Errorf("failed to initialize plan store: %w", err)
		}
		m.planStore = store
	}

	if opts.Continue {
		sessionStore, err := session.NewStore()
		if err != nil {
			return fmt.Errorf("failed to initialize session store: %w", err)
		}
		m.sessionStore = sessionStore

		cwd, _ := os.Getwd()
		sess, err := sessionStore.GetLatestByCwd(cwd)
		if err != nil {
			return fmt.Errorf("no previous session to continue: %w", err)
		}

		m.messages = convertFromStoredMessages(sess.Messages)
		m.currentSessionID = sess.Metadata.ID

		if len(sess.Tasks) > 0 {
			tool.DefaultTodoStore.Import(sess.Tasks)
		}
	}

	if opts.Resume {
		sessionStore, err := session.NewStore()
		if err != nil {
			return fmt.Errorf("failed to initialize session store: %w", err)
		}
		m.sessionStore = sessionStore

		m.pendingSessionSelector = true
	}

	return nil
}

func newModel() model {
	cwd, _ := os.Getwd()

	// Initialize components
	ta := newTextarea()
	sp := newSpinner()
	store, llmProvider, currentModel := initializeProvider()
	mcpRegistry := initializeRegistries(cwd)
	settings := loadSettings()

	sessionID := fmt.Sprintf("session-%d", time.Now().UnixNano())
	hookEngine := hooks.NewEngine(settings, sessionID, cwd, "")

	if llmProvider != nil {
		modelID := ""
		if currentModel != nil {
			modelID = currentModel.ModelID
		}
		configureTaskTool(llmProvider, cwd, modelID, hookEngine)
	}

	suggestions := newSuggestionState(cwd)

	return model{
		textarea:           ta,
		spinner:            sp,
		messages:           []chatMessage{},
		llmProvider:        llmProvider,
		store:              store,
		currentModel:       currentModel,
		inputHistory:       history.Load(cwd),
		historyIndex:       -1,
		cwd:                cwd,
		mdRenderer:         createMarkdownRenderer(defaultWidth),
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
		loop:               &core.Loop{},
	}
}

func newTextarea() textarea.Model {
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
	ta.BlurredStyle.Base = lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(theme.CurrentTheme.Muted)
	ta.KeyMap.InsertNewline.SetEnabled(true)
	return ta
}

func newSpinner() spinner.Model {
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    80 * time.Millisecond,
	}
	sp.Style = thinkingStyle
	return sp
}

func initializeProvider() (*provider.Store, provider.LLMProvider, *provider.CurrentModelInfo) {
	store, _ := provider.NewStore()
	if store == nil {
		return nil, nil, nil
	}

	currentModel := store.GetCurrentModel()
	ctx := context.Background()

	// Try to connect to current model's provider first
	if currentModel != nil {
		if p, err := provider.GetProvider(ctx, currentModel.Provider, currentModel.AuthMethod); err == nil {
			return store, p, currentModel
		}
	}

	// Fall back to any available provider
	for providerName, conn := range store.GetConnections() {
		if p, err := provider.GetProvider(ctx, provider.Provider(providerName), conn.AuthMethod); err == nil {
			return store, p, currentModel
		}
	}

	return store, nil, currentModel
}

func initializeRegistries(cwd string) *mcp.Registry {
	ctx := context.Background()

	if err := plugin.DefaultRegistry.Load(ctx, cwd); err != nil {
		log.Logger().Warn("Failed to load plugins", zap.Error(err))
	}

	if err := skill.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize skill registry", zap.Error(err))
	}

	agent.Init(cwd)

	if err := mcp.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize MCP registry", zap.Error(err))
		return nil
	}
	return mcp.DefaultRegistry
}

func loadSettings() *config.Settings {
	settings, _ := config.Load()
	if settings == nil {
		return config.Default()
	}
	return settings
}

func newSuggestionState(cwd string) suggest.State {
	suggestions := suggest.NewState(func(query string) []suggest.Suggestion {
		cmds := GetMatchingCommands(query)
		result := make([]suggest.Suggestion, len(cmds))
		for i, c := range cmds {
			result[i] = suggest.Suggestion{Name: c.Name, Description: c.Description}
		}
		return result
	})
	suggestions.SetCwd(cwd)
	return suggestions
}

func (m model) Init() tea.Cmd {
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
	return tea.Batch(textarea.Blink, m.spinner.Tick, AutoConnectMCPServers())
}
