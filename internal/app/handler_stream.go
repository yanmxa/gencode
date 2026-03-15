package app

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appconv "github.com/yanmxa/gencode/internal/app/conversation"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
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

	if len(msg.ToolCalls) > 0 {
		m.conv.SetLastToolCalls(msg.ToolCalls)
		commitCmds := m.commitMessages()

		if m.shouldAutoCompact() {
			m.conv.Compact.AutoContinue = true // Auto-continue after compaction
			commitCmds = append(commitCmds, m.triggerAutoCompact())
			return tea.Batch(commitCmds...)
		}

		commitCmds = append(commitCmds, m.handleStartToolExecution(msg.ToolCalls))
		return tea.Batch(commitCmds...)
	}

	m.conv.Stream.Stop()

	commitCmds := m.commitMessages()

	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.Stop, hooks.HookInput{})
	}

	_ = m.saveSession()

	if m.shouldAutoCompact() {
		commitCmds = append(commitCmds, m.triggerAutoCompact())
	}

	return tea.Batch(commitCmds...)
}

// handleStreamError processes a stream error.
func (m *model) handleStreamError(err error) tea.Cmd {
	// If "prompt too long", trigger auto-compact and retry
	if strings.Contains(err.Error(), "prompt is too long") && len(m.conv.Messages) >= 3 {
		m.conv.RemoveEmptyLastAssistant()
		m.conv.Stream.Stop()
		m.conv.Compact.AutoContinue = true // Auto-continue after compaction
		return m.triggerAutoCompact()
	}

	m.conv.AppendErrorToLast(err)
	m.conv.Stream.Stop()
	return tea.Batch(m.commitMessages()...)
}

// startContinueStream sets up a follow-up LLM stream after tool results.
func (m *model) startContinueStream() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.conv.Stream.Cancel = cancel

	// Configure loop and set messages BEFORE appending the empty assistant placeholder,
	// so the placeholder is not included in the API request.
	m.configureLoop(nil)
	m.loop.SetMessages(m.conv.ConvertToProvider())

	// Commit any pending messages before starting new stream
	commitCmds := m.commitMessages()

	m.conv.Append(message.ChatMessage{Role: message.RoleAssistant, Content: ""})

	m.conv.Stream.Ch = m.loop.Stream(ctx)
	allCmds := append(commitCmds, m.waitForChunk(), m.output.Spinner.Tick)
	return tea.Batch(allCmds...)
}

func (m *model) handleSpinnerTick(msg tea.Msg) tea.Cmd {
	interactiveActive := m.mode.Question.IsActive() || (m.mode.PlanApproval != nil && m.mode.PlanApproval.IsActive())
	return m.output.HandleTick(msg, m.conv.Stream.Active, m.provider.FetchingLimits, m.conv.Compact.Active, interactiveActive, m.hasRunningTaskTools())
}

// startLLMStream sets up and starts an LLM streaming request with optional extra prompt content.
// It appends an empty assistant message, sets up cancellation, and starts streaming.
func (m *model) startLLMStream(extra []string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.conv.Stream.Cancel = cancel
	m.conv.Stream.Active = true

	// Configure loop with current state and set messages
	m.configureLoop(extra)
	m.loop.SetMessages(m.conv.ConvertToProvider())

	// Commit any pending messages before starting stream
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

// convertDoneChunk handles the done chunk type.
func convertDoneChunk(chunk message.StreamChunk) appconv.ChunkMsg {
	var usage *message.Usage
	if chunk.Response != nil {
		usage = &chunk.Response.Usage
	}

	if chunk.Response != nil && len(chunk.Response.ToolCalls) > 0 {
		return appconv.ChunkMsg{
			Done:      true,
			ToolCalls: chunk.Response.ToolCalls,
			Usage:     usage,
		}
	}

	return appconv.ChunkMsg{Done: true, Usage: usage}
}
