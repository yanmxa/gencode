// agent_events.go handles all events from the core.Agent outbox.
// This is the single integration point between the core.Agent (background goroutine)
// and the Bubble Tea TUI (Model-View-Update). Every model mutation from the agent
// flows through handleAgentEvent.
package app

import (
	"context"
	"fmt"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	appagent "github.com/yanmxa/gencode/internal/app/agent"
	"github.com/yanmxa/gencode/internal/app/progress"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/llm"
	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/util/log"
)

// agentOutboxMsg carries an event from the core.Agent's outbox to the TUI.
type agentOutboxMsg struct {
	Event  core.Event
	Closed bool
}

// drainAgentOutbox returns a tea.Cmd that reads events from the agent's outbox
// channel and emits them as agentOutboxMsg for the TUI.
func drainAgentOutbox(outbox <-chan core.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-outbox
		if !ok {
			return agentOutboxMsg{Closed: true}
		}
		return agentOutboxMsg{Event: ev}
	}
}

// updateAgent handles core.Agent events (outbox events and permission bridge
// requests) in the TUI update loop.
func (m *model) updateAgent(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case agentOutboxMsg:
		if msg.Closed {
			return m.handleAgentStopped(nil), true
		}
		return m.handleAgentEvent(msg.Event), true
	case agentPermissionMsg:
		m.pendingPermBridge = msg.Request
		return m.showPermissionPrompt(msg.Request), true
	case progress.UpdateMsg:
		return m.agentOutput.HandleProgress(msg), true
	case progress.CheckTickMsg:
		return m.agentOutput.HandleProgressTick(false), true
	}
	return nil, false
}

// --- Core event dispatch ---

// handleAgentEvent processes a single event from the core.Agent outbox.
func (m *model) handleAgentEvent(ev core.Event) tea.Cmd {
	switch ev.Type {
	case core.OnStart:
		return m.continueOutbox()

	case core.OnMessage:
		return m.continueOutbox()

	case core.PreInfer:
		return m.handlePreInfer()

	case core.OnChunk:
		return m.handleChunk(ev)

	case core.PostInfer:
		return m.handlePostInfer(ev)

	case core.PreTool:
		return m.handlePreTool(ev)

	case core.PostTool:
		return m.handlePostTool(ev)

	case core.OnTurn:
		return m.handleTurn(ev)

	case core.OnStop:
		err, _ := ev.Error()
		return m.handleAgentStopped(err)

	default:
		return m.continueOutbox()
	}
}

// --- Event handlers ---

// handlePreInfer fires when the agent is about to call the LLM.
// We mark the stream as active and append an empty assistant message for
// incremental text accumulation.
func (m *model) handlePreInfer() tea.Cmd {
	m.conv.Stream.Active = true
	m.conv.Stream.BuildingTool = ""

	// Commit any pending messages (e.g. user input, tool results) to scrollback
	// before the new assistant message starts.
	commitCmds := m.commitMessages()

	// Append an empty assistant message that will accumulate streamed text.
	m.conv.Append(core.ChatMessage{Role: core.RoleAssistant, Content: ""})

	cmds := append(commitCmds, m.agentOutput.Spinner.Tick)
	cmds = append(cmds, m.continueOutbox())
	return tea.Batch(cmds...)
}

// handleChunk processes a streaming text/thinking chunk from the LLM.
func (m *model) handleChunk(ev core.Event) tea.Cmd {
	chunk, ok := ev.Chunk()
	if !ok {
		return m.continueOutbox()
	}
	if chunk.Text != "" || chunk.Thinking != "" {
		m.conv.AppendToLast(chunk.Text, chunk.Thinking)
	}
	return m.continueOutbox()
}

