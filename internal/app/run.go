// Package app provides the unified entry point for interactive and non-interactive modes.
package app

import (
	"context"
	"fmt"
	"maps"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/agent"
	appthemeselect "github.com/yanmxa/gencode/internal/app/themeselect"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/options"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/provider"
	_ "github.com/yanmxa/gencode/internal/provider/anthropic"
	_ "github.com/yanmxa/gencode/internal/provider/google"
	_ "github.com/yanmxa/gencode/internal/provider/openai"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/ui/theme"
)

// RunWithOptions routes to either print mode or interactive TUI.
func RunWithOptions(opts options.RunOptions) error {
	if opts.Print != "" {
		return runNonInteractive(opts.Print)
	}

	// Resolve theme: config > selector prompt
	settings := loadSettings()
	themeValue := settings.Theme
	if themeValue == "" {
		chosen, err := appthemeselect.Run()
		if err != nil {
			return fmt.Errorf("theme selection failed: %w", err)
		}
		if chosen == "" {
			return nil // user quit
		}
		themeValue = chosen
		if err := config.SaveTheme(themeValue); err != nil {
			log.Logger().Warn("failed to save theme", zap.Error(err))
		}
	}
	theme.Init(themeValue)

	m, err := newModel(opts)
	if err != nil {
		return err
	}

	finalModel, err := tea.NewProgram(m).Run()
	if err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	if fm, ok := finalModel.(model); ok {
		printExitMessage(fm)
	}
	return nil
}

// runNonInteractive sends a single message and streams the response to stdout.
func runNonInteractive(userMessage string) error {
	ctx := context.Background()

	store, err := provider.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	var llmProvider provider.LLMProvider
	var modelID string

	current := store.GetCurrentModel()
	if current != nil {
		p, err := provider.GetProvider(ctx, current.Provider, current.AuthMethod)
		if err != nil {
			return fmt.Errorf("provider %s (%s) not available: %w. Run 'gen' and use /provider to connect",
				current.Provider, current.AuthMethod, err)
		}
		llmProvider = p
		modelID = current.ModelID
	} else {
		for providerName, conn := range store.GetConnections() {
			p, err := provider.GetProvider(ctx, provider.Provider(providerName), conn.AuthMethod)
			if err == nil {
				llmProvider = p
				modelID = options.DefaultModel(providerName, conn.AuthMethod)
				break
			}
		}
	}

	if llmProvider == nil {
		return fmt.Errorf("no provider connected. Run 'gen' and use /provider to connect")
	}

	completionOpts := options.NewCompletionOptions(modelID, userMessage)

	streamChan := llmProvider.Stream(ctx, completionOpts)
	for chunk := range streamChan {
		switch chunk.Type {
		case message.ChunkTypeText:
			fmt.Print(chunk.Text)
		case message.ChunkTypeError:
			return chunk.Error
		case message.ChunkTypeDone:
			fmt.Println()
		}
	}

	return nil
}

// --- Infrastructure initialization ---

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
		settings = config.Default()
	}
	cloned := cloneSettings(settings)
	plugin.MergePluginHooksIntoSettings(cloned)
	return cloned
}

func cloneSettings(src *config.Settings) *config.Settings {
	if src == nil {
		return config.Default()
	}
	dst := config.NewSettings()
	dst.Permissions.Allow = append([]string(nil), src.Permissions.Allow...)
	dst.Permissions.Deny = append([]string(nil), src.Permissions.Deny...)
	dst.Permissions.Ask = append([]string(nil), src.Permissions.Ask...)
	dst.Model = src.Model
	dst.Theme = src.Theme
	for k, v := range src.Env {
		dst.Env[k] = v
	}
	for k, v := range src.EnabledPlugins {
		dst.EnabledPlugins[k] = v
	}
	for k, v := range src.DisabledTools {
		dst.DisabledTools[k] = v
	}
	for event, hooks := range src.Hooks {
		clonedHooks := make([]config.Hook, len(hooks))
		for i, hook := range hooks {
			clonedHooks[i].Matcher = hook.Matcher
			clonedHooks[i].Hooks = make([]config.HookCmd, len(hook.Hooks))
			for j, cmd := range hook.Hooks {
				clonedHooks[i].Hooks[j] = config.HookCmd{
					Type:           cmd.Type,
					Command:        cmd.Command,
					Prompt:         cmd.Prompt,
					URL:            cmd.URL,
					If:             cmd.If,
					Shell:          cmd.Shell,
					Model:          cmd.Model,
					Async:          cmd.Async,
					AsyncRewake:    cmd.AsyncRewake,
					Timeout:        cmd.Timeout,
					StatusMessage:  cmd.StatusMessage,
					Once:           cmd.Once,
					Headers:        maps.Clone(cmd.Headers),
					AllowedEnvVars: append([]string(nil), cmd.AllowedEnvVars...),
				}
			}
		}
		dst.Hooks[event] = clonedHooks
	}
	return dst
}

// printExitMessage prints resume command after the TUI exits.
func printExitMessage(m model) {
	if m.session.CurrentID != "" {
		dim := lipgloss.NewStyle().Foreground(theme.CurrentTheme.TextDim)
		fmt.Println()
		fmt.Println(dim.Render("Resume this session with:"))
		fmt.Println(dim.Render("gen -r " + m.session.CurrentID))
		fmt.Println()
	}
}
