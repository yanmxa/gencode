// Message conversion between TUI chatMessage and provider message formats, LLM loop configuration.
package tui

import (
	"os"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/skill"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
)

func isGitRepo(dir string) bool {
	_, err := os.Stat(dir + "/.git")
	return err == nil
}

func (m model) convertMessagesToProvider() []message.Message {
	providerMsgs := make([]message.Message, 0, len(m.messages))
	for _, msg := range m.messages {
		if msg.role == roleNotice {
			continue
		}

		providerMsg := message.Message{
			Role:      message.Role(msg.role),
			Content:   msg.content,
			Images:    msg.images,
			ToolCalls: msg.toolCalls,
			Thinking:  msg.thinking,
		}

		if msg.toolResult != nil {
			tr := *msg.toolResult
			if msg.toolName != "" {
				tr.ToolName = msg.toolName
			}
			providerMsg.ToolResult = &tr
		}

		providerMsgs = append(providerMsgs, providerMsg)
	}
	return providerMsgs
}

func (m *model) configureLoop(extra []string) {
	var mcpToolsGetter func() []provider.Tool
	if m.mcpRegistry != nil {
		mcpToolsGetter = m.mcpRegistry.GetToolSchemas
	}

	if m.cachedMemory == "" {
		m.cachedMemory = system.LoadMemory(m.cwd)
	}

	m.loop.Client = &client.Client{
		Provider:  m.llmProvider,
		Model:     m.getModelID(),
		MaxTokens: m.getMaxTokens(),
	}
	m.loop.System = &system.System{
		Client:   m.loop.Client,
		Cwd:      m.cwd,
		IsGit:    isGitRepo(m.cwd),
		PlanMode: m.planMode,
		Extra:    extra,
		Memory:   m.cachedMemory,
	}
	m.loop.Tool = &tool.Set{
		Disabled: m.disabledTools,
		PlanMode: m.planMode,
		MCP:      mcpToolsGetter,
	}
	m.loop.Permission = nil
	m.loop.Hooks = m.hookEngine
}

func (m model) getModelID() string {
	if m.currentModel != nil {
		return m.currentModel.ModelID
	}
	return "claude-sonnet-4-20250514"
}

func (m model) buildExtraContext() []string {
	var extra []string
	if skill.DefaultRegistry != nil {
		if metadata := skill.DefaultRegistry.GetAvailableSkillsPrompt(); metadata != "" {
			extra = append(extra, metadata)
		}
	}
	if agent.DefaultRegistry != nil {
		if metadata := agent.DefaultRegistry.GetAgentPromptForLLM(); metadata != "" {
			extra = append(extra, metadata)
		}
	}
	return extra
}
