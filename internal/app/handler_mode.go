package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appmode "github.com/yanmxa/gencode/internal/app/mode"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plan"
)

func (m *model) cycleOperationMode() {
	m.mode.Operation = m.mode.Operation.Next()
	m.applyOperationModePermissions()
	m.mode.Enabled = m.mode.Operation == appmode.Plan

	if m.hookEngine != nil {
		m.hookEngine.SetPermissionMode(m.operationModeName())
	}
}

// applyOperationModePermissions configures session permissions based on the current mode.
func (m *model) applyOperationModePermissions() {
	// Reset all permissions first
	m.mode.SessionPermissions.AllowAllEdits = false
	m.mode.SessionPermissions.AllowAllWrites = false
	m.mode.SessionPermissions.AllowAllBash = false
	m.mode.SessionPermissions.AllowAllSkills = false

	// Enable auto-accept permissions
	if m.mode.Operation == appmode.AutoAccept {
		m.mode.SessionPermissions.AllowAllEdits = true
		m.mode.SessionPermissions.AllowAllWrites = true
		m.mode.SessionPermissions.AddWorkingDirectory(m.cwd)
		for _, pattern := range config.CommonAllowPatterns {
			m.mode.SessionPermissions.AllowPattern(pattern)
		}
	}
}

// operationModeName returns the string name of the current operation mode.
func (m *model) operationModeName() string {
	switch m.mode.Operation {
	case appmode.AutoAccept:
		return "auto"
	case appmode.Plan:
		return "plan"
	default:
		return "default"
	}
}

// enableAutoAcceptMode enables auto-accept permissions and sets the mode.
func (m *model) enableAutoAcceptMode() {
	m.mode.SessionPermissions.AllowAllEdits = true
	m.mode.SessionPermissions.AllowAllWrites = true
	m.mode.SessionPermissions.AddWorkingDirectory(m.cwd)
	for _, pattern := range config.CommonAllowPatterns {
		m.mode.SessionPermissions.AllowPattern(pattern)
	}
	m.mode.Operation = appmode.AutoAccept
	m.mode.Enabled = false
}

// updateMode routes interactive prompt request messages (questions, plans, enter-plan).
// Note: response messages are handled directly in delegateToActiveModal.
func (m *model) updateMode(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appmode.QuestionRequestMsg:
		c := m.handleQuestionRequest(msg)
		return c, true
	case appmode.PlanRequestMsg:
		c := m.handlePlanRequest(msg)
		return c, true
	case appmode.EnterPlanRequestMsg:
		c := m.handleEnterPlanRequest(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handleQuestionRequest(msg appmode.QuestionRequestMsg) tea.Cmd {
	m.mode.PendingQuestion = msg.Request
	m.mode.Question.Show(msg.Request, m.width)
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleQuestionResponse(msg appmode.QuestionResponseMsg) tea.Cmd {
	if msg.Cancelled {
		m.mode.PendingQuestion = nil
		return m.abortToolWithError("User cancelled the question prompt")
	}

	tc := m.tool.PendingCalls[m.tool.CurrentIdx]
	m.mode.PendingQuestion = nil
	return apptool.ExecuteInteractive(m.tool.Ctx, tc, msg.Response, m.cwd)
}

func (m *model) handlePlanRequest(msg appmode.PlanRequestMsg) tea.Cmd {
	var planPath string
	if m.mode.Store != nil {
		planPath = m.mode.Store.GetPath(plan.GeneratePlanName(m.mode.Task))
	}

	cmds := m.commitMessages()

	planScrollback := m.renderPlanForScrollback(msg.Request)
	cmds = append(cmds, tea.Println(planScrollback))

	m.mode.PlanApproval.Show(msg.Request, planPath, m.width, m.height)
	return tea.Batch(cmds...)
}

func (m *model) handlePlanResponse(msg appmode.PlanResponseMsg) tea.Cmd {
	if !msg.Approved {
		m.mode.Enabled = false
		m.mode.Operation = appmode.Normal
		return m.abortToolWithError("Plan was rejected by the user. Please ask for clarification or modify your approach.")
	}

	tc := m.tool.PendingCalls[m.tool.CurrentIdx]

	planContent := msg.ModifiedPlan
	if planContent == "" && msg.Request != nil {
		planContent = msg.Request.Plan
	}

	if msg.ApproveMode != "modify" {
		if m.mode.Store == nil {
			m.mode.Store, _ = plan.NewStore()
		}
		if m.mode.Store != nil {
			savedPlan := &plan.Plan{
				Task:    m.mode.Task,
				Status:  plan.StatusApproved,
				Content: planContent,
			}
			if _, err := m.mode.Store.Save(savedPlan); err != nil {
				m.conv.Append(message.ChatMessage{
					Role:    message.RoleNotice,
					Content: fmt.Sprintf("Warning: failed to save plan: %v", err),
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
		m.mode.Operation = appmode.Normal
		m.mode.Enabled = false
	case "modify":
		m.mode.Operation = appmode.Plan
	}

	return apptool.ExecuteInteractive(m.tool.Ctx, tc, msg.Response, m.cwd)
}

// handlePlanClearAutoMode handles the "clear-auto" approve mode for plans.
// Clears conversation, enables auto-accept, and starts implementation.
func (m *model) handlePlanClearAutoMode(planContent string) tea.Cmd {
	m.conv.Clear()
	m.enableAutoAcceptMode()
	m.tool.Reset()

	userMsg := fmt.Sprintf("Implement the following approved plan step by step. Start coding immediately — do NOT explore or investigate further.\n\n%s", planContent)
	m.conv.Append(message.ChatMessage{Role: message.RoleUser, Content: userMsg})

	return m.startLLMStream(nil)
}

func (m *model) handleEnterPlanRequest(msg appmode.EnterPlanRequestMsg) tea.Cmd {
	m.mode.PlanEntry.Show(msg.Request, m.width)
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleEnterPlanResponse(msg appmode.EnterPlanResponseMsg) tea.Cmd {
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]

	if msg.Approved {
		m.mode.Enabled = true
		m.mode.Operation = appmode.Plan
		if msg.Request != nil && msg.Request.Message != "" {
			m.mode.Task = msg.Request.Message
		}
		if m.mode.Store == nil {
			m.mode.Store, _ = plan.NewStore()
		}
	}

	return apptool.ExecuteInteractive(m.tool.Ctx, tc, msg.Response, m.cwd)
}
