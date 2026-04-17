package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/tool"
)

func (m *model) cycleOperationMode() {
	m.runtime.CycleOperationMode()
	m.runtime.ApplyModePermissions(m.cwd)

	if m.runtime.PlanEnabled {
		m.runtime.EnsurePlanStore()
	}

	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.SetPermissionMode(m.runtime.OperationModeName())
	}
}

// updateMode routes interactive prompt request messages (questions, plans, enter-plan).
// Note: response messages are handled directly in delegateToActiveModal.
func (m *model) updateMode(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case conv.ProgressQuestionMsg:
		c := m.handleQuestionRequest(conv.QuestionRequestMsg{
			Request: msg.Request,
			Reply:   msg.Reply,
		})
		return c, true
	case conv.QuestionRequestMsg:
		c := m.handleQuestionRequest(msg)
		return c, true
	case conv.PlanRequestMsg:
		c := m.handlePlanRequest(msg)
		return c, true
	case conv.EnterPlanRequestMsg:
		c := m.handleEnterPlanRequest(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handleQuestionRequest(msg conv.QuestionRequestMsg) tea.Cmd {
	m.pendingQuestion = msg.Request
	m.pendingQuestionReply = msg.Reply
	m.mode.Question.Show(msg.Request, m.width)
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleQuestionResponse(msg conv.QuestionResponseMsg) tea.Cmd {
	reply := m.pendingQuestionReply
	m.pendingQuestionReply = nil
	defer func() { m.pendingQuestion = nil }()

	if reply == nil {
		return nil
	}

	if msg.Cancelled {
		reply <- &tool.QuestionResponse{
			RequestID: msg.Request.ID,
			Cancelled: true,
		}
		return nil
	}
	reply <- msg.Response
	return nil
}

func (m *model) handlePlanRequest(msg conv.PlanRequestMsg) tea.Cmd {
	var planPath string
	if m.runtime.PlanStore != nil {
		planPath = m.runtime.PlanStore.GetPath(plan.GeneratePlanName(m.runtime.PlanTask))
	}

	cmds := m.commitMessages()

	planScrollback := m.renderPlanForScrollback(msg.Request)
	cmds = append(cmds, tea.Println(planScrollback))

	m.mode.PlanApproval.Show(msg.Request, planPath, m.width, m.height)
	return tea.Batch(cmds...)
}

func (m *model) handlePlanResponse(msg conv.PlanResponseMsg) tea.Cmd {
	if !msg.Approved {
		m.runtime.PlanEnabled = false
		m.runtime.OperationMode = setting.ModeNormal
		return m.abortToolWithError("Plan was rejected by the user. Please ask for clarification or modify your approach.", false)
	}

	planContent := msg.ModifiedPlan
	if planContent == "" && msg.Request != nil {
		planContent = msg.Request.Plan
	}

	if msg.ApproveMode != "modify" {
		m.runtime.EnsurePlanStore()
		if m.runtime.PlanStore != nil {
			savedPlan := &plan.Plan{
				Task:    m.runtime.PlanTask,
				Status:  plan.StatusApproved,
				Content: planContent,
			}
			if _, err := m.runtime.PlanStore.Save(savedPlan); err != nil {
				m.conv.Append(core.ChatMessage{
					Role:    core.RoleNotice,
					Content: fmt.Sprintf("Warning: failed to save plan: %v", err),
				})
			}
		}
	}

	switch msg.ApproveMode {
	case "clear-auto":
		return m.handlePlanClearAutoMode(planContent)
	case "auto":
		m.runtime.EnableAutoAcceptMode(m.cwd)
	case "manual":
		m.runtime.OperationMode = setting.ModeNormal
		m.runtime.PlanEnabled = false
	case "modify":
		m.runtime.OperationMode = setting.ModePlan
		m.runtime.PlanEnabled = true
	}

	return tea.Batch(m.commitMessages()...)
}

// handlePlanClearAutoMode handles the "clear-auto" approve mode for plans.
// Clears conversation, enables auto-accept, and starts implementation.
func (m *model) handlePlanClearAutoMode(planContent string) tea.Cmd {
	m.conv.Clear()
	m.runtime.EnableAutoAcceptMode(m.cwd)
	m.tool.Reset()

	userMsg := fmt.Sprintf("Implement the following approved plan step by step. Start coding immediately — do NOT explore or investigate further.\n\n%s", planContent)
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})

	return m.sendToAgent(userMsg, nil)
}

func (m *model) handleEnterPlanRequest(msg conv.EnterPlanRequestMsg) tea.Cmd {
	m.mode.PlanEntry.Show(msg.Request, m.width)
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleEnterPlanResponse(msg conv.EnterPlanResponseMsg) tea.Cmd {
	if msg.Approved {
		m.runtime.PlanEnabled = true
		m.runtime.OperationMode = setting.ModePlan
		if msg.Request != nil && msg.Request.Message != "" {
			m.runtime.PlanTask = msg.Request.Message
		}
		m.runtime.EnsurePlanStore()
	}

	return tea.Batch(m.commitMessages()...)
}
