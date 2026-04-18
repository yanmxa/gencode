// Bubble Tea Update: top-level message dispatch, key routing, input side effects,
// submit flow, approval flow, permission bridge, and mode handling.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	"github.com/yanmxa/gencode/internal/app/input"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/app/notify"
	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/image"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/perm"
)

// ============================================================
// Update dispatch and routing
// ============================================================

type overlaySelector interface {
	IsActive() bool
	HandleKeypress(tea.KeyMsg) tea.Cmd
	Render() string
}

func (m *model) overlaySelectors() []overlaySelector {
	return []overlaySelector{
		&m.userInput.Provider.Selector,
		&m.userInput.Tool,
		&m.userInput.Skill.Selector,
		&m.userInput.Agent,
		&m.userInput.MCP.Selector,
		&m.userInput.Plugin,
		&m.userInput.Session.Selector,
		&m.userInput.Memory.Selector,
		&m.userInput.Search,
	}
}

type initialPromptMsg string

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case initialPromptMsg:
		m.userInput.Textarea.SetValue(string(msg))
		return m, m.handleSubmit()
	case tea.KeyMsg:
		if c, ok := m.handleKeypress(msg); ok {
			return m, c
		}
	case tea.WindowSizeMsg:
		return m, m.handleWindowResize(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.conv.Spinner, cmd = m.conv.Spinner.Update(msg)
		return m, cmd
	case ctrlOSingleTickMsg:
		return m, m.handleCtrlOSingleTick()
	case input.PromptSuggestionMsg:
		input.HandlePromptSuggestion(&m.userInput, m.conv.Stream.Active, m.userInput.Textarea.Value(), msg)
		return m, nil
	case kit.DismissedMsg, input.ToolToggleMsg, input.SkillCycleMsg, input.AgentToggleMsg:
		return m, nil
	}

	if cmd, handled := m.routeFeatureUpdate(msg); handled {
		return m, cmd
	}
	return m, m.updateTextarea(msg)
}

func (m *model) routeFeatureUpdate(msg tea.Msg) (tea.Cmd, bool) {
	if cmd, ok := conv.Update(m, &m.conv, msg); ok {
		return cmd, true
	}
	if cmd, ok := notify.Update(m.notifyDeps(), &m.agentInput, msg); ok {
		return cmd, true
	}
	if cmd, ok := input.UpdateApproval(m.approvalDeps(), msg); ok {
		return cmd, true
	}
	if cmd, ok := m.updateMode(msg); ok {
		return cmd, true
	}
	if cmd, ok := input.Update(m.overlayDeps(), msg); ok {
		return cmd, true
	}
	if cmd, ok := trigger.Update(m.triggerDeps(), &m.systemInput, msg); ok {
		return cmd, true
	}
	return nil, false
}

func (m *model) updateTextarea(msg tea.Msg) tea.Cmd {
	cmd, changed := m.userInput.HandleTextareaUpdate(msg)
	cmds := []tea.Cmd{cmd}
	if changed {
		m.userInput.PromptSuggestion.Clear()
	}

	if m.conv.Stream.Active || m.userInput.Provider.FetchingLimits || m.conv.Compact.Active {
		var spinnerCmd tea.Cmd
		m.conv.Spinner, spinnerCmd = m.conv.Spinner.Update(msg)
		cmds = append(cmds, spinnerCmd)
	}

	return tea.Batch(cmds...)
}

// ============================================================
// Key dispatch
// ============================================================

func (m *model) handleKeypress(msg tea.KeyMsg) (tea.Cmd, bool) {
	if active, cmd := m.delegateToActiveModal(msg); active {
		return cmd, true
	}

	if c, ok := m.userInput.HandleImageSelectKey(msg); ok {
		return c, ok
	}
	if c, ok := m.userInput.HandleSuggestionKey(msg); ok {
		return c, ok
	}
	if c, ok := m.userInput.HandleQueueSelectKey(msg); ok {
		return c, ok
	}

	return m.handleInputKey(msg)
}

