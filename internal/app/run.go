// Package app provides the unified entry point for interactive and non-interactive modes.
package app

import (
	"context"
	"fmt"
	"maps"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/config"
	appcommand "github.com/yanmxa/gencode/internal/ext/command"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/ext/skill"
	"github.com/yanmxa/gencode/internal/ext/subagent"
	"github.com/yanmxa/gencode/internal/util/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
	_ "github.com/yanmxa/gencode/internal/tool/registry"
	"github.com/yanmxa/gencode/internal/app/ui/theme"
)

// Run routes to either print mode or interactive TUI.
func Run(opts config.RunOptions) error {
	if opts.Print != "" {
		return runPrint(opts.Print)
	}

	// Resolve theme: config > selector prompt
	settings := loadSettings("")
	themeValue := settings.Theme
	if themeValue == "" {
		chosen, err := theme.RunSelector()
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

	if fm, ok := finalModel.(*model); ok {
		printExitMessage(*fm)
	}
	return nil
}

// runPrint sends a single message and streams the response to stdout.
func runPrint(userMessage string) error {
	ctx := context.Background()

	store, err := llm.NewStore()
	if err != nil {
		return fmt.Errorf("failed to load store: %w", err)
	}

	var llmProvider llm.Provider
	var modelID string

	current := store.GetCurrentModel()
	if current != nil {
		p, err := llm.GetProvider(ctx, current.Provider, current.AuthMethod)
		if err != nil {
			return fmt.Errorf("provider %s (%s) not available: %w. Run 'gen' and use /provider to connect",
				current.Provider, current.AuthMethod, err)
		}
		llmProvider = p
		modelID = current.ModelID
	} else {
		for providerName, conn := range store.GetConnections() {
			p, err := llm.GetProvider(ctx, llm.Name(providerName), conn.AuthMethod)
			if err == nil {
				llmProvider = p
				modelID = config.DefaultModel(providerName, conn.AuthMethod)
				break
			}
		}
	}

	if llmProvider == nil {
		return fmt.Errorf("no provider connected. Run 'gen' and use /provider to connect")
	}

	completionOpts := llm.CompletionOptions{
		Model:        modelID,
		MaxTokens:    config.DefaultMaxTokens,
		SystemPrompt: config.DefaultSystemPrompt,
		Messages:     []core.Message{core.UserMessage(userMessage, nil)},
		Tools:        tool.GetToolSchemas(),
	}

	streamChan := llmProvider.Stream(ctx, completionOpts)
	for chunk := range streamChan {
		switch chunk.Type {
		case core.ChunkTypeText:
			fmt.Print(chunk.Text)
		case core.ChunkTypeError:
			return chunk.Error
		case core.ChunkTypeDone:
			fmt.Println()
		}
	}

	return nil
}

// --- Infrastructure initialization ---

func initProvider() (*llm.Store, llm.Provider, *llm.CurrentModelInfo) {
	store, _ := llm.NewStore()
	if store == nil {
		return nil, nil, nil
	}

	currentModel := store.GetCurrentModel()
	ctx := context.Background()

	// Try to connect to current model's provider first
	if currentModel != nil {
		if p, err := llm.GetProvider(ctx, currentModel.Provider, currentModel.AuthMethod); err == nil {
			return store, p, currentModel
		}
	}

	// Fall back to any available provider
	for providerName, conn := range store.GetConnections() {
		if p, err := llm.GetProvider(ctx, llm.Name(providerName), conn.AuthMethod); err == nil {
			return store, p, currentModel
		}
	}

	return store, nil, currentModel
}

// initRegistries loads all component registries in dependency order.
func initRegistries(cwd string) {
	ctx := context.Background()

	if err := plugin.DefaultRegistry.Load(ctx, cwd); err != nil {
		log.Logger().Warn("Failed to load plugins", zap.Error(err))
	}
	if err := skill.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize skill registry", zap.Error(err))
	}
	appcommand.SetDynamicInfoProviders(skillCommandInfos)
	if err := appcommand.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize custom commands", zap.Error(err))
	}
	if err := subagent.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize agent registry", zap.Error(err))
	}
	if err := mcp.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize MCP registry", zap.Error(err))
	}
}

func loadSettings(cwd string) *config.Settings {
	var (
		settings *config.Settings
		err      error
	)
	if cwd != "" {
		settings, err = config.LoadForCwd(cwd)
	} else {
		settings, err = config.Load()
	}
	_ = err
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
	if src.AllowBypass != nil {
		v := *src.AllowBypass
		dst.AllowBypass = &v
	}
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
