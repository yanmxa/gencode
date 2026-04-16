package app

import (
	"fmt"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/core/prompt"
	"github.com/yanmxa/gencode/internal/ext/skill"
	"github.com/yanmxa/gencode/internal/ext/subagent"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/tool"
)

func (m *model) buildLoopClient() *client.Client {
	return &client.Client{
		Provider:      m.provider.LLM,
		Model:         m.getModelID(),
		MaxTokens:     m.getMaxTokens(),
		ThinkingLevel: m.effectiveThinkingLevel(),
	}
}

func (m *model) buildLoopSystem(extra []string, loopClient *client.Client) core.System {
	providerName := ""
	modelID := ""
	if loopClient != nil {
		modelID = loopClient.Model
		if loopClient.Provider != nil {
			providerName = loopClient.Provider.Name()
		}
	}
	return prompt.Build(prompt.Config{
		ProviderName:        providerName,
		ModelID:             modelID,
		Cwd:                 m.cwd,
		IsGit:               m.isGit,
		PlanMode:            m.mode.Enabled,
		UserInstructions:    m.memory.CachedUser,
		ProjectInstructions: m.memory.CachedProject,
		SessionSummary:      m.buildSessionSummaryBlock(),
		Skills:              m.buildLoopSkillsSection(),
		Agents:              m.buildLoopAgentsSection(),
		DeferredTools:       tool.FormatDeferredToolsPrompt(),
		Extra:               m.buildLoopExtra(extra),
	})
}

func (m *model) buildLoopToolSet() *tool.Set {
	return &tool.Set{
		Disabled: m.mode.DisabledTools,
		PlanMode: m.mode.Enabled,
		MCP:      m.buildMCPToolsGetter(),
	}
}

func (m *model) buildLoopExtra(extra []string) []string {
	allExtra := append([]string{}, extra...)
	if coordinator := buildCoordinatorGuidance(); coordinator != "" {
		allExtra = append(allExtra, coordinator)
	}
	if m.skill.ActiveInvocation != "" {
		allExtra = append(allExtra, m.skill.ActiveInvocation)
	}
	if reminder := m.buildTaskReminder(); reminder != "" {
		allExtra = append(allExtra, reminder)
	}
	return allExtra
}

func buildCoordinatorGuidance() string {
	return prompt.CoordinatorGuidance()
}

func (m *model) buildSessionSummaryBlock() string {
	if m.session.Summary == "" {
		return ""
	}
	return fmt.Sprintf("<session-summary>\n%s\n</session-summary>", m.session.Summary)
}

func (m *model) buildLoopSkillsSection() string {
	if skill.DefaultRegistry == nil {
		return ""
	}
	return skill.DefaultRegistry.GetSkillsSection()
}

func (m *model) buildLoopAgentsSection() string {
	if subagent.DefaultRegistry == nil {
		return ""
	}
	return subagent.DefaultRegistry.GetAgentsSection()
}

func (m *model) buildMCPToolsGetter() func() []message.ToolSchema {
	if m.mcp.Registry == nil {
		return nil
	}
	return m.mcp.Registry.GetToolSchemas
}