func (m *model) handleInputKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyRight:
		if m.userInput.PromptSuggestion.Text != "" && m.userInput.Textarea.Value() == "" {
			m.userInput.Textarea.SetValue(m.userInput.PromptSuggestion.Text)
			m.userInput.Textarea.CursorEnd()
			m.userInput.PromptSuggestion.Clear()
			return nil, true
		}

	case tea.KeyShiftTab:
		if !m.conv.Stream.Active && !m.userInput.Approval.IsActive() &&
			!m.conv.Modal.Question.IsActive() &&
			(m.conv.Modal.PlanApproval == nil || !m.conv.Modal.PlanApproval.IsActive()) &&
			!m.userInput.Provider.Selector.IsActive() && !m.userInput.Suggestions.IsVisible() {
			m.cycleOperationMode()
			return nil, true
		}

	case tea.KeyCtrlT:
		m.conv.ShowTasks = !m.conv.ShowTasks
		return nil, true

	case tea.KeyCtrlO:
		return m.handleCtrlO(), true

	case tea.KeyCtrlE:
		return m.expandCollapseAll(), true

	case tea.KeyCtrlX:
		return nil, false

	case tea.KeyCtrlU:
		if m.userInput.Queue.Len() > 0 {
			m.userInput.Queue.Clear()
			m.userInput.Queue.SelectIdx = -1
			m.userInput.Queue.Stashed = ""
			return nil, true
		}
		return nil, false

	case tea.KeyCtrlV, tea.KeyCtrlY:
		return m.pasteImageFromClipboard()

	case tea.KeyCtrlC:
		if m.userInput.Textarea.Value() != "" {
			m.userInput.Reset()
			m.userInput.History.Index = -1
			return nil, true
		}
		return m.QuitWithCancel()

	case tea.KeyCtrlD:
		if m.userInput.Textarea.Value() != "" {
			return nil, false
		}
		return m.QuitWithCancel()

	case tea.KeyCtrlL:
		_, cmd, _ := m.executeCommand(context.Background(), "/clear")
		return cmd, true

	case tea.KeyEsc:
		if m.userInput.PromptSuggestion.Text != "" {
			m.userInput.PromptSuggestion.Clear()
			return nil, true
		}
		if m.userInput.Suggestions.IsVisible() {
			m.userInput.Suggestions.Hide()
			return nil, true
		}
		if m.conv.Stream.Active {
			return m.handleStreamCancel(), true
		}
		return nil, true

	case tea.KeyUp:
		if m.userInput.Textarea.Line() == 0 {
			if m.userInput.Queue.Len() > 0 {
				m.userInput.EnterQueueSelection()
				return nil, true
			}
			m.userInput.HistoryUp()
			return nil, true
		}

	case tea.KeyDown:
		lines := strings.Count(m.userInput.Textarea.Value(), "\n")
		if m.userInput.Textarea.Line() == lines {
			m.userInput.HistoryDown()
			return nil, true
		}

	case tea.KeyEnter:
		if msg.Alt {
			m.userInput.Textarea.InsertString("\n")
			m.userInput.UpdateHeight()
			return nil, true
		}
		return m.handleSubmit(), true
	}

	return nil, false
}

func (m *model) delegateToActiveModal(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.conv.Modal.PlanApproval != nil && m.conv.Modal.PlanApproval.IsActive() {
		cmd, resp := m.conv.Modal.PlanApproval.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handlePlanResponse(*resp))
		}
		return true, cmd
	}
	if m.conv.Modal.Question.IsActive() {
		cmd, resp := m.conv.Modal.Question.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handleQuestionResponse(*resp))
		}
		return true, cmd
	}
	if m.userInput.Approval.IsActive() {
		cmd, resp := m.userInput.Approval.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handlePermBridgeDecision(permissionDecision{Approved: resp.Approved, AllowAll: resp.AllowAll, Request: resp.Request}))
		}
		return true, cmd
	}
	if m.conv.Modal.PlanEntry.IsActive() {
		cmd, resp := m.conv.Modal.PlanEntry.HandleKeypress(msg)
		if resp != nil {
			return true, tea.Batch(cmd, m.handleEnterPlanResponse(*resp))
		}
		return true, cmd
	}

	for _, s := range m.overlaySelectors() {
		if s.IsActive() {
			return true, s.HandleKeypress(msg)
		}
	}

	return false, nil
}

// ============================================================
// Key handlers: Ctrl+O, expand/collapse, window resize, scrollback
// ============================================================

