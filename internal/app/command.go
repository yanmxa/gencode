package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/command"
	"github.com/yanmxa/gencode/internal/skill"
)

func handlerRegistry() map[string]any {
	handlers := command.BuiltinNames()
	registry := make(map[string]any, len(handlers))
	for name := range handlers {
		registry[name] = struct{}{}
	}
	return registry
}

type commandController struct {
	inner input.CommandController
}

func (m *model) commandDeps() input.CommandDeps {
	return input.CommandDeps{
		Input:                   &m.userInput,
		Conversation:            &m.conv,
		Runtime:                 &m.runtime,
		Tool:                    &m.tool,
		Width:                   m.width,
		Height:                  m.height,
		Cwd:                     m.cwd,
		CommitMessages:          m.commitMessages,
		StartProviderTurn:       m.startProviderTurn,
		HandleSkillInvocation:   m.handleSkillInvocation,
		StartExternalEditor:     startExternalEditor,
		ReloadPluginBackedState: m.reloadPluginBackedState,
		SaveSession:             m.saveSession,
		InitTaskStorage:         m.initTaskStorage,
		ReconfigureAgentTool:    m.reconfigureAgentTool,
		StopAgentSession: func() {
			if m.agentSess != nil {
				m.agentSess.stop()
				m.agentSess = nil
			}
		},
		FireSessionEnd:      m.fireSessionEnd,
		BuildCompactRequest: m.buildCompactRequest,
		SpinnerTick:         m.agentOutput.Spinner.Tick,
		ResetCronQueue: func() {
			m.systemInput.CronQueue = nil
		},
	}
}

func (m *model) commands() commandController {
	return commandController{inner: input.NewCommandController(m.commandDeps())}
}

func (c commandController) execute(ctx context.Context, inputText string) (string, tea.Cmd, bool) {
	return c.inner.Execute(ctx, inputText)
}

func (c commandController) handleSubmit(inputText string) (tea.Cmd, bool) {
	return c.inner.HandleSubmit(inputText)
}

func executeCommand(ctx context.Context, m *model, inputText string) (string, tea.Cmd, bool) {
	return m.commands().execute(ctx, inputText)
}

func executeSkillCommand(m *model, sk *skill.Skill, args string) {
	input.ApplySkillInvocation(&m.userInput, sk, args)
}

func skillCommandInfos() []command.Info {
	return input.SkillCommandInfos()
}

func handleClearCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	return m.commands().inner.HandleClearForTests(ctx, args)
}
