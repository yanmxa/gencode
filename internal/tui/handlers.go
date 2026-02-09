package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/agent"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/plan"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/permission"
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
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "Error: " + err.Error()})
	} else {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: result})
		if p, err := provider.GetProvider(ctx, msg.Provider, msg.AuthMethod); err == nil {
			m.llmProvider = p
			// Configure Task tool with executor (use current model if available)
			modelID := ""
			if m.currentModel != nil {
				modelID = m.currentModel.ModelID
			}
			configureTaskTool(p, m.cwd, modelID)
		}
	}
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

func (m *model) handleModelSelected(msg ModelSelectedMsg) (tea.Model, tea.Cmd) {
	result, err := m.selector.SetModel(msg.ModelID, msg.ProviderName, msg.AuthMethod)
	if err != nil {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: "Error: " + err.Error()})
	} else {
		m.messages = append(m.messages, chatMessage{role: roleNotice, content: result})
		m.currentModel = &provider.CurrentModelInfo{
			ModelID:    msg.ModelID,
			Provider:   provider.Provider(msg.ProviderName),
			AuthMethod: msg.AuthMethod,
		}
		ctx := context.Background()
		if p, err := provider.GetProvider(ctx, provider.Provider(msg.ProviderName), msg.AuthMethod); err == nil {
			m.llmProvider = p
			// Configure Task tool with executor
			configureTaskTool(p, m.cwd, msg.ModelID)
		}
	}
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// configureTaskTool sets up the Task tool with the agent executor
func configureTaskTool(llmProvider provider.LLMProvider, cwd string, modelID string) {
	// Get Task tool from registry
	if t, ok := tool.Get("Task"); ok {
		if taskTool, ok := t.(*tool.TaskTool); ok {
			// Create executor and adapter
			executor := agent.NewExecutor(llmProvider, cwd, modelID)
			adapter := agent.NewExecutorAdapter(executor)
			taskTool.SetExecutor(adapter)
		}
	}
}

// Permission handlers

func (m *model) handlePermissionRequest(msg PermissionRequestMsg) (tea.Model, tea.Cmd) {
	if blocked, reason := m.checkPermissionHook(msg.Request); blocked {
		tc := m.pendingToolCalls[m.pendingToolIdx]
		m.messages = append(m.messages, chatMessage{
			role:     roleUser,
			toolName: tc.Name,
			toolResult: &provider.ToolResult{
				ToolCallID: tc.ID,
				Content:    "Blocked by hook: " + reason,
				IsError:    true,
			},
		})
		m.pendingToolCalls = nil
		m.pendingToolIdx = 0
		m.streaming = false
		m.viewport.SetContent(m.renderMessages())
		return m, nil
	}

	m.permissionPrompt.Show(msg.Request, m.width, m.height)
	return m, nil
}

