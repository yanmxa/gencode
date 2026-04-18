// Handler logic for core.Agent outbox events. Each handler function takes
// a Runtime (mutation primitives from parent) and a *OutputModel (output-local state).
package conv

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool"
)

// --- Agent event dispatch ---

// handleAgentEvent processes a single event from the core.Agent outbox.
func handleAgentEvent(rt Runtime, m *OutputModel, cm *ConversationModel, ev core.Event) tea.Cmd {
	switch ev.Type {
	case core.OnStart, core.OnMessage:
		return rt.ContinueOutbox()

	case core.PreInfer:
		return handlePreInfer(rt, m, cm)

	case core.OnChunk:
		return handleChunk(rt, cm, ev)

	case core.PostInfer:
		return handlePostInfer(rt, cm, ev)

	case core.PreTool:
		return handlePreTool(rt, cm, ev)

	case core.PostTool:
		return handlePostTool(rt, m, cm, ev)

	case core.OnTurn:
		return handleTurn(rt, m, cm, ev)

	case core.OnStop:
		err, _ := ev.Error()
		return handleAgentStopped(rt, m, cm, err)

	default:
		return rt.ContinueOutbox()
	}
}

// --- Event handlers ---

// handlePreInfer fires when the agent is about to call the LLM.
// Marks the stream as active and appends an empty assistant message
// for incremental text accumulation.
func handlePreInfer(rt Runtime, m *OutputModel, cm *ConversationModel) tea.Cmd {
	cm.Stream.Active = true
	cm.Stream.BuildingTool = ""

	commitCmds := rt.CommitMessages()
	cm.Append(core.ChatMessage{Role: core.RoleAssistant, Content: ""})

	cmds := append(commitCmds, m.Spinner.Tick)
	cmds = append(cmds, rt.ContinueOutbox())
	return tea.Batch(cmds...)
}

// handleChunk processes a streaming text/thinking chunk from the LLM.
func handleChunk(rt Runtime, cm *ConversationModel, ev core.Event) tea.Cmd {
	chunk, ok := ev.Chunk()
	if !ok {
		return rt.ContinueOutbox()
	}
	if chunk.Text != "" || chunk.Thinking != "" {
		cm.AppendToLast(chunk.Text, chunk.Thinking)
	}
	return rt.ContinueOutbox()
}

// handlePostInfer fires after the LLM response is fully received.
// Updates token counts, thinking signature, and tool call display state.
func handlePostInfer(rt Runtime, cm *ConversationModel, ev core.Event) tea.Cmd {
	resp, ok := ev.Response()
	if !ok {
		return rt.ContinueOutbox()
	}

	rt.SetTokenCounts(resp.TokensIn, resp.TokensOut)
	cm.Compact.WarningSuppressed = false

	if resp.ThinkingSignature != "" {
		cm.SetLastThinkingSignature(resp.ThinkingSignature)
	}
	if len(resp.ToolCalls) > 0 {
		cm.SetLastToolCalls(resp.ToolCalls)
	}

	cm.Stream.BuildingTool = ""
	return rt.ContinueOutbox()
}

// handlePreTool fires before a tool is executed. Updates the building
// tool indicator for the spinner display.
func handlePreTool(rt Runtime, cm *ConversationModel, ev core.Event) tea.Cmd {
	if tc, ok := ev.ToolCall(); ok {
		cm.Stream.BuildingTool = tc.Name
	}
	return rt.ContinueOutbox()
}

// handlePostTool fires after a tool execution completes. Applies side effects
// (cwd changes, file cache, background task tracking) and appends the tool
// result to the conversation display.
func handlePostTool(rt Runtime, m *OutputModel, cm *ConversationModel, ev core.Event) tea.Cmd {
	tr, ok := ev.ToolResult()
	if !ok {
		return rt.ContinueOutbox()
	}

	cm.Stream.BuildingTool = ""

	sideEffect := rt.PopToolSideEffect(tr.ToolCallID)
	if sideEffect != nil {
		rt.ApplyToolSideEffects(tr.ToolName, sideEffect)
	}

	if tool.IsAgentToolName(tr.ToolName) {
		m.TaskProgress = nil
	}

	rt.FirePostToolHook(tr, sideEffect)

	result := &core.ToolResult{
		ToolCallID: tr.ToolCallID,
		ToolName:   tr.ToolName,
		Content:    tr.Content,
		IsError:    tr.IsError,
	}
	rt.PersistOverflow(result)
	cm.Append(core.ChatMessage{
		Role:     core.RoleUser,
		ToolName: tr.ToolName,
		ToolResult: &core.ToolResult{
			ToolCallID: tr.ToolCallID,
			ToolName:   tr.ToolName,
			Content:    result.Content,
			IsError:    tr.IsError,
		},
	})

	return rt.ContinueOutbox()
}

