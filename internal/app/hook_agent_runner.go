package app

import (
	"context"
	"fmt"

	agentruntime "github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/mcp"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
)

type hookAgentRunner struct {
	llmProvider provider.LLMProvider
	settings    *config.Settings
	cwd         string
	isGit       bool
	mcpRegistry *mcp.Registry
}

func newHookAgentRunner(llmProvider provider.LLMProvider, settings *config.Settings, cwd string, isGit bool, mcpRegistry *mcp.Registry) *hookAgentRunner {
	return &hookAgentRunner{
		llmProvider: llmProvider,
		settings:    settings,
		cwd:         cwd,
		isGit:       isGit,
		mcpRegistry: mcpRegistry,
	}
}

func (r *hookAgentRunner) RunAgentHook(ctx context.Context, prompt string, model string) (string, error) {
	if r == nil || r.llmProvider == nil {
		return "", fmt.Errorf("agent hook runner requires an active provider")
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	userInstructions, projectInstructions := system.LoadInstructions(r.cwd)
	loopClient := &client.Client{
		Provider:      r.llmProvider,
		Model:         model,
		ThinkingLevel: provider.ThinkingHigh,
	}

	loop := &core.Loop{
		System: &system.System{
			Client:              loopClient,
			Cwd:                 r.cwd,
			IsGit:               r.isGit,
			UserInstructions:    userInstructions,
			ProjectInstructions: projectInstructions,
			Skills:              r.skillsSection(),
			Agents:              r.agentsSection(),
			Extra: []string{
				"You are an autonomous hook verifier. Use tools when needed, keep steps minimal, and finish by returning exactly one JSON object matching the hook output schema with no markdown fences or commentary.",
			},
		},
		Client:     loopClient,
		Tool:       &tool.Set{Disabled: r.disabledTools(), MCP: r.mcpToolsGetter()},
		Permission: permission.BypassPermissions(),
		MCP:        r.mcpCaller(),
	}
	loop.AddUser(prompt, nil)

	result, err := loop.Run(ctx, core.RunOptions{MaxTurns: 16})
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

func (r *hookAgentRunner) mcpToolsGetter() func() []provider.Tool {
	if r.mcpRegistry == nil {
		return nil
	}
	return r.mcpRegistry.GetToolSchemas
}

func (r *hookAgentRunner) mcpCaller() core.MCPCaller {
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
	if agentruntime.DefaultRegistry == nil {
		return ""
	}
	return agentruntime.DefaultRegistry.GetAgentsSection()
}

var _ hooks.AgentRunner = (*hookAgentRunner)(nil)
