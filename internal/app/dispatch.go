package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/skill"
)

// --- Submit flow ---

func (m *model) handleSubmit() tea.Cmd {
	return input.HandleSubmit(m.submitDeps())
}

func (m *model) drainInputQueue() tea.Cmd {
	return input.DrainInputQueue(m.submitDeps())
}

func (m *model) submitDeps() input.SubmitDeps {
	return input.SubmitDeps{
		Input:          &m.userInput,
		Conversation:   &m.conv,
		Runtime:        &m.runtime,
		Cwd:            m.cwd,
		CommitMessages: m.commitMessages,
		HandleCommand: func(text string) (tea.Cmd, bool) {
			ctrl := input.NewCommandController(m.commandDeps())
			return ctrl.HandleSubmit(text)
		},
		QuitWithCancel:    m.quitWithCancel,
		StartProviderTurn: m.startProviderTurn,
	}
}

// startProviderTurn starts an LLM turn by sending the user message to the agent.
func (m *model) startProviderTurn(content string) tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "No provider connected. Use /provider to connect.",
		})
		return tea.Batch(m.commitMessages()...)
	}

	if err := m.ensureAgentSession(); err != nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "Failed to start agent: " + err.Error(),
		})
		return tea.Batch(m.commitMessages()...)
	}

	m.runtime.DetectThinkingKeywords(content)

	var images []core.Image
	if len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		images = lastMsg.Images
	}

	return m.sendToAgent(content, images)
}

// --- Command execution ---

func (m *model) commandDeps() input.CommandDeps {
	return input.CommandDeps{
		Input:                   &m.userInput,
		Conversation:            &m.conv,
		Runtime:                 &m.runtime,
		Tool:                    &m.conv.Tool,
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

func executeCommand(ctx context.Context, m *model, inputText string) (string, tea.Cmd, bool) {
	return input.NewCommandController(m.commandDeps()).Execute(ctx, inputText)
}

func executeSkillCommand(m *model, sk *skill.Skill, args string) {
	input.ApplySkillInvocation(&m.userInput, sk, args)
}

func handleClearCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	return input.NewCommandController(m.commandDeps()).HandleClearForTests(ctx, args)
}

// --- Approval flow ---

func (m *model) approvalDeps() input.ApprovalFlowDeps {
	return input.ApprovalFlowDeps{
		Input:                &m.userInput,
		Runtime:              &m.runtime,
		Tool:                 &m.conv.Tool,
		Width:                m.width,
		Height:               m.height,
		Cwd:                  m.cwd,
		ProgressHub:          m.agentOutput.ProgressHub,
		ContinueOutbox:       m.continueOutbox,
		AbortToolWithError:   m.abortToolWithError,
		ReloadProjectContext: m.reloadProjectContext,
	}
}

func (m *model) handlePermissionRequest(msg input.ApprovalRequestMsg) tea.Cmd {
	return input.HandlePermissionRequest(m.approvalDeps(), msg)
}

func (m *model) handleHookPermissionResult(msg input.HookPermissionResultMsg) tea.Cmd {
	return input.HandleHookPermissionResult(m.approvalDeps(), msg)
}

func (m *model) handlePermissionResponse(msg input.ApprovalResponseMsg) tea.Cmd {
	return m.handlePermBridgeDecision(permissionDecision{Approved: msg.Approved, AllowAll: msg.AllowAll, Request: msg.Request})
}

func (m *model) togglePermissionPreview() {
	input.TogglePermissionPreview(&m.userInput)
}

// abortToolWithError cancels the current tool execution and appends an error result.
func (m *model) abortToolWithError(errorMsg string, retry bool) tea.Cmd {
	if m.conv.Tool.PendingCalls == nil || m.conv.Tool.CurrentIdx >= len(m.conv.Tool.PendingCalls) {
		m.conv.Tool.Reset()
		m.conv.Stream.Stop()
		return tea.Batch(m.commitMessages()...)
	}
	tc := m.conv.Tool.PendingCalls[m.conv.Tool.CurrentIdx]
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, ToolName: tc.Name, ToolResult: &core.ToolResult{ToolCallID: tc.ID, Content: errorMsg, IsError: true}})
	m.cancelRemainingToolCalls(m.conv.Tool.CurrentIdx + 1)
	m.conv.Tool.Reset()
	m.conv.Stream.Stop()
	commitCmds := m.commitMessages()
	if retry {
		commitCmds = append(commitCmds, m.continueOutbox())
	}
	return tea.Batch(commitCmds...)
}
