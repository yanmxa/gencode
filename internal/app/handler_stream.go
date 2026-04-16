package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/provider"
	"github.com/yanmxa/gencode/internal/runtime"
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
	// Discard stale chunks from a cancelled stream — the waitForChunk closure
	// captured the old channel and may still deliver a final Done/Error after
	// handleStreamCancel already called Stream.Stop().
	if !m.conv.Stream.Active {
		return nil
	}

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
	decision := runtime.DecideCompletion(msg.StopReason, msg.ToolCalls, m.maxOutputRecoveryCount, runtime.DefaultMaxOutputRecovery)
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

func (m *model) handleCompletionDecision(decision runtime.CompletionDecision) tea.Cmd {
	switch decision.Action {
	case runtime.CompletionRecoverMaxTokens:
		return m.recoverMaxOutputStream()
	case runtime.CompletionRunTools:
		return m.handleCompletionToolCalls(decision.ToolCalls)
	default:
		return m.finalizeCompletedTurn()
	}
}

func (m *model) recoverMaxOutputStream() tea.Cmd {
	m.maxOutputRecoveryCount++
	m.conv.Append(message.ChatMessage{
		Role:    message.RoleUser,
		Content: runtime.MaxOutputRecoveryPrompt,
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
	if m.fireIdleHooks() {
		// Stop hook blocked — continue the conversation instead of idling
		commitCmds = append(commitCmds, m.startContinueStream())
		return tea.Batch(commitCmds...)
	}
	if err := m.saveSession(); err != nil {
		log.Logger().Warn("failed to save session", zap.Error(err))
	}

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

// fireIdleHooks fires Stop hooks synchronously (honoring block decisions)
// and Notification hooks asynchronously. Returns true if a Stop hook blocked.
func (m *model) fireIdleHooks() bool {
	if m.hookEngine == nil {
		return false
	}

	blocked := false
	if m.hookEngine.HasHooks(core.Stop) {
		outcome := m.hookEngine.Execute(context.Background(), core.Stop, hooks.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
			StopHookActive:       m.hookEngine.StopHookActive(),
		})
		if outcome.ShouldBlock {
			// Inject block reason as context and continue the conversation
			m.conv.Append(message.ChatMessage{
				Role:    message.RoleUser,
				Content: "Stop hook blocked: " + outcome.BlockReason,
			})
			blocked = true
		}
	}

	m.hookEngine.ExecuteAsync(core.Notification, hooks.HookInput{
		Message:          "Claude is waiting for your input",
		NotificationType: "idle_prompt",
	})
	return blocked
}

// handleStreamError processes a stream error.
func (m *model) handleStreamError(err error) tea.Cmd {
	// If "prompt too long", trigger auto-compact and retry
	if runtime.ShouldCompactPromptTooLong(err, len(m.conv.Messages)) {
		m.conv.RemoveEmptyLastAssistant()
		m.conv.Stream.Stop()
		m.conv.Compact.AutoContinue = true // Auto-continue after compaction
		return m.triggerAutoCompact()
	}

	m.conv.AppendErrorToLast(err)
	m.conv.Stream.Stop()
	m.provider.ThinkingOverride = provider.ThinkingOff

	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(core.StopFailure, hooks.HookInput{
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
	return m.output.HandleTick(msg, m.conv.Stream.Active, m.provider.FetchingLimits, m.conv.Compact.Active, interactiveActive, m.hasInFlightToolExecution())
}

// startLLMStream sets up and starts an LLM streaming request with optional extra prompt content.
// It appends an empty assistant message, sets up cancellation, and starts streaming.
func (m *model) startLLMStream(extra []string) tea.Cmd {
	m.conv.Compact.ClearResult()
	return m.startConversationStream(m.buildStreamRequest(extra))
}

func (m *model) waitForChunk() tea.Cmd {
	ch := m.conv.Stream.Ch
	return func() tea.Msg {
		if ch == nil {
			return appconv.ChunkMsg{Done: true}
		}
		chunk, ok := <-ch
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
