package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/orchestration"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/session"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/tool/fs"
)

var appCwd string

func initInfrastructure() error {
	appCwd, _ = os.Getwd()

	llm.Initialize()
	initExtensions(appCwd)
	setting.Initialize(appCwd)
	if err := initTools(appCwd); err != nil {
		return err
	}
	session.Initialize(appCwd)

	hookSettings := setting.Default().Snapshot()
	plugin.MergePluginHooksIntoSettings(hookSettings)
	hook.Initialize(hook.InitializeConfig{
		Settings:       hookSettings,
		SessionID:      session.Default().ID(),
		CWD:            appCwd,
		TranscriptPath: session.Default().TranscriptPath(),
		Completer:      buildHookCompleter(llm.Default().Provider()),
		ModelID:        llm.Default().ModelID(),
		EnvProvider:    plugin.PluginEnv,
	})

	return nil
}

func initTools(cwd string) error {
	orchestration.Default().Reset()
	cron.Default().Reset()
	cron.DefaultStore.SetStoragePath(filepath.Join(cwd, ".gen", "scheduled_tasks.json"))
	if err := cron.DefaultStore.LoadDurable(); err != nil {
		return fmt.Errorf("failed to load scheduled tasks: %w", err)
	}
	fs.SetEnvProvider(plugin.PluginEnv)
	return nil
}

func initExtensions(cwd string) {
	if err := plugin.Initialize(context.Background(), cwd); err != nil {
		log.Logger().Warn("Failed to initialize plugin", zap.Error(err))
	}
	if err := skill.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize skill", zap.Error(err))
	}
	command.SetDynamicInfoProviders(skillCommandInfos)
	if err := command.Initialize(cwd); err != nil {
		log.Logger().Warn("Failed to initialize command", zap.Error(err))
	}
	if err := subagent.Initialize(cwd, pluginAgentPaths); err != nil {
		log.Logger().Warn("Failed to initialize subagent", zap.Error(err))
	}
	if err := mcp.Initialize(cwd, pluginMCPServers); err != nil {
		log.Logger().Warn("Failed to initialize mcp", zap.Error(err))
	}
}

func pluginAgentPaths() []subagent.PluginAgentPath {
	pPaths := plugin.GetPluginAgentPaths()
	paths := make([]subagent.PluginAgentPath, len(pPaths))
	for i, p := range pPaths {
		paths[i] = subagent.PluginAgentPath{
			Path:      p.Path,
			Namespace: p.Namespace,
		}
	}
	return paths
}

func pluginMCPServers() []mcp.PluginServer {
	pServers := plugin.GetPluginMCPServers()
	servers := make([]mcp.PluginServer, len(pServers))
	for i, s := range pServers {
		servers[i] = mcp.PluginServer{
			Name:    s.Name,
			Type:    string(s.Config.Type),
			Command: s.Config.Command,
			Args:    append([]string(nil), s.Config.Args...),
			Env:     s.Config.Env,
			URL:     s.Config.URL,
			Headers: s.Config.Headers,
			Scope:   string(s.Scope),
		}
	}
	return servers
}

func commandSuggestionMatcher() func(string) []suggest.Suggestion {
	return func(query string) []suggest.Suggestion {
		cmds := command.GetMatchingCommands(query)
		result := make([]suggest.Suggestion, len(cmds))
		for i, c := range cmds {
			result[i] = suggest.Suggestion{Name: c.Name, Description: c.Description}
		}
		return result
	}
}

type agentRegistryAdapter struct {
	reg *subagent.Registry
}

func (a *agentRegistryAdapter) ListConfigs() []input.AgentConfigInfo {
	configs := a.reg.ListConfigs()
	out := make([]input.AgentConfigInfo, len(configs))
	for i, cfg := range configs {
		var tools []string
		if cfg.Tools != nil {
			tools = []string(cfg.Tools)
		}
		out[i] = input.AgentConfigInfo{
			Name:           cfg.Name,
			Description:    cfg.Description,
			Model:          cfg.Model,
			PermissionMode: string(cfg.PermissionMode),
			Tools:          tools,
			SourceFile:     cfg.SourceFile,
		}
	}
	return out
}

func (a *agentRegistryAdapter) GetDisabledAt(userLevel bool) map[string]bool {
	return a.reg.GetDisabledAt(userLevel)
}

func (a *agentRegistryAdapter) SetEnabled(name string, enabled bool, userLevel bool) error {
	return a.reg.SetEnabled(name, enabled, userLevel)
}

func skillCommandInfos() []command.Info {
	return input.SkillCommandInfos()
}