// handlePostInfer fires after the LLM response is fully received.
// Updates token counts, thinking signature, and tool call display state.
func (m *model) handlePostInfer(ev core.Event) tea.Cmd {
	resp, ok := ev.Response()
	if !ok {
		return m.continueOutbox()
	}

	// Update token tracking
	m.provider.InputTokens = resp.TokensIn
	m.provider.OutputTokens = resp.TokensOut
	m.conv.Compact.WarningSuppressed = false

	// Track turns for task reminder nudges
	m.conv.TurnsSinceLastTaskTool++

	// Store thinking signature for session persistence
	if resp.ThinkingSignature != "" {
		m.conv.SetLastThinkingSignature(resp.ThinkingSignature)
	}

	// If the response contains tool calls, display them on the last message
	if len(resp.ToolCalls) > 0 {
		m.conv.SetLastToolCalls(resp.ToolCalls)
	}

	// Clear building tool indicator
	m.conv.Stream.BuildingTool = ""

	return m.continueOutbox()
}

// handlePreTool fires before a tool is executed. Updates the building tool
// indicator for the spinner display.
func (m *model) handlePreTool(ev core.Event) tea.Cmd {
	if tc, ok := ev.ToolCall(); ok {
		m.conv.Stream.BuildingTool = tc.Name
	}
	return m.continueOutbox()
}

// handlePostTool fires after a tool execution completes. Applies side effects
// (cwd changes, file cache, background task tracking) and appends the tool
// result to the conversation display.
func (m *model) handlePostTool(ev core.Event) tea.Cmd {
	tr, ok := ev.ToolResult()
	if !ok {
		return m.continueOutbox()
	}

	m.conv.Stream.BuildingTool = ""

	// Retrieve side effects stored by the tool adapter
	sideEffect := tool.PopSideEffect(tr.ToolCallID)
	if sideEffect != nil {
		m.applyAgentToolSideEffects(tr.ToolName, sideEffect)
	}

	// Track task tools for reminder nudges
	if isTaskTool(tr.ToolName) {
		m.conv.TurnsSinceLastTaskTool = 0
	}

	// Clean up agent progress display
	if tool.IsAgentToolName(tr.ToolName) {
		// Clear progress for this tool (we don't have an index, clear all)
		m.agentOutput.TaskProgress = nil
	}

	// Fire PostToolUse hook asynchronously
	m.fireAgentPostToolHook(tr, sideEffect)

	// Persist overflow and append to conversation display
	m.persistToolResultOverflow(&core.ToolResult{
		ToolCallID: tr.ToolCallID,
		ToolName:   tr.ToolName,
		Content:    tr.Content,
		IsError:    tr.IsError,
	})
	m.conv.Append(core.ChatMessage{
		Role:     core.RoleUser,
		ToolName: tr.ToolName,
		ToolResult: &core.ToolResult{
			ToolCallID: tr.ToolCallID,
			ToolName:   tr.ToolName,
			Content:    tr.Content,
			IsError:    tr.IsError,
		},
	})

	return m.continueOutbox()
}

