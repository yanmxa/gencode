package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	"github.com/yanmxa/gencode/internal/core"
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
	m.conv.Stream.BuildingTool = ""

	// Increment turns-since-last-task-tool counter for nudge system
	m.conv.TurnsSinceLastTaskTool++

	if msg.Usage != nil {
		m.provider.InputTokens = msg.Usage.InputTokens
		m.provider.OutputTokens = msg.Usage.OutputTokens
	}

	m.conv.SetLastThinkingSignature(msg.ThinkingSignature)

	decision := core.DecideCompletion(msg.StopReason, msg.ToolCalls, m.maxOutputRecoveryCount, core.DefaultMaxOutputRecovery)

	// Max-output-tokens recovery: if output was truncated with no tool calls,
	// inject a resume message and continue streaming (up to 3 retries).
	if decision.Action == core.CompletionRecoverMaxTokens {
		m.maxOutputRecoveryCount++
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleUser,
			Content: core.MaxOutputRecoveryPrompt,
		})
		return m.startStream(nil, false)
	}

	if decision.Action == core.CompletionRunTools {
		m.conv.SetLastToolCalls(decision.ToolCalls)
		commitCmds := m.commitMessages()

		if m.shouldAutoCompact() {
			m.conv.Compact.AutoContinue = true // Auto-continue after compaction
			commitCmds = append(commitCmds, m.triggerAutoCompact())
			return tea.Batch(commitCmds...)
		}

		commitCmds = append(commitCmds, m.handleStartToolExecution(decision.ToolCalls))
		return tea.Batch(commitCmds...)
	}

	m.conv.Stream.Stop()

	// Reset per-turn thinking override
	m.provider.ThinkingOverride = provider.ThinkingOff

	commitCmds := m.commitMessages()

	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.Stop, hooks.HookInput{
			LastAssistantMessage: m.lastAssistantContent(),
		})
		// Fire idle notification
		m.hookEngine.ExecuteAsync(hooks.Notification, hooks.HookInput{
			Message:          "Claude is waiting for your input",
			NotificationType: "idle_prompt",
		})
	}

	_ = m.saveSession()

	if m.shouldAutoCompact() {
		commitCmds = append(commitCmds, m.triggerAutoCompact())
	} else {
		// Generate prompt suggestion in background
		if cmd := m.startPromptSuggestion(); cmd != nil {
			commitCmds = append(commitCmds, cmd)
		}
	}

	// Drain queued cron prompts now that we're idle
	if cmd := m.drainCronQueue(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
	}

	return tea.Batch(commitCmds...)
}

// handleStreamError processes a stream error.
func (m *model) handleStreamError(err error) tea.Cmd {
	// If "prompt too long", trigger auto-compact and retry
	if core.IsPromptTooLong(err) && len(m.conv.Messages) >= 3 {
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
		})
	}

	return tea.Batch(m.commitMessages()...)
}

func (m *model) handleSpinnerTick(msg tea.Msg) tea.Cmd {
	interactiveActive := m.mode.Question.IsActive() || (m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive())
	return m.output.HandleTick(msg, m.conv.Stream.Active, m.provider.FetchingLimits, m.conv.Compact.Active, interactiveActive, m.hasRunningTaskTools())
}

// startStream starts (or continues) an LLM stream.
// extra is injected into the system prompt for this turn only.
// setActive should be true for new user-initiated turns, false for follow-up streams after tools.
func (m *model) startStream(extra []string, setActive bool) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.conv.Stream.Cancel = cancel
	if setActive {
		m.conv.Stream.Active = true
	}
	m.configureLoop(extra)
	m.loop.SetMessages(m.conv.ConvertToProvider())
	commitCmds := m.commitMessages()
	m.conv.Append(message.ChatMessage{Role: message.RoleAssistant, Content: ""})
	m.conv.Stream.Ch = m.loop.Stream(ctx)
	allCmds := append(commitCmds, m.waitForChunk(), m.output.Spinner.Tick)
	return tea.Batch(allCmds...)
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
