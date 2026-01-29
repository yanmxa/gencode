package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
)

// Provider and model selection handlers

func (m *model) handleProviderConnectResult(msg ProviderConnectResultMsg) (tea.Model, tea.Cmd) {
	m.selector.HandleConnectResult(msg)
	return m, nil
}

func (m *model) handleProviderSelected(msg ProviderSelectedMsg) (tea.Model, tea.Cmd) {
	ctx := context.Background()
	result, err := m.selector.ConnectProvider(ctx, msg.Provider, msg.AuthMethod)
	if err != nil {
		m.messages = append(m.messages, chatMessage{role: "system", content: "Error: " + err.Error()})
	} else {
		m.messages = append(m.messages, chatMessage{role: "system", content: result})
		if p, err := provider.GetProvider(ctx, msg.Provider, msg.AuthMethod); err == nil {
			m.llmProvider = p
		}
	}
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

func (m *model) handleModelSelected(msg ModelSelectedMsg) (tea.Model, tea.Cmd) {
	result, err := m.selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
	if err != nil {
		m.messages = append(m.messages, chatMessage{role: "system", content: "Error: " + err.Error()})
	} else {
		m.messages = append(m.messages, chatMessage{role: "system", content: result})
		m.currentModel = &provider.CurrentModelInfo{
			ModelID:    msg.ModelID,
			Provider:   provider.Provider(msg.ProviderName),
			AuthMethod: msg.AuthMethod,
		}
		ctx := context.Background()
		if p, err := provider.GetProvider(ctx, provider.Provider(msg.ProviderName), msg.AuthMethod); err == nil {
			m.llmProvider = p
		}
	}
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// Permission handlers

func (m *model) handlePermissionRequest(msg PermissionRequestMsg) (tea.Model, tea.Cmd) {
	m.permissionPrompt.Show(msg.Request, m.width, m.height)
	return m, nil
}

func (m *model) handlePermissionResponse(msg PermissionResponseMsg) (tea.Model, tea.Cmd) {
	if msg.Approved {
		if msg.AllowAll && m.sessionPermissions != nil && msg.Request != nil {
			toolName := msg.Request.ToolName
			switch toolName {
			case "Edit":
				m.sessionPermissions.AllowAllEdits = true
			case "Write":
				m.sessionPermissions.AllowAllWrites = true
			case "Bash":
				m.sessionPermissions.AllowAllBash = true
			default:
				m.sessionPermissions.AllowTool(toolName)
			}
		}
		return m, executeApprovedTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd)
	}

	tc := m.pendingToolCalls[m.pendingToolIdx]
	m.messages = append(m.messages, chatMessage{
		role:     "user",
		toolName: tc.Name,
		toolResult: &provider.ToolResult{
			ToolCallID: tc.ID,
			Content:    "User denied permission",
			IsError:    true,
		},
	})
	m.pendingToolCalls = nil
	m.pendingToolIdx = 0
	m.streaming = false
	m.viewport.SetContent(m.renderMessages())
	return m, nil
}

// Interactive tool handlers (Question, Plan)

func (m *model) handleQuestionRequest(msg QuestionRequestMsg) (tea.Model, tea.Cmd) {
	m.pendingQuestion = msg.Request
	m.questionPrompt.Show(msg.Request, m.width)
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

func (m *model) handleQuestionResponse(msg QuestionResponseMsg) (tea.Model, tea.Cmd) {
	if msg.Cancelled {
		tc := m.pendingToolCalls[m.pendingToolIdx]
		m.messages = append(m.messages, chatMessage{
			role:     "user",
			toolName: tc.Name,
			toolResult: &provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    "User cancelled the question prompt",
				IsError:    true,
			},
		})
		m.pendingToolCalls = nil
		m.pendingToolIdx = 0
		m.pendingQuestion = nil
		m.streaming = false
		m.viewport.SetContent(m.renderMessages())
		return m, nil
	}

	tc := m.pendingToolCalls[m.pendingToolIdx]
	m.pendingQuestion = nil
	return m, executeInteractiveTool(tc, msg.Response, m.cwd)
}

func (m *model) handlePlanRequest(msg PlanRequestMsg) (tea.Model, tea.Cmd) {
	var planPath string
	if m.planStore != nil {
		planPath = m.planStore.GetPath(plan.GeneratePlanName(m.planTask))
	}
	m.planPrompt.Show(msg.Request, planPath, m.width, m.height)
	chatContent := m.renderMessages()
	planContent := m.planPrompt.RenderContent()
	m.viewport.SetContent(chatContent + "\n" + planContent)
	m.viewport.GotoBottom()
	return m, nil
}

func (m *model) handlePlanResponse(msg PlanResponseMsg) (tea.Model, tea.Cmd) {
	if !msg.Approved {
		tc := m.pendingToolCalls[m.pendingToolIdx]
		m.messages = append(m.messages, chatMessage{
			role:     "user",
			toolName: tc.Name,
			toolResult: &provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    "Plan was rejected by the user. Please ask for clarification or modify your approach.",
				IsError:    true,
			},
		})
		m.pendingToolCalls = nil
		m.pendingToolIdx = 0
		m.streaming = false
		m.planMode = false
		m.operationMode = modeNormal
		m.viewport.SetContent(m.renderMessages())
		return m, nil
	}

	tc := m.pendingToolCalls[m.pendingToolIdx]

	if m.planStore == nil {
		m.planStore, _ = plan.NewStore()
	}
	if m.planStore != nil {
		planContent := msg.ModifiedPlan
		if planContent == "" && msg.Request != nil {
			planContent = msg.Request.Plan
		}
		savedPlan := &plan.Plan{
			Task:    m.planTask,
			Status:  plan.StatusApproved,
			Content: planContent,
		}
		m.planStore.Save(savedPlan)
	}

	switch msg.ApproveMode {
	case "clear-auto":
		m.messages = []chatMessage{}
		m.sessionPermissions.AllowAllEdits = true
		m.sessionPermissions.AllowAllWrites = true
		for _, pattern := range config.CommonAllowPatterns {
			m.sessionPermissions.AllowPattern(pattern)
		}
		m.operationMode = modeAutoAccept
		m.planMode = false

		m.pendingToolCalls = nil
		m.pendingToolIdx = 0

		planContent := msg.ModifiedPlan
		if planContent == "" && msg.Request != nil {
			planContent = msg.Request.Plan
		}
		userMsg := fmt.Sprintf("Please implement the following plan:\n\n%s", planContent)
		m.messages = append(m.messages, chatMessage{role: "user", content: userMsg})

		m.streaming = true
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelFunc = cancel
		providerMsgs := m.convertMessagesToProvider()
		m.messages = append(m.messages, chatMessage{role: "assistant", content: ""})
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		modelID := m.getModelID()
		sysPrompt := system.Prompt(system.Config{
			Provider: m.llmProvider.Name(),
			Model:    modelID,
			Cwd:      m.cwd,
			IsGit:    isGitRepo(m.cwd),
			PlanMode: false,
		})
		tools := m.getToolsForMode()

		m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
			Model:        modelID,
			Messages:     providerMsgs,
			MaxTokens:    defaultMaxTokens,
			Tools:        tools,
			SystemPrompt: sysPrompt,
		})
		return m, tea.Batch(m.waitForChunk(), m.spinner.Tick)

	case "auto":
		m.sessionPermissions.AllowAllEdits = true
		m.sessionPermissions.AllowAllWrites = true
		for _, pattern := range config.CommonAllowPatterns {
			m.sessionPermissions.AllowPattern(pattern)
		}
		m.operationMode = modeAutoAccept
	case "manual":
		m.operationMode = modeNormal
	case "modify":
		m.operationMode = modeNormal
	}

	m.planMode = false
	return m, executeInteractiveTool(tc, msg.Response, m.cwd)
}
