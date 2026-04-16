package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	appmode "github.com/yanmxa/gencode/internal/app/mode"
	"github.com/yanmxa/gencode/internal/app/toolui"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/tool"
)

// ensurePlanStore lazily initializes the plan store if not yet created.
func (m *model) ensurePlanStore() {
	if m.mode.Store != nil {
		return
	}
	store, err := plan.NewStore()
	if err != nil {
		log.Logger().Warn("failed to initialize plan store", zap.Error(err))
	}
	m.mode.Store = store
}

func (m *model) cycleOperationMode() {
	m.mode.Operation = m.mode.Operation.NextWithBypass(m.settings != nil && m.settings.AllowBypass != nil && *m.settings.AllowBypass)
	m.applyOperationModePermissions()
	m.mode.Enabled = m.mode.Operation == config.ModePlan

	// Ensure plan store is initialized when entering plan mode via shift+tab.
	if m.mode.Enabled {
		m.ensurePlanStore()
	}

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
	m.mode.SessionPermissions.Mode = config.ModeNormal

	// Enable auto-accept permissions
	if m.mode.Operation == config.ModeAutoAccept {
		m.mode.SessionPermissions.AllowAllEdits = true
		m.mode.SessionPermissions.AllowAllWrites = true
		m.mode.SessionPermissions.AddWorkingDirectory(m.cwd)
		for _, pattern := range config.CommonAllowPatterns {
			m.mode.SessionPermissions.AllowPattern(pattern)
		}
	}

	if m.mode.Operation == config.ModeBypassPermissions {
		m.mode.SessionPermissions.Mode = config.ModeBypassPermissions
	}
}

// operationModeName returns the string name of the current operation mode.
func (m *model) operationModeName() string {
	switch m.mode.Operation {
	case config.ModeAutoAccept:
		return "auto"
	case config.ModePlan:
		return "plan"
	case config.ModeBypassPermissions:
		return "bypassPermissions"
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
	m.mode.Operation = config.ModeAutoAccept
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
	m.mode.PendingQuestionReply = msg.Reply
	m.mode.Question.Show(msg.Request, m.width)
	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleQuestionResponse(msg appmode.QuestionResponseMsg) tea.Cmd {
	reply := m.mode.PendingQuestionReply
	m.mode.PendingQuestionReply = nil

	if reply != nil {
		defer func() { m.mode.PendingQuestion = nil }()
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

	if msg.Cancelled {
		m.mode.PendingQuestion = nil
		return m.abortToolWithError("User cancelled the question prompt", false)
	}

	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		m.mode.PendingQuestion = nil
		m.tool.Reset()
		return tea.Batch(m.commitMessages()...)
	}
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]
	m.mode.PendingQuestion = nil
	return toolui.ExecuteInteractive(m.tool.Context(), tc, msg.Response, m.cwd)
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
		m.mode.Operation = config.ModeNormal
		return m.abortToolWithError("Plan was rejected by the user. Please ask for clarification or modify your approach.", false)
	}

	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		m.tool.Reset()
		return tea.Batch(m.commitMessages()...)
	}
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]

	planContent := msg.ModifiedPlan
	if planContent == "" && msg.Request != nil {
		planContent = msg.Request.Plan
	}

	if msg.ApproveMode != "modify" {
		m.ensurePlanStore()
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
		m.mode.Operation = config.ModeNormal
		m.mode.Enabled = false
	case "modify":
		m.mode.Operation = config.ModePlan
		m.mode.Enabled = true
	}

	return toolui.ExecuteInteractive(m.tool.Context(), tc, msg.Response, m.cwd)
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
	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		m.tool.Reset()
		return tea.Batch(m.commitMessages()...)
	}
	tc := m.tool.PendingCalls[m.tool.CurrentIdx]

	if msg.Approved {
		m.mode.Enabled = true
		m.mode.Operation = config.ModePlan
		if msg.Request != nil && msg.Request.Message != "" {
			m.mode.Task = msg.Request.Message
		}
		m.ensurePlanStore()
	}

	return toolui.ExecuteInteractive(m.tool.Context(), tc, msg.Response, m.cwd)
}