const ctrlODoubleTapWindow = 300 * time.Millisecond

type ctrlOSingleTickMsg struct{}

func (m *model) handleCtrlO() tea.Cmd {
	if m.userInput.Approval.IsActive() {
		input.TogglePermissionPreview(&m.userInput)
		return nil
	}

	now := time.Now()
	if !m.userInput.LastCtrlO.IsZero() && now.Sub(m.userInput.LastCtrlO) < ctrlODoubleTapWindow {
		m.userInput.LastCtrlO = time.Time{}
		return m.expandCollapseAll()
	}

	m.userInput.LastCtrlO = now
	return tea.Tick(ctrlODoubleTapWindow, func(time.Time) tea.Msg {
		return ctrlOSingleTickMsg{}
	})
}

func (m *model) handleCtrlOSingleTick() tea.Cmd {
	if m.userInput.LastCtrlO.IsZero() {
		return nil
	}
	m.userInput.LastCtrlO = time.Time{}
	m.conv.ToggleMostRecentExpandable()
	return m.reflowScrollback()
}

func (m *model) expandCollapseAll() tea.Cmd {
	m.conv.ToggleAllExpandable()
	return m.reflowScrollback()
}

func (m *model) handleWindowResize(msg tea.WindowSizeMsg) tea.Cmd {
	oldWidth := m.width
	m.width = msg.Width
	m.height = msg.Height
	m.userInput.TerminalHeight = msg.Height

	m.conv.ResizeMDRenderer(msg.Width)

	if m.conv.Modal.PlanApproval != nil {
		m.conv.Modal.PlanApproval.SetSize(msg.Width, msg.Height)
	}

	if !m.ready {
		m.ready = true

		var cmds []tea.Cmd
		if len(m.conv.Messages) > 0 {
			cmds = append(cmds, m.commitAllMessages()...)
		} else {
			cmds = append(cmds, tea.Println(conv.RenderWelcome()))
		}

		if m.userInput.Session.PendingSelector {
			m.userInput.Session.PendingSelector = false
			if m.runtime.SessionStore != nil {
				_ = m.userInput.Session.Selector.EnterSelect(m.width, m.height, m.runtime.SessionStore, m.cwd)
			}
		}

		m.userInput.Textarea.SetWidth(msg.Width - 4 - 2)
		if len(cmds) > 0 {
			return tea.Batch(cmds...)
		}
		return nil
	}

	m.userInput.Textarea.SetWidth(msg.Width - 4 - 2)

	if oldWidth != msg.Width && m.conv.CommittedCount > 0 {
		return m.reflowScrollback()
	}

	return nil
}

func (m *model) reflowScrollback() tea.Cmd {
	committed := m.conv.CommittedCount
	m.conv.CommittedCount = 0

	var cmds []tea.Cmd
	cmds = append(cmds, tea.ClearScreen)

	for i := 0; i < committed; i++ {
		if rendered := conv.RenderSingleMessage(m.messageRenderParams(), i); rendered != "" {
			cmds = append(cmds, tea.Println(rendered))
		}
		m.conv.CommittedCount = i + 1
	}

	return tea.Sequence(cmds...)
}

// ============================================================
// Submit and command execution
// ============================================================

func (m *model) handleSubmit() tea.Cmd {
	return input.HandleSubmit(m.submitDeps())
}

func (m *model) submitDeps() input.SubmitDeps {
	return input.SubmitDeps{
		Actions:      m,
		Input:        &m.userInput,
		Conversation: &m.conv.ConversationModel,
		Runtime:      &m.runtime,
		Cwd:          m.cwd,
		HandleCommand: func(text string) (tea.Cmd, bool) {
			ctrl := input.NewCommandController(m.commandDeps())
			return ctrl.HandleSubmit(text)
		},
	}
}

func (m *model) StartProviderTurn(content string) tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "No provider connected. Use /provider to connect.",
		})
		return tea.Batch(m.CommitMessages()...)
	}

	if err := m.ensureAgentSession(); err != nil {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: "Failed to start agent: " + err.Error(),
		})
		return tea.Batch(m.CommitMessages()...)
	}

	m.runtime.DetectThinkingKeywords(content)

	var images []core.Image
	if len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		images = lastMsg.Images
	}

	return m.sendToAgent(content, images)
}

