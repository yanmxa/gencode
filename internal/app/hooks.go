// Hook integration: event bridges (task/worktree/config → hook system)
// and the autonomous agent runner for hook verification.
package app

import (
	"context"
	"fmt"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/task"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/worktree"
)

// --- Event bridges ---

type taskHookBridge struct {
	engine        *hook.Engine
	notifications *appagent.NotificationQueue
}

func (b taskHookBridge) TaskCreated(info task.TaskInfo) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hook.TaskCreated, hook.HookInput{
		TaskID:          info.ID,
		TaskSubject:     taskSubject(info),
		TaskDescription: info.Description,
	})
}

func (b taskHookBridge) TaskCompleted(info task.TaskInfo) {
	if b.engine != nil {
		b.engine.ExecuteAsync(hook.TaskCompleted, hook.HookInput{
			TaskID:          info.ID,
			TaskSubject:     taskSubject(info),
			TaskDescription: info.Description,
		})
	}

	appagent.UpdateBackgroundWorkerTracker(info)
	if b.notifications == nil {
		return
	}
	notifInput := appagent.TaskNotificationInput{
		Info:    info,
		Subject: taskSubject(info),
		Batch:   appagent.SnapshotBackgroundBatchForTask(info.ID),
	}
	if item, ok := appagent.BuildTaskNotification(notifInput); ok {
		b.notifications.Push(item)
	}
}

type worktreeHookBridge struct {
	engine *hook.Engine
}

type configHookBridge struct {
	engine *hook.Engine
}

func (b worktreeHookBridge) WorktreeCreated(name, path string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hook.WorktreeCreate, hook.HookInput{
		Name:         name,
		WorktreePath: path,
	})
}

func (b worktreeHookBridge) WorktreeRemoved(path string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hook.WorktreeRemove, hook.HookInput{
		WorktreePath: path,
	})
}

func (b configHookBridge) ConfigChanged(source, filePath string) {
	if b.engine == nil {
		return
	}
	b.engine.ExecuteAsync(hook.ConfigChange, hook.HookInput{
		Source:   source,
		FilePath: filePath,
	})
	b.engine.ExecuteAsync(hook.FileChanged, hook.HookInput{
		Source:   source,
		FilePath: filePath,
	})
}

func taskSubject(info task.TaskInfo) string {
	switch info.Type {
	case task.TaskTypeAgent:
		if s := appagent.JoinNameDesc(info.AgentName, info.Description); s != "" {
			return s
		}
	case task.TaskTypeBash:
		if info.Command != "" {
			return info.Command
		}
	}
	return info.Description
}

func installHookBridges(engine *hook.Engine, notifications *appagent.NotificationQueue) {
	task.SetHookObserver(taskHookBridge{engine: engine, notifications: notifications})
	worktree.SetHookObserver(worktreeHookBridge{engine: engine})
	plugin.SetConfigObserver(configHookBridge{engine: engine})
	mcp.SetConfigObserver(configHookBridge{engine: engine})
}

// --- Hook agent runner ---

type HookAgentRunner struct {
	llmProvider  llm.Provider
	settings     *config.Settings
	cwd          string
	isGit        bool
	mcpRegistry  *mcp.Registry
	defaultModel string
}

func NewHookAgentRunner(llmProvider llm.Provider, settings *config.Settings, cwd string, isGit bool, mcpRegistry *mcp.Registry, defaultModel string) *HookAgentRunner {
	return &HookAgentRunner{
		llmProvider:  llmProvider,
		settings:     settings,
		cwd:          cwd,
		isGit:        isGit,
		mcpRegistry:  mcpRegistry,
		defaultModel: defaultModel,
	}
}

func (r *HookAgentRunner) RunAgentHook(ctx context.Context, userPrompt string, model string) (string, error) {
	if r == nil || r.llmProvider == nil {
		return "", fmt.Errorf("agent hook runner requires an active provider")
	}
	if model == "" {
		model = r.defaultModel
	}

	userInstructions, projectInstructions := system.LoadInstructions(r.cwd)
	loopClient := llm.NewClient(r.llmProvider, model, 0)
	loopClient.SetThinking(llm.ThinkingHigh)

	lp, err := runtime.NewLoop(runtime.LoopConfig{
		System: system.Build(system.Config{
			ProviderName:        r.llmProvider.Name(),
			ModelID:             model,
			Cwd:                 r.cwd,
			IsGit:               r.isGit,
			UserInstructions:    userInstructions,
			ProjectInstructions: projectInstructions,
			Skills:              r.skillsSection(),
			Agents:              r.agentsSection(),
			Extra: []string{
				"You are an autonomous hook verifier. Use tools when needed, keep steps minimal, and finish by returning exactly one JSON object matching the hook output schema with no markdown fences or commentary.",
			},
		}),
		Client:     loopClient,
		Tool:       &tool.Set{Disabled: r.disabledTools(), MCP: r.mcpToolsGetter()},
		Permission: permission.BypassPermissions(),
		MCP:        r.mcpCaller(),
		Cwd:        r.cwd,
	})
	if err != nil {
		return "", err
	}
	lp.AddUser(userPrompt, nil)

	result, err := lp.Run(ctx, runtime.RunOptions{MaxTurns: 16})
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("agent hook returned no result")
	}
	if result.Content == "" {
		return "", fmt.Errorf("agent hook produced empty output")
	}
	return result.Content, nil
}

func (r *HookAgentRunner) disabledTools() map[string]bool {
	if r.settings == nil || len(r.settings.DisabledTools) == 0 {
		return nil
	}
	dup := make(map[string]bool, len(r.settings.DisabledTools))
	for k, v := range r.settings.DisabledTools {
		dup[k] = v
	}
	return dup
}

func (r *HookAgentRunner) mcpToolsGetter() func() []core.ToolSchema {
	if r.mcpRegistry == nil {
		return nil
	}
	return r.mcpRegistry.GetToolSchemas
}

func (r *HookAgentRunner) mcpCaller() runtime.MCPCaller {
	if r.mcpRegistry == nil {
		return nil
	}
	return mcp.NewCaller(r.mcpRegistry)
}

func (r *HookAgentRunner) skillsSection() string {
	if skill.DefaultRegistry == nil {
		return ""
	}
	return skill.DefaultRegistry.GetSkillsSection()
}

func (r *HookAgentRunner) agentsSection() string {
	if subagent.DefaultRegistry == nil {
		return ""
	}
	return subagent.DefaultRegistry.GetAgentsSection()
}

var _ hook.AgentRunner = (*HookAgentRunner)(nil)
