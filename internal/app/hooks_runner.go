package app

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/subagent"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/tool"
)

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
	if agent.DefaultRegistry == nil {
		return ""
	}
	return agent.DefaultRegistry.GetAgentsSection()
}

var _ hooks.AgentRunner = (*HookAgentRunner)(nil)