func (m *model) commandDeps() input.CommandDeps {
	return input.CommandDeps{
		Actions:      m,
		Input:        &m.userInput,
		Conversation: &m.conv.ConversationModel,
		Runtime:      &m.runtime,
		Tool:         &m.conv.Tool,
		Width:        m.width,
		Height:       m.height,
		Cwd:          m.cwd,
	}
}

func (m *model) executeCommand(ctx context.Context, inputText string) (string, tea.Cmd, bool) {
	return input.NewCommandController(m.commandDeps()).Execute(ctx, inputText)
}

// ============================================================
// Approval flow
// ============================================================

func (m *model) approvalDeps() input.ApprovalFlowDeps {
	return input.ApprovalFlowDeps{
		Actions:     m,
		Input:       &m.userInput,
		Runtime:     &m.runtime,
		Tool:        &m.conv.Tool,
		Width:       m.width,
		Height:      m.height,
		Cwd:         m.cwd,
		ProgressHub: m.conv.ProgressHub,
	}
}

func (m *model) AbortToolWithError(errorMsg string, retry bool) tea.Cmd {
	if m.conv.Tool.PendingCalls == nil || m.conv.Tool.CurrentIdx >= len(m.conv.Tool.PendingCalls) {
		m.conv.Tool.Reset()
		m.conv.Stream.Stop()
		return tea.Batch(m.CommitMessages()...)
	}
	tc := m.conv.Tool.PendingCalls[m.conv.Tool.CurrentIdx]
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, ToolName: tc.Name, ToolResult: &core.ToolResult{ToolCallID: tc.ID, Content: errorMsg, IsError: true}})
	m.cancelRemainingToolCalls(m.conv.Tool.CurrentIdx + 1)
	m.conv.Tool.Reset()
	m.conv.Stream.Stop()
	commitCmds := m.CommitMessages()
	if retry {
		commitCmds = append(commitCmds, m.ContinueOutbox())
	}
	return tea.Batch(commitCmds...)
}

// ============================================================
// Mode handling (operation mode, plan, question, enter-plan)
// ============================================================

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

func (m *model) updateMode(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case conv.ProgressQuestionMsg:
		return m.handleQuestionRequest(conv.QuestionRequestMsg{
			Request: msg.Request,
			Reply:   msg.Reply,
		}), true
	case conv.QuestionRequestMsg:
		return m.handleQuestionRequest(msg), true
	case conv.PlanRequestMsg:
		return m.handlePlanRequest(msg), true
	case conv.EnterPlanRequestMsg:
		return m.handleEnterPlanRequest(msg), true
	}
	return nil, false
}

func (m *model) handleQuestionRequest(msg conv.QuestionRequestMsg) tea.Cmd {
	m.conv.Modal.PendingQuestion = msg.Request
	m.conv.Modal.PendingQuestionReply = msg.Reply
	m.conv.Modal.Question.Show(msg.Request, m.width)
	return tea.Batch(m.CommitMessages()...)
}

func (m *model) handleQuestionResponse(msg conv.QuestionResponseMsg) tea.Cmd {
	reply := m.conv.Modal.PendingQuestionReply
	m.conv.Modal.PendingQuestionReply = nil
	defer func() { m.conv.Modal.PendingQuestion = nil }()

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

	cmds := m.CommitMessages()

	planScrollback := m.renderPlanForScrollback(msg.Request)
	cmds = append(cmds, tea.Println(planScrollback))

	m.conv.Modal.PlanApproval.Show(msg.Request, planPath, m.width, m.height)
	return tea.Batch(cmds...)
}

func (m *model) handlePlanResponse(msg conv.PlanResponseMsg) tea.Cmd {
	if !msg.Approved {
		m.runtime.PlanEnabled = false
		m.runtime.OperationMode = setting.ModeNormal
		return m.AbortToolWithError("Plan was rejected by the user. Please ask for clarification or modify your approach.", false)
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

	return tea.Batch(m.CommitMessages()...)
}

func (m *model) handlePlanClearAutoMode(planContent string) tea.Cmd {
	m.conv.Clear()
	m.runtime.EnableAutoAcceptMode(m.cwd)
	m.conv.Tool.Reset()

	userMsg := fmt.Sprintf("Implement the following approved plan step by step. Start coding immediately — do NOT explore or investigate further.\n\n%s", planContent)
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})

	return m.sendToAgent(userMsg, nil)
}

