package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	appoutput "github.com/yanmxa/gencode/internal/app/output"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/tool"
)

// ensurePlanStore lazily initializes the plan store if not yet created.
func (m *model) ensurePlanStore() {
	if m.runtime.PlanStore != nil {
		return
	}
	store, err := plan.NewStore()
	if err != nil {
		log.Logger().Warn("failed to initialize plan store", zap.Error(err))
	}
	m.runtime.PlanStore = store
}

func (m *model) cycleOperationMode() {
	m.runtime.OperationMode = m.runtime.OperationMode.NextWithBypass(m.runtime.Settings != nil && m.runtime.Settings.AllowBypass != nil && *m.runtime.Settings.AllowBypass)
	m.applyOperationModePermissions()
	m.runtime.PlanEnabled = m.runtime.OperationMode == setting.ModePlan

	// Ensure plan store is initialized when entering plan mode via shift+tab.
	if m.runtime.PlanEnabled {
		m.ensurePlanStore()
	}

	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.SetPermissionMode(m.operationModeName())
	}
}

// applyOperationModePermissions configures session permissions based on the current mode.
func (m *model) applyOperationModePermissions() {
	// Reset all permissions first
	m.runtime.SessionPermissions.AllowAllEdits = false
	m.runtime.SessionPermissions.AllowAllWrites = false
	m.runtime.SessionPermissions.AllowAllBash = false
	m.runtime.SessionPermissions.AllowAllSkills = false
	m.runtime.SessionPermissions.Mode = setting.ModeNormal

	// Enable auto-accept permissions
	if m.runtime.OperationMode == setting.ModeAutoAccept {
		m.runtime.SessionPermissions.AllowAllEdits = true
		m.runtime.SessionPermissions.AllowAllWrites = true
		m.runtime.SessionPermissions.AddWorkingDirectory(m.cwd)
		for _, pattern := range setting.CommonAllowPatterns {
			m.runtime.SessionPermissions.AllowPattern(pattern)
		}
	}

	if m.runtime.OperationMode == setting.ModeBypassPermissions {
		m.runtime.SessionPermissions.Mode = setting.ModeBypassPermissions
	}
}

// operationModeName returns the string name of the current operation mode.
func (m *model) operationModeName() string {
	switch m.runtime.OperationMode {
	case setting.ModeAutoAccept:
		return "auto"
	case setting.ModePlan:
		return "plan"
	case setting.ModeBypassPermissions:
		return "bypassPermissions"
	default:
		return "default"
	}
}

// enableAutoAcceptMode enables auto-accept permissions and sets the mode.
func (m *model) enableAutoAcceptMode() {
	m.runtime.SessionPermissions.AllowAllEdits = true
	m.runtime.SessionPermissions.AllowAllWrites = true
	m.runtime.SessionPermissions.AddWorkingDirectory(m.cwd)
	for _, pattern := range setting.CommonAllowPatterns {
		m.runtime.SessionPermissions.AllowPattern(pattern)
	}
	m.runtime.OperationMode = setting.ModeAutoAccept
	m.runtime.PlanEnabled = false
}

// updateMode routes interactive prompt request messages (questions, plans, enter-plan).
// Note: response messages are handled directly in delegateToActiveModal.
func (m *model) updateMode(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appoutput.ProgressQuestionMsg:
		c := m.handleQuestionRequest(appoutput.QuestionRequestMsg{
			Request: msg.Request,
			Reply:   msg.Reply,
		})
		return c, true
	case appoutput.QuestionRequestMsg:
		c := m.handleQuestionRequest(msg)
		return c, true
	case appoutput.PlanRequestMsg:
		c := m.handlePlanRequest(msg)
		return c, true
	case appoutput.EnterPlanRequestMsg:
		c := m.handleEnterPlanRequest(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handleQuestionRequest(msg appoutput.QuestionRequestMsg) tea.Cmd {
	m.pendingQuestion = msg.Request
	m.pendingQuestionReply = msg.Reply
	m.mode.Question.Show(msg.Request, m.width)
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleQuestionResponse(msg appoutput.QuestionResponseMsg) tea.Cmd {
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

func (m *model) handlePlanRequest(msg appoutput.PlanRequestMsg) tea.Cmd {
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

func (m *model) handlePlanResponse(msg appoutput.PlanResponseMsg) tea.Cmd {
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
		m.ensurePlanStore()
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
		m.enableAutoAcceptMode()
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
	m.enableAutoAcceptMode()
	m.tool.Reset()

	userMsg := fmt.Sprintf("Implement the following approved plan step by step. Start coding immediately — do NOT explore or investigate further.\n\n%s", planContent)
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})

	return m.sendToAgent(userMsg, nil)
}

func (m *model) handleEnterPlanRequest(msg appoutput.EnterPlanRequestMsg) tea.Cmd {
	m.mode.PlanEntry.Show(msg.Request, m.width)
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleEnterPlanResponse(msg appoutput.EnterPlanResponseMsg) tea.Cmd {
	if msg.Approved {
		m.runtime.PlanEnabled = true
		m.runtime.OperationMode = setting.ModePlan
		if msg.Request != nil && msg.Request.Message != "" {
			m.runtime.PlanTask = msg.Request.Message
		}
		m.ensurePlanStore()
	}

	return tea.Batch(m.commitMessages()...)
}
