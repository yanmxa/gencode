package app

import (
	tea "github.com/charmbracelet/bubbletea"

	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
)

// updateStream routes LLM streaming messages.
func (m *model) updateStream(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appconv.ChunkMsg:
		c := m.handleStreamChunk(msg)
		return c, true
	}
	return nil, false
}

func (m *model) handleStreamChunk(msg appconv.ChunkMsg) tea.Cmd {
	if msg.BuildingToolName != "" {
		m.conv.Stream.BuildingTool = msg.BuildingToolName
	}

	if msg.Err != nil {
		return m.handleStreamError(msg.Err)
	}

	if msg.Done {
		return m.handleStreamDone(msg)
	}

	// Streaming text/thinking chunks: update the message content
	m.conv.AppendToLast(msg.Text, msg.Thinking)
	return tea.Batch(m.waitForChunk(), m.output.Spinner.Tick)
}

// handleStreamDone processes a completed stream.
func (m *model) handleStreamDone(msg appconv.ChunkMsg) tea.Cmd {
	m.applyCompletedStreamState(msg)
	decision := decideCompletion(msg.StopReason, msg.ToolCalls, m.maxOutputRecoveryCount, defaultMaxOutputRecovery)
	return m.handleCompletionDecision(decision)
}

func (m *model) applyCompletedStreamState(msg appconv.ChunkMsg) {
	m.conv.Stream.BuildingTool = ""
	m.conv.TurnsSinceLastTaskTool++
	if msg.Usage != nil {
		m.provider.InputTokens = msg.Usage.InputTokens
		m.provider.OutputTokens = msg.Usage.OutputTokens
		m.conv.Compact.WarningSuppressed = false
	}
	m.conv.SetLastThinkingSignature(msg.ThinkingSignature)
}

func (m *model) handleCompletionDecision(decision completionDecision) tea.Cmd {
	switch decision.Action {
	case completionRecoverMaxTokens:
		return m.recoverMaxOutputStream()
	case completionRunTools:
		return m.handleCompletionToolCalls(decision.ToolCalls)
	default:
		return m.finalizeCompletedTurn()
	}
}

func (m *model) recoverMaxOutputStream() tea.Cmd {
	m.maxOutputRecoveryCount++
	m.conv.Append(message.ChatMessage{
		Role:    message.RoleUser,
		Content: maxOutputRecoveryPrompt,
	})
	return m.startContinueStream()
}

func (m *model) handleCompletionToolCalls(toolCalls []message.ToolCall) tea.Cmd {
	m.stopCompletedStreamPhase()
	m.conv.SetLastToolCalls(toolCalls)
	commitCmds := m.commitMessages()
	if m.shouldAutoCompact() {
		m.conv.Compact.AutoContinue = true
		commitCmds = append(commitCmds, m.triggerAutoCompact())
		return tea.Batch(commitCmds...)
	}
	commitCmds = append(commitCmds, m.handleStartToolExecution(toolCalls))
	return tea.Batch(commitCmds...)
}

func (m *model) finalizeCompletedTurn() tea.Cmd {
	m.stopCompletedStreamPhase()

	commitCmds := m.commitMessages()
	m.fireIdleHooks()
	_ = m.saveSession()

	if m.shouldAutoCompact() {
		commitCmds = append(commitCmds, m.triggerAutoCompact())
	} else {
		// Generate prompt suggestion in background
		if cmd := m.startPromptSuggestion(); cmd != nil {
			commitCmds = append(commitCmds, cmd)
		}
	}

	// Drain queued user inputs first (higher priority than cron)
	if cmd := m.drainInputQueue(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	// Drain queued cron prompts now that we're idle
	if cmd := m.drainCronQueue(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
	}

	return tea.Batch(commitCmds...)
}

func (m *model) stopCompletedStreamPhase() {
	m.conv.Stream.Stop()
	m.provider.ThinkingOverride = provider.ThinkingOff
}

func (m *model) fireIdleHooks() {
	if m.hookEngine == nil {
		return
	}
	m.hookEngine.ExecuteAsync(hooks.Stop, hooks.HookInput{
		LastAssistantMessage: m.lastAssistantContent(),
		StopHookActive:       m.hookEngine.StopHookActive(),
	})
	m.hookEngine.ExecuteAsync(hooks.Notification, hooks.HookInput{
		Message:          "Claude is waiting for your input",
		NotificationType: "idle_prompt",
	})
}

// handleStreamError processes a stream error.
func (m *model) handleStreamError(err error) tea.Cmd {
	// If "prompt too long", trigger auto-compact and retry
	if shouldCompactPromptTooLong(err, len(m.conv.Messages)) {
		m.conv.RemoveEmptyLastAssistant()
		m.conv.Stream.Stop()
		m.conv.Compact.AutoContinue = true // Auto-continue after compaction
		return m.triggerAutoCompact()
	}

	m.conv.AppendErrorToLast(err)
	m.conv.Stream.Stop()
	m.provider.ThinkingOverride = provider.ThinkingOff

	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.StopFailure, hooks.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			Error:                err.Error(),
			StopHookActive:       m.hookEngine.StopHookActive(),
		})
	}

	return tea.Batch(m.commitMessages()...)
}

// startContinueStream sets up a follow-up LLM stream after tool results.
func (m *model) startContinueStream() tea.Cmd {
	return m.startConversationStream(m.buildStreamRequest(nil))
}

func (m *model) handleSpinnerTick(msg tea.Msg) tea.Cmd {
	interactiveActive := m.mode.Question.IsActive() || (m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive())
	return m.output.HandleTick(msg, m.conv.Stream.Active, m.provider.FetchingLimits, m.conv.Compact.Active, interactiveActive, m.hasRunningToolExecution())
}

// startLLMStream sets up and starts an LLM streaming request with optional extra prompt content.
// It appends an empty assistant message, sets up cancellation, and starts streaming.
func (m *model) startLLMStream(extra []string) tea.Cmd {
	m.conv.Compact.ClearResult()
	return m.startConversationStream(m.buildStreamRequest(extra))
}

func (m model) waitForChunk() tea.Cmd {
	return func() tea.Msg {
		if m.conv.Stream.Ch == nil {
			return appconv.ChunkMsg{Done: true}
		}
		chunk, ok := <-m.conv.Stream.Ch
		if !ok {
			return appconv.ChunkMsg{Done: true}
		}

		return convertChunkToMsg(chunk)
	}
}

// convertChunkToMsg converts a stream chunk to a tea message.
func convertChunkToMsg(chunk message.StreamChunk) appconv.ChunkMsg {
	switch chunk.Type {
	case message.ChunkTypeText:
		return appconv.ChunkMsg{Text: chunk.Text}
	case message.ChunkTypeThinking:
		return appconv.ChunkMsg{Thinking: chunk.Text}
	case message.ChunkTypeDone:
		return convertDoneChunk(chunk)
	case message.ChunkTypeError:
		return appconv.ChunkMsg{Err: chunk.Error}
	case message.ChunkTypeToolStart:
		return appconv.ChunkMsg{BuildingToolName: chunk.ToolName}
	default:
		return appconv.ChunkMsg{}
	}
}

func convertDoneChunk(chunk message.StreamChunk) appconv.ChunkMsg {
	if chunk.Response == nil {
		return appconv.ChunkMsg{Done: true}
	}
	return appconv.ChunkMsg{
		Done:              true,
		ToolCalls:         chunk.Response.ToolCalls,
		ThinkingSignature: chunk.Response.ThinkingSignature,
		Usage:             &chunk.Response.Usage,
		StopReason:        chunk.Response.StopReason,
	}
}
