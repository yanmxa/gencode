package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/plan"
)

func (m *model) handleQuestionRequest(msg QuestionRequestMsg) (tea.Model, tea.Cmd) {
	m.pendingQuestion = msg.Request
	m.questionPrompt.Show(msg.Request, m.width)
	return m, tea.Batch(m.commitMessages()...)
}

func (m *model) handleQuestionResponse(msg QuestionResponseMsg) (tea.Model, tea.Cmd) {
	if msg.Cancelled {
		m.pendingQuestion = nil
		return m.abortToolWithError("User cancelled the question prompt")
	}

	tc := m.pendingToolCalls[m.pendingToolIdx]
	m.pendingQuestion = nil
	return m, ExecuteInteractive(tc, msg.Response, m.cwd)
}

func (m *model) handlePlanRequest(msg PlanRequestMsg) (tea.Model, tea.Cmd) {
	var planPath string
	if m.planStore != nil {
		planPath = m.planStore.GetPath(plan.GeneratePlanName(m.planTask))
	}

	cmds := m.commitMessages()

	planScrollback := m.renderPlanForScrollback(msg.Request)
	cmds = append(cmds, tea.Println(planScrollback))

	m.planPrompt.Show(msg.Request, planPath, m.width, m.height)
	return m, tea.Batch(cmds...)
}

func (m *model) handlePlanResponse(msg PlanResponseMsg) (tea.Model, tea.Cmd) {
	if !msg.Approved {
		m.planMode = false
		m.operationMode = modeNormal
		return m.abortToolWithError("Plan was rejected by the user. Please ask for clarification or modify your approach.")
	}

	tc := m.pendingToolCalls[m.pendingToolIdx]

	planContent := msg.ModifiedPlan
	if planContent == "" && msg.Request != nil {
		planContent = msg.Request.Plan
	}

	if msg.ApproveMode != "modify" {
		if m.planStore == nil {
			m.planStore, _ = plan.NewStore()
		}
		if m.planStore != nil {
			savedPlan := &plan.Plan{
				Task:    m.planTask,
				Status:  plan.StatusApproved,
				Content: planContent,
			}
			if _, err := m.planStore.Save(savedPlan); err != nil {
				m.messages = append(m.messages, chatMessage{
					role:    roleNotice,
					content: fmt.Sprintf("Warning: failed to save plan: %v", err),
				})
			}
		}
	}

	switch msg.ApproveMode {
	case "clear-auto":
		return m.handlePlanClearAutoMode(planContent)
	case "auto":
		m.enableAutoAcceptMode()
	case "manual":
		m.operationMode = modeNormal
		m.planMode = false
	case "modify":
		m.operationMode = modePlan
	}

	return m, ExecuteInteractive(tc, msg.Response, m.cwd)
}

// handlePlanClearAutoMode handles the "clear-auto" approve mode for plans.
// Clears conversation, enables auto-accept, and starts implementation.
func (m *model) handlePlanClearAutoMode(planContent string) (tea.Model, tea.Cmd) {
	m.messages = []chatMessage{}
	m.committedCount = 0
	m.enableAutoAcceptMode()
	m.pendingToolCalls = nil
	m.pendingToolIdx = 0

	userMsg := fmt.Sprintf("Implement the following approved plan step by step. Start coding immediately — do NOT explore or investigate further.\n\n%s", planContent)
	m.messages = append(m.messages, chatMessage{role: roleUser, content: userMsg})

	return m, m.startLLMStream(nil)
}

// enableAutoAcceptMode enables auto-accept permissions and sets the mode.
func (m *model) enableAutoAcceptMode() {
	m.sessionPermissions.AllowAllEdits = true
	m.sessionPermissions.AllowAllWrites = true
	for _, pattern := range config.CommonAllowPatterns {
		m.sessionPermissions.AllowPattern(pattern)
	}
	m.operationMode = modeAutoAccept
	m.planMode = false
}

func (m *model) handleEnterPlanRequest(msg EnterPlanRequestMsg) (tea.Model, tea.Cmd) {
	m.enterPlanPrompt.Show(msg.Request, m.width)
	return m, tea.Batch(m.commitMessages()...)
}

func (m *model) handleEnterPlanResponse(msg EnterPlanResponseMsg) (tea.Model, tea.Cmd) {
	tc := m.pendingToolCalls[m.pendingToolIdx]

	if msg.Approved {
		m.planMode = true
		m.operationMode = modePlan
		if msg.Request != nil && msg.Request.Message != "" {
			m.planTask = msg.Request.Message
		}
		if m.planStore == nil {
			m.planStore, _ = plan.NewStore()
		}
	}

	return m, ExecuteInteractive(tc, msg.Response, m.cwd)
}