// handleTurn fires when the agent completes a think+act cycle (end_turn).
// This is the idle point — save session, fire hooks, check compaction,
// drain queued inputs.
func (m *model) handleTurn(ev core.Event) tea.Cmd {
	result, _ := ev.Result()

	// Stop streaming state
	m.conv.Stream.Stop()
	m.provider.ThinkingOverride = llm.ThinkingOff

	// Commit all pending messages to scrollback
	commitCmds := m.commitMessages()

	// Fire idle hooks (Stop + Notification)
	if m.fireIdleHooks() {
		// Stop hook blocked — send continuation to agent
		cmds := append(commitCmds, m.continueOutbox())
		return tea.Batch(cmds...)
	}

	// Save session
	if err := m.saveSession(); err != nil {
		log.Logger().Warn("failed to save session", zap.Error(err))
	}

	// Check auto-compact
	if m.shouldAutoCompact() {
		m.conv.Compact.AutoContinue = true
		commitCmds = append(commitCmds, m.triggerAutoCompact())
		return tea.Batch(commitCmds...)
	}

	// Try prompt suggestion if idle
	if cmd := m.startPromptSuggestion(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
	}

	// Drain queued inputs
	if cmd := m.drainInputQueueToAgent(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	// Drain cron queue
	if cmd := m.drainCronQueueToAgent(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	// Drain async hook queue
	if cmd := m.drainAsyncHookQueueToAgent(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	// Drain task notifications
	if cmd := m.drainTaskNotificationsToAgent(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	// Check for stop reason details (max_turns etc.)
	if result.StopReason != "" && result.StopReason != core.StopEndTurn {
		m.conv.AddNotice(fmt.Sprintf("Agent stopped: %s", result.StopReason))
		if result.StopDetail != "" {
			m.conv.AddNotice(result.StopDetail)
		}
	}

	// Continue draining outbox (agent goes back to waitForInput)
	cmds := append(commitCmds, m.continueOutbox())
	return tea.Batch(cmds...)
}

// handleAgentStopped processes agent shutdown.
func (m *model) handleAgentStopped(err error) tea.Cmd {
	m.conv.Stream.Stop()

	if err != nil {
		m.conv.AddNotice(fmt.Sprintf("Agent error: %v", err))
	}

	// Fire StopFailure hook if there was an error
	if err != nil && m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hook.StopFailure, hook.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			Error:                err.Error(),
			StopHookActive:       m.hookEngine.StopHookActive(),
		})
	}

	commitCmds := m.commitMessages()

	// Clean up agent session
	if m.agentSess != nil {
		m.agentSess.stop()
		m.agentSess = nil
	}

	return tea.Batch(commitCmds...)
}

// --- Side effects ---

// applyAgentToolSideEffects applies environment changes from tool execution.
// This mirrors the logic from the old applyEnvironmentSideEffects but works
// with the side-effect map from tool.PopSideEffect instead of toolui.ExecResultMsg.
func (m *model) applyAgentToolSideEffects(toolName string, sideEffect any) {
	resp, ok := sideEffect.(map[string]any)
	if !ok {
		return
	}

	// Background task tracking
	m.syncBackgroundTaskTrackerFromAgent(toolName, resp)

	switch toolName {
	case "Bash":
		if newCwd := getHookResponseString(resp, "cwd"); newCwd != "" {
			m.changeCwd(newCwd)
		}
	case tool.ToolEnterWorktree:
		if worktreePath := getHookResponseString(resp, "worktreePath"); worktreePath != "" {
			m.changeCwd(worktreePath)
		}
	case tool.ToolExitWorktree:
		if restoredPath := getHookResponseString(resp, "restoredPath"); restoredPath != "" {
			m.changeCwd(restoredPath)
		}
	case "Write", "Edit":
		if filePath := getHookResponseString(resp, "filePath"); filePath != "" {
			m.fireFileChanged(filePath, toolName)
			if m.fileCache != nil {
				m.fileCache.Touch(filePath)
			}
		}
	case "Read":
		if fileData, ok := resp["file"].(map[string]any); ok {
			if filePath := getHookResponseString(fileData, "filePath"); filePath != "" {
				if m.fileCache != nil {
					m.fileCache.Touch(filePath)
				}
			}
		}
	}
}

// syncBackgroundTaskTrackerFromAgent handles background agent task tracking
// when a tool result contains backgroundTask metadata.
func (m *model) syncBackgroundTaskTrackerFromAgent(toolName string, resp map[string]any) {
	if !tool.IsAgentToolName(toolName) {
		return
	}
	bg, ok := resp["backgroundTask"].(map[string]any)
	if !ok {
		return
	}
	launch := appagent.BackgroundTaskLaunch{
		TaskID:      appagent.MetadataString(bg, "taskId"),
		AgentName:   appagent.MetadataString(bg, "agentName"),
		AgentType:   appagent.MetadataString(bg, "agentType"),
		Description: appagent.MetadataString(bg, "description"),
		ResumeID:    appagent.MetadataString(bg, "resumeId"),
	}
	if launch.TaskID == "" {
		return
	}
	// For agent-managed tool calls, we don't have PendingCalls for batch spec.
	// Record as a single worker (no batch).
	childID := appagent.EnsureBackgroundWorkerTracker(launch, "", "")
	if childID != "" {
		appagent.RecordBackgroundTaskLaunch(launch, "", "", 0)
	}
}

// fireAgentPostToolHook fires the PostToolUse hook asynchronously for a tool result.
func (m *model) fireAgentPostToolHook(tr core.ToolResult, sideEffect any) {
	if m.hookEngine == nil {
		return
	}
	eventType := hook.PostToolUse
	if tr.IsError {
		eventType = hook.PostToolUseFailure
	}
	toolResponse := any(tr.Content)
	if sideEffect != nil {
		toolResponse = sideEffect
	}
	input := hook.HookInput{
		ToolName:     tr.ToolName,
		ToolUseID:    tr.ToolCallID,
		ToolResponse: toolResponse,
	}
	if tr.IsError {
		input.Error = tr.Content
	}
	m.hookEngine.ExecuteAsync(eventType, input)
}

// fireIdleHooks fires Stop hooks synchronously (honoring block decisions)
// and Notification hooks asynchronously. Returns true if a Stop hook blocked,
// in which case a continuation message is sent to the agent.
func (m *model) fireIdleHooks() bool {
	if m.hookEngine == nil {
		return false
	}

	blocked := false
	if m.hookEngine.HasHooks(hook.Stop) {
		outcome := m.hookEngine.Execute(context.Background(), hook.Stop, hook.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			StopHookActive:       m.hookEngine.StopHookActive(),
		})
		if outcome.ShouldBlock {
			// Send the block reason to the agent so it can re-evaluate
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleUser,
				Content: "Stop hook blocked: " + outcome.BlockReason,
			})
			if m.agentSess != nil {
				m.agentSess.agent.Inbox() <- core.Message{
					Role:    core.RoleUser,
					Content: "Stop hook blocked: " + outcome.BlockReason,
				}
			}
			blocked = true
		}
	}

	m.hookEngine.ExecuteAsync(hook.Notification, hook.HookInput{
		Message:          "Claude is waiting for your input",
		NotificationType: "idle_prompt",
	})
	return blocked
}