// checkPermissionHook runs PermissionRequest hook and returns (blocked, reason).
func (m *model) checkPermissionHook(req *permission.PermissionRequest) (bool, string) {
	if m.hookEngine == nil || req == nil {
		return false, ""
	}

	toolInput := make(map[string]any)
	if req.FilePath != "" {
		toolInput["file_path"] = req.FilePath
	}
	if req.BashMeta != nil {
		toolInput["command"] = req.BashMeta.Command
	}

	outcome := m.hookEngine.Execute(context.Background(), hooks.PermissionRequest, hooks.HookInput{
		ToolName:  req.ToolName,
		ToolInput: toolInput,
	})
	return outcome.ShouldBlock, outcome.BlockReason
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
			case "Skill":
				m.sessionPermissions.AllowAllSkills = true
			case "Task":
				m.sessionPermissions.AllowAllTasks = true
			default:
				m.sessionPermissions.AllowTool(toolName)
			}
		}

		// For Task tool, clear progress and start checking for updates
		if msg.Request != nil && msg.Request.ToolName == "Task" {
			m.taskProgress = nil
			return m, tea.Batch(
				executeApprovedTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd),
				checkTaskProgress(),
			)
		}

		return m, executeApprovedTool(m.pendingToolCalls, m.pendingToolIdx, m.cwd)
	}

	tc := m.pendingToolCalls[m.pendingToolIdx]
	m.messages = append(m.messages, chatMessage{
		role:     roleUser,
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
			role:     roleUser,
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
			role:     roleUser,
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

	// Extract plan content once
	planContent := msg.ModifiedPlan
	if planContent == "" && msg.Request != nil {
		planContent = msg.Request.Plan
	}

	// Save the plan (skip for "modify" which feeds back into plan mode)
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

		userMsg := fmt.Sprintf("Please implement the following plan:\n\n%s", planContent)
		m.messages = append(m.messages, chatMessage{role: roleUser, content: userMsg})

		m.streaming = true
		ctx, cancel := context.WithCancel(context.Background())
		m.cancelFunc = cancel
		providerMsgs := m.convertMessagesToProvider()
		m.messages = append(m.messages, chatMessage{role: roleAssistant, content: ""})
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()

		modelID := m.getModelID()
		sysPrompt := system.Prompt(system.Config{
			Provider: m.llmProvider.Name(),
			Model:    modelID,
			Cwd:      m.cwd,
			IsGit:    isGitRepo(m.cwd),
			PlanMode: false,
			Memory:   system.LoadMemory(m.cwd),
		})
		tools := m.getToolsForMode()

		m.streamChan = m.llmProvider.Stream(ctx, provider.CompletionOptions{
			Model:        modelID,
			Messages:     providerMsgs,
			MaxTokens:    m.getMaxTokens(),
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
		m.planMode = false
	case "manual":
		m.operationMode = modeNormal
		m.planMode = false
	case "modify":
		// Stay in plan mode — LLM will revise the plan based on user feedback
		m.operationMode = modePlan
	}

	return m, executeInteractiveTool(tc, msg.Response, m.cwd)
}

// Enter Plan Mode handlers

func (m *model) handleEnterPlanRequest(msg EnterPlanRequestMsg) (tea.Model, tea.Cmd) {
	m.enterPlanPrompt.Show(msg.Request, m.width)
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

func (m *model) handleEnterPlanResponse(msg EnterPlanResponseMsg) (tea.Model, tea.Cmd) {
	tc := m.pendingToolCalls[m.pendingToolIdx]

	if msg.Approved {
		// User approved entering plan mode
		m.planMode = true
		m.operationMode = modePlan
		if msg.Request != nil && msg.Request.Message != "" {
			m.planTask = msg.Request.Message
		}
		if m.planStore == nil {
			m.planStore, _ = plan.NewStore()
		}
	}

	return m, executeInteractiveTool(tc, msg.Response, m.cwd)
}

// Compact handlers

func (m *model) handleCompactResult(msg CompactResultMsg) (tea.Model, tea.Cmd) {
	m.compacting = false
	m.compactFocus = ""       // Reset focus
	m.autoCompactNext = false // Reset auto-compact flag

	if msg.Error != nil {
		m.messages = append(m.messages, chatMessage{
			role:    roleNotice,
			content: fmt.Sprintf("⚠ Compact failed: %v", msg.Error),
		})
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil
	}

	// Replace message history with the summary as a user message
	// This ensures the summary is sent to LLM as context for future messages
	m.messages = []chatMessage{{
		role:         roleUser,
		content:      fmt.Sprintf("Here is a summary of our previous conversation:\n\n%s", msg.Summary),
		isSummary:    true,
		summaryCount: msg.OriginalCount,
		expanded:     false, // Collapsed by default
	}}

	m.lastInputTokens = 0
	m.lastOutputTokens = 0

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// Token Limit handlers

func (m *model) handleTokenLimitResult(msg TokenLimitResultMsg) (tea.Model, tea.Cmd) {
	m.fetchingTokenLimits = false

	// Add result message
	var content string
	if msg.Error != nil {
		content = "Error: " + msg.Error.Error()
	} else {
		content = msg.Result
	}
	m.messages = append(m.messages, chatMessage{role: roleNotice, content: content})

	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}

// Editor finished handler

func (m *model) handleEditorFinished(msg EditorFinishedMsg) (tea.Model, tea.Cmd) {
	filePath := m.editingMemoryFile
	m.editingMemoryFile = ""

	content := fmt.Sprintf("Saved: %s", filePath)
	if msg.Err != nil {
		content = fmt.Sprintf("Editor error: %v", msg.Err)
	}

	m.messages = append(m.messages, chatMessage{role: roleNotice, content: content})
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m, nil
}
