// Handler logic for core.Agent outbox events. Each handler function takes
// a Runtime (mutation primitives from parent) and a *Model (output-local state).
package output

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/output/progress"
	"github.com/yanmxa/gencode/internal/app/output/render"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/task/tracker"
	"github.com/yanmxa/gencode/internal/tool"
)

// --- Agent event dispatch ---

// handleAgentEvent processes a single event from the core.Agent outbox.
func handleAgentEvent(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	switch ev.Type {
	case core.OnStart, core.OnMessage:
		return rt.ContinueOutbox()

	case core.PreInfer:
		return handlePreInfer(rt, m)

	case core.OnChunk:
		return handleChunk(rt, ev)

	case core.PostInfer:
		return handlePostInfer(rt, ev)

	case core.PreTool:
		return handlePreTool(rt, ev)

	case core.PostTool:
		return handlePostTool(rt, m, ev)

	case core.OnTurn:
		return handleTurn(rt, m, ev)

	case core.OnStop:
		err, _ := ev.Error()
		return handleAgentStopped(rt, m, err)

	default:
		return rt.ContinueOutbox()
	}
}

// --- Event handlers ---

// handlePreInfer fires when the agent is about to call the LLM.
// Marks the stream as active and appends an empty assistant message
// for incremental text accumulation.
func handlePreInfer(rt Runtime, m *Model) tea.Cmd {
	rt.ActivateStream()

	// Commit pending messages (e.g. user input, tool results) to scrollback
	// before the new assistant message starts.
	commitCmds := rt.CommitMessages()

	// Append an empty assistant message that will accumulate streamed text.
	rt.AppendMessage(core.ChatMessage{Role: core.RoleAssistant, Content: ""})

	cmds := append(commitCmds, m.Spinner.Tick)
	cmds = append(cmds, rt.ContinueOutbox())
	return tea.Batch(cmds...)
}

// handleChunk processes a streaming text/thinking chunk from the LLM.
func handleChunk(rt Runtime, ev core.Event) tea.Cmd {
	chunk, ok := ev.Chunk()
	if !ok {
		return rt.ContinueOutbox()
	}
	if chunk.Text != "" || chunk.Thinking != "" {
		rt.AppendToLast(chunk.Text, chunk.Thinking)
	}
	return rt.ContinueOutbox()
}

// handlePostInfer fires after the LLM response is fully received.
// Updates token counts, thinking signature, and tool call display state.
func handlePostInfer(rt Runtime, ev core.Event) tea.Cmd {
	resp, ok := ev.Response()
	if !ok {
		return rt.ContinueOutbox()
	}

	rt.SetTokenCounts(resp.TokensIn, resp.TokensOut)
	rt.ClearWarningSuppressed()
	rt.IncrementTurnCounter()

	if resp.ThinkingSignature != "" {
		rt.SetLastThinkingSignature(resp.ThinkingSignature)
	}
	if len(resp.ToolCalls) > 0 {
		rt.SetLastToolCalls(resp.ToolCalls)
	}

	rt.SetBuildingTool("")
	return rt.ContinueOutbox()
}

// handlePreTool fires before a tool is executed. Updates the building
// tool indicator for the spinner display.
func handlePreTool(rt Runtime, ev core.Event) tea.Cmd {
	if tc, ok := ev.ToolCall(); ok {
		rt.SetBuildingTool(tc.Name)
	}
	return rt.ContinueOutbox()
}