// --- Continuation injection helpers ---

// drainInputQueueToAgent pops the next queued user input and sends it to the agent.
func (m *model) drainInputQueueToAgent() tea.Cmd {
	if m.inputQueue.Len() == 0 {
		return nil
	}
	item, ok := m.inputQueue.Dequeue()
	if !ok {
		return nil
	}
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: item.Content,
		Images:  item.Images,
	})
	return m.sendToAgent(item.Content, item.Images)
}

// drainCronQueueToAgent pops one queued cron prompt and sends it to the agent.
func (m *model) drainCronQueueToAgent() tea.Cmd {
	if len(m.systemInput.CronQueue) == 0 {
		return nil
	}
	prompt := m.systemInput.CronQueue[0]
	m.systemInput.CronQueue = m.systemInput.CronQueue[1:]

	m.conv.Append(core.ChatMessage{
		Role:    core.RoleNotice,
		Content: "Scheduled task fired",
	})
	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: prompt,
	})
	return m.sendToAgent(prompt, nil)
}

// drainAsyncHookQueueToAgent pops one async hook rewake and sends it to the agent.
func (m *model) drainAsyncHookQueueToAgent() tea.Cmd {
	if m.systemInput.AsyncHookQueue == nil {
		return nil
	}
	item, ok := m.systemInput.AsyncHookQueue.Pop()
	if !ok {
		return nil
	}

	if item.Notice != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleNotice,
			Content: item.Notice,
		})
	}
	if len(item.Context) == 0 && item.ContinuationPrompt == "" {
		return nil
	}

	// Build the continuation message combining context and prompt
	var content string
	if item.ContinuationPrompt != "" {
		content = item.ContinuationPrompt
	}
	for _, ctx := range item.Context {
		if content != "" {
			content += "\n"
		}
		content += ctx
	}

	m.conv.Append(core.ChatMessage{
		Role:    core.RoleUser,
		Content: content,
	})
	return m.sendToAgent(content, nil)
}