// handleTurn fires when the agent completes a think+act cycle (end_turn).
// This is the idle point — save session, fire hooks, check compaction,
// drain queued inputs.
func handleTurn(rt Runtime, m *OutputModel, cm *ConversationModel, ev core.Event) tea.Cmd {
	result, _ := ev.Result()

	cm.Stream.Stop()
	rt.ClearThinkingOverride()

	commitCmds := rt.CommitMessages()

	if rt.FireIdleHooks() {
		cmds := append(commitCmds, rt.ContinueOutbox())
		return tea.Batch(cmds...)
	}

	rt.SaveSession()

	if rt.ShouldAutoCompact() {
		cm.Compact.AutoContinue = true
		cmds := append(commitCmds, rt.TriggerAutoCompact())
		return tea.Batch(cmds...)
	}

	if cmd := rt.StartPromptSuggestion(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
	}

	if cmd := rt.DrainTurnQueues(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	if result.StopReason != "" && result.StopReason != core.StopEndTurn {
		cm.AddNotice(fmt.Sprintf("Agent stopped: %s", result.StopReason))
		if result.StopDetail != "" {
			cm.AddNotice(result.StopDetail)
		}
	}

	cmds := append(commitCmds, rt.ContinueOutbox())
	return tea.Batch(cmds...)
}

// handleAgentStopped processes agent shutdown.
func handleAgentStopped(rt Runtime, m *OutputModel, cm *ConversationModel, err error) tea.Cmd {
	cm.Stream.Stop()

	if err != nil {
		cm.AddNotice(fmt.Sprintf("Agent error: %v", err))
		rt.FireStopFailureHook(err)
	}

	commitCmds := rt.CommitMessages()
	rt.StopAgentSession()
	return tea.Batch(commitCmds...)
}

// --- Progress handling (operates on output Model directly) ---

// drainProgress drains all pending task progress messages from the channel.
func (m *OutputModel) drainProgress() {
	if m.ProgressHub == nil {
		return
	}
	m.TaskProgress = m.ProgressHub.Drain(m.TaskProgress)
}

// HandleProgress processes a task progress message.
func (m *OutputModel) HandleProgress(msg ProgressUpdateMsg) tea.Cmd {
	if m.TaskProgress == nil {
		m.TaskProgress = make(map[int][]string)
	}
	m.TaskProgress[msg.Index] = append(m.TaskProgress[msg.Index], msg.Message)
	// Cap progress entries per agent to prevent unbounded growth
	if len(m.TaskProgress[msg.Index]) > 5 {
		m.TaskProgress[msg.Index] = m.TaskProgress[msg.Index][len(m.TaskProgress[msg.Index])-5:]
	}

	if m.ProgressHub == nil {
		return m.Spinner.Tick
	}
	return tea.Batch(m.Spinner.Tick, m.ProgressHub.Check())
}

// HandleProgressTick processes a progress tick when tasks may be running.
func (m *OutputModel) HandleProgressTick(hasRunningTasks bool) tea.Cmd {
	if hasRunningTasks {
		if m.ProgressHub == nil {
			return m.Spinner.Tick
		}
		return tea.Batch(m.Spinner.Tick, m.ProgressHub.Check())
	}
	return nil
}

// HandleTick handles spinner tick messages with context-aware updates.
func (m *OutputModel) HandleTick(msg tea.Msg, active, fetching, compacting, interactiveActive, hasRunningTasks bool) tea.Cmd {
	// Handle token limit fetching spinner
	if fetching {
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return cmd
	}

	if !active && !hasRunningTasks {
		return nil
	}

	var cmd tea.Cmd
	m.Spinner, cmd = m.Spinner.Update(msg)

	if interactiveActive {
		return cmd
	}

	// Check for Task progress updates (drains all pending messages)
	if hasRunningTasks {
		m.drainProgress()
	}

	return cmd
}

// ResizeMDRenderer recreates the markdown renderer for the given width.
func (m *OutputModel) ResizeMDRenderer(width int) {
	m.MDRenderer = NewMDRenderer(width)
}
