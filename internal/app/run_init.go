package app

import (
	"context"
	"maps"

	"go.uber.org/zap"

	appcommand "github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
)

func initLLM() (*llm.Store, llm.Provider, *llm.CurrentModelInfo) {
	store, _ := llm.NewStore()
	if store == nil {
		return nil, nil, nil
	}

	currentModel := store.GetCurrentModel()
	ctx := context.Background()

	if currentModel != nil {
		if p, err := llm.GetProvider(ctx, currentModel.Provider, currentModel.AuthMethod); err == nil {
			return store, p, currentModel
		}
	}

	for providerName, conn := range store.GetConnections() {
		if p, err := llm.GetProvider(ctx, llm.Name(providerName), conn.AuthMethod); err == nil {
			return store, p, currentModel
		}
	}

	return store, nil, currentModel
}

// initExt loads all component registries in dependency order.
func initExt(cwd string) {
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

func initSettings(cwd string) *config.Settings {
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