// drainTaskNotificationsToAgent pops completed task notifications and sends them to the agent.
func (m *model) drainTaskNotificationsToAgent() tea.Cmd {
	if m.agentInput.Notifications == nil {
		return nil
	}
	items := appagent.PopReadyNotifications(m.agentInput.Notifications, true)
	if len(items) == 0 {
		return nil
	}
	return m.injectTaskNotificationContinuation(appagent.MergeNotifications(items))
}

// --- Permission bridge ---

// showPermissionPrompt shows the TUI permission approval dialog for a bridge request.
func (m *model) showPermissionPrompt(req *permBridgeRequest) tea.Cmd {
	if req == nil || req.Request == nil {
		return nil
	}
	m.approval.Show(req.Request, m.width, m.height)
	return nil
}

// sendToAgent sends a user message to the core.Agent's inbox.
func (m *model) sendToAgent(content string, images []core.Image) tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	inbox := m.agentSess.agent.Inbox()
	msg := core.Message{
		Role:    core.RoleUser,
		Content: content,
		Images:  images,
	}
	return func() tea.Msg {
		inbox <- msg
		return nil
	}
}

// continueOutbox returns the command to keep reading the agent outbox.
func (m *model) continueOutbox() tea.Cmd {
	if m.agentSess == nil || m.agentSess.agent == nil {
		return nil
	}
	return drainAgentOutbox(m.agentSess.agent.Outbox())
}

// --- Overflow persistence (ported from handler_tool.go) ---

const (
	// toolResultOverflowThreshold is the size in bytes above which tool results
	// are persisted to disk and replaced with a truncated preview.
	toolResultOverflowThreshold = 100_000 // 100KB

	// toolResultPreviewSize is the number of bytes to keep as an inline preview
	// when a tool result exceeds the overflow threshold.
	toolResultPreviewSize = 10_000 // 10KB
)

// persistToolResultOverflow checks if a tool result exceeds the overflow threshold
// and, if so, persists the full content to disk and replaces it with a truncated preview.
func (m *model) persistToolResultOverflow(result *core.ToolResult) {
	if len(result.Content) <= toolResultOverflowThreshold {
		return
	}

	cutoff := toolResultPreviewSize
	if cutoff > len(result.Content) {
		cutoff = len(result.Content)
	}
	for cutoff > 0 && !utf8.RuneStart(result.Content[cutoff]) {
		cutoff--
	}
	preview := result.Content[:cutoff]

	persisted := false
	if err := m.ensureSessionStore(); err == nil && m.session.CurrentID != "" {
		if err := m.session.Store.PersistToolResult(m.session.CurrentID, result.ToolCallID, result.Content); err == nil {
			persisted = true
		}
	}

	if persisted {
		result.Content = fmt.Sprintf("%s\n\n[Full output persisted to blobs/tool-result/%s/%s]", preview, m.session.CurrentID, result.ToolCallID)
	} else {
		result.Content = fmt.Sprintf("%s\n\n[Output truncated from %d bytes — full content not persisted]", preview, len(result.Content))
	}
}

// getHookResponseString extracts a string value from a hook response map.
func getHookResponseString(resp map[string]any, key string) string {
	if value, ok := resp[key].(string); ok {
		return value
	}
	return ""
}

// isTaskTool returns true if the tool name is a task management tool.
func isTaskTool(name string) bool {
	switch name {
	case tool.ToolTaskCreate, tool.ToolTaskGet, tool.ToolTaskUpdate, tool.ToolTaskList:
		return true
	}
	return false
}