func (m *model) handleEnterPlanRequest(msg conv.EnterPlanRequestMsg) tea.Cmd {
	m.conv.Modal.PlanEntry.Show(msg.Request, m.width)
	return tea.Batch(m.CommitMessages()...)
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

	return tea.Batch(m.CommitMessages()...)
}

// ============================================================
// Input side effects
// ============================================================

func (m *model) handleStreamCancel() tea.Cmd {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	m.runtime.ClearThinkingOverride()
	m.cancelPendingToolCalls()
	m.conv.MarkLastInterrupted()

	cmds := m.CommitMessages()
	if cmd := input.DrainInputQueue(m.submitDeps()); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (m *model) cancelPendingToolCalls() {
	toolCalls := m.conv.Tool.DrainPendingCalls()
	if toolCalls == nil && len(m.conv.Messages) > 0 {
		lastMsg := m.conv.Messages[len(m.conv.Messages)-1]
		if lastMsg.Role == core.RoleAssistant {
			toolCalls = lastMsg.ToolCalls
		}
	}
	m.conv.AppendCancelledToolResults(toolCalls, func(tc core.ToolCall) string {
		if tc.Name == "TaskOutput" {
			return "Stopped waiting for background task output because the user sent a new message. The background task may still be running."
		}
		return "Tool execution interrupted because the user sent a new message."
	})
}

func (m *model) cancelRemainingToolCalls(startIdx int) {
	m.conv.AppendCancelledToolResults(m.conv.Tool.RemainingCalls(startIdx), func(core.ToolCall) string {
		return "Tool execution skipped."
	})
}

func (m *model) HandleSkillInvocation() tea.Cmd {
	if m.runtime.LLMProvider == nil {
		m.conv.AddNotice("No provider connected. Use /provider to connect.")
		m.userInput.Skill.ClearPending()
		return tea.Batch(m.CommitMessages()...)
	}
	userMsg := m.userInput.Skill.ConsumeInvocation()
	m.conv.Append(core.ChatMessage{Role: core.RoleUser, Content: userMsg})
	return m.sendToAgent(userMsg, nil)
}

func (m *model) pasteImageFromClipboard() (tea.Cmd, bool) {
	imgData, err := image.ReadImageToProviderData()
	if err != nil {
		m.conv.Append(core.ChatMessage{Role: core.RoleNotice, Content: "Image paste error: " + err.Error()})
		return tea.Batch(m.CommitMessages()...), true
	}
	if imgData == nil {
		return nil, false
	}
	label := m.userInput.AddPendingImage(*imgData)
	m.userInput.Images.Selection = input.ImageSelection{}
	m.userInput.Textarea.InsertString(label)
	m.userInput.UpdateHeight()
	return nil, true
}

func (m *model) QuitWithCancel() (tea.Cmd, bool) {
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}
	m.conv.Stream.Stop()
	if m.conv.Tool.Cancel != nil {
		m.conv.Tool.Cancel()
	}
	m.FireSessionEnd("prompt_input_exit")
	return tea.Quit, true
}

// ============================================================
// Permission bridge response
// ============================================================

type permissionDecision struct {
	Approved bool
	AllowAll bool
	Request  *perm.PermissionRequest
}

func (m *model) handlePermBridgeDecision(decision permissionDecision) tea.Cmd {
	if m.agentSess == nil {
		return nil
	}
	req := m.agentSess.pendingPermRequest
	m.agentSess.pendingPermRequest = nil
	if req == nil {
		return nil
	}
	resp := conv.PermBridgeResponse{Allow: decision.Approved, Reason: "user decision"}
	if decision.Approved {
		if decision.AllowAll && m.runtime.SessionPermissions != nil && decision.Request != nil {
			m.runtime.SessionPermissions.AllowTool(decision.Request.ToolName)
		}
		resp.Reason = "user approved"
	} else {
		resp.Reason = "user denied"
	}
	select {
	case req.Response <- resp:
	default:
	}
	return conv.PollPermBridge(m.agentSess.permBridge)
}
