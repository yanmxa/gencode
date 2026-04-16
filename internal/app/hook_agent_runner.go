package app

import (
	"context"
	"fmt"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core/prompt"
	"github.com/yanmxa/gencode/internal/ext/mcp"
	"github.com/yanmxa/gencode/internal/ext/skill"
	"github.com/yanmxa/gencode/internal/ext/subagent"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/tool"
)

type hookAgentRunner struct {
	llmProvider  provider.LLMProvider
	settings     *config.Settings
	cwd          string
	isGit        bool
	mcpRegistry  *mcp.Registry
	defaultModel string
}

func newHookAgentRunner(llmProvider provider.LLMProvider, settings *config.Settings, cwd string, isGit bool, mcpRegistry *mcp.Registry, defaultModel string) *hookAgentRunner {
	return &hookAgentRunner{
		llmProvider:  llmProvider,
		settings:     settings,
		cwd:          cwd,
		isGit:        isGit,
		mcpRegistry:  mcpRegistry,
		defaultModel: defaultModel,
	}
}

func (r *hookAgentRunner) RunAgentHook(ctx context.Context, userPrompt string, model string) (string, error) {
	if r == nil || r.llmProvider == nil {
		return "", fmt.Errorf("agent hook runner requires an active provider")
	}
	if model == "" {
		model = r.defaultModel
	}

	userInstructions, projectInstructions := prompt.LoadInstructions(r.cwd)
	loopClient := client.NewClient(r.llmProvider, model)
	loopClient.ThinkingLevel = provider.ThinkingHigh

	loop, err := runtime.NewLoop(runtime.LoopConfig{
		System: prompt.Build(prompt.Config{
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
	loop.AddUser(userPrompt, nil)

	result, err := loop.Run(ctx, runtime.RunOptions{MaxTurns: 16})
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

func (r *hookAgentRunner) disabledTools() map[string]bool {
	if r.settings == nil || len(r.settings.DisabledTools) == 0 {
		return nil
	}
	dup := make(map[string]bool, len(r.settings.DisabledTools))
	for k, v := range r.settings.DisabledTools {
		dup[k] = v
	}
	return dup
}

func (r *hookAgentRunner) mcpToolsGetter() func() []message.ToolSchema {
	if r.mcpRegistry == nil {
		return nil
	}
	return r.mcpRegistry.GetToolSchemas
}

func (r *hookAgentRunner) mcpCaller() runtime.MCPCaller {
	if r.mcpRegistry == nil {
		return nil
	}
	return mcp.NewCaller(r.mcpRegistry)
}

func (r *hookAgentRunner) skillsSection() string {
	if skill.DefaultRegistry == nil {
		return ""
	}
	return skill.DefaultRegistry.GetSkillsSection()
}

func (r *hookAgentRunner) agentsSection() string {
	if subagent.DefaultRegistry == nil {
		return ""
	}
	return subagent.DefaultRegistry.GetAgentsSection()
}

var _ hooks.AgentRunner = (*hookAgentRunner)(nil)