// handlePostTool fires after a tool execution completes. Applies side effects
// (cwd changes, file cache, background task tracking) and appends the tool
// result to the conversation display.
func handlePostTool(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	tr, ok := ev.ToolResult()
	if !ok {
		return rt.ContinueOutbox()
	}

	rt.SetBuildingTool("")

	// Retrieve side effects stored by the tool adapter
	sideEffect := tool.PopSideEffect(tr.ToolCallID)
	if sideEffect != nil {
		rt.ApplyToolSideEffects(tr.ToolName, sideEffect)
	}

	// Track task tools for reminder nudges
	if isTaskTool(tr.ToolName) {
		rt.ResetTurnCounter()
	}

	// Clean up agent progress display
	if tool.IsAgentToolName(tr.ToolName) {
		m.TaskProgress = nil
	}

	// Fire PostToolUse hook asynchronously
	rt.FirePostToolHook(tr, sideEffect)

	// Persist overflow and append to conversation display
	result := &core.ToolResult{
		ToolCallID: tr.ToolCallID,
		ToolName:   tr.ToolName,
		Content:    tr.Content,
		IsError:    tr.IsError,
	}
	rt.PersistOverflow(result)
	rt.AppendMessage(core.ChatMessage{
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
func handleTurn(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	result, _ := ev.Result()

	rt.StopStream()
	rt.ClearThinkingOverride()

	commitCmds := rt.CommitMessages()

	// Fire idle hooks (Stop + Notification)
	if rt.FireIdleHooks() {
		// Stop hook blocked — send continuation to agent
		cmds := append(commitCmds, rt.ContinueOutbox())
		return tea.Batch(cmds...)
	}

	rt.SaveSession()

	// Check auto-compact
	if rt.ShouldAutoCompact() {
		rt.SetAutoCompactContinue()
		cmds := append(commitCmds, rt.TriggerAutoCompact())
		return tea.Batch(cmds...)
	}

	// Try prompt suggestion if idle
	if cmd := rt.StartPromptSuggestion(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
	}

	// Drain queued Source 1/2/3 items through the runtime coordinator.
	if cmd := rt.DrainTurnQueues(); cmd != nil {
		commitCmds = append(commitCmds, cmd)
		return tea.Batch(commitCmds...)
	}

	// Check for stop reason details (max_turns etc.)
	if result.StopReason != "" && result.StopReason != core.StopEndTurn {
		rt.AddNotice(fmt.Sprintf("Agent stopped: %s", result.StopReason))
		if result.StopDetail != "" {
			rt.AddNotice(result.StopDetail)
		}
	}

	// Continue draining outbox (agent goes back to waitForInput)
	cmds := append(commitCmds, rt.ContinueOutbox())
	return tea.Batch(cmds...)
}

// handleAgentStopped processes agent shutdown.
func handleAgentStopped(rt Runtime, m *Model, err error) tea.Cmd {
	rt.StopStream()

	if err != nil {
		rt.AddNotice(fmt.Sprintf("Agent error: %v", err))
		rt.FireStopFailureHook(err)
	}

	commitCmds := rt.CommitMessages()
	rt.StopAgentSession()
	return tea.Batch(commitCmds...)
}

// --- Utilities ---

// isTaskTool returns true if the tool name is a task management tool.
func isTaskTool(name string) bool {
	switch name {
	case tool.ToolTaskCreate, tool.ToolTaskGet, tool.ToolTaskUpdate, tool.ToolTaskList:
		return true
	}
	return false
}

// --- Progress handling (operates on output Model directly) ---

// drainProgress drains all pending task progress messages from the channel.
func (m *Model) drainProgress() {
	if m.ProgressHub == nil {
		return
	}
	m.TaskProgress = m.ProgressHub.Drain(m.TaskProgress)
}

// HandleProgress processes a task progress message.
func (m *Model) HandleProgress(msg progress.UpdateMsg) tea.Cmd {
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
func (m *Model) HandleProgressTick(hasRunningTasks bool) tea.Cmd {
	if hasRunningTasks {
		if m.ProgressHub == nil {
			return m.Spinner.Tick
		}
		return tea.Batch(m.Spinner.Tick, m.ProgressHub.Check())
	}
	return nil
}

// HandleTick handles spinner tick messages with context-aware updates.
func (m *Model) HandleTick(msg tea.Msg, active, fetching, compacting, interactiveActive, hasRunningTasks bool) tea.Cmd {
	// Handle token limit fetching spinner
	if fetching {
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return cmd
	}

	if !active && !hasRunningTasks {
		// Keep spinner alive when tasks are in-progress (e.g., background agents)
		if !tracker.DefaultStore.HasInProgress() {
			return nil
		}
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
func (m *Model) ResizeMDRenderer(width int) {
	m.MDRenderer = render.NewMDRenderer(width)
}
