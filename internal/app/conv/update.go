// Handler logic for core.Agent outbox events.
package conv

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/log"
	"github.com/yanmxa/gencode/internal/tool"
)

// Update routes all output-path messages: agent outbox, permission bridge,
// compaction results, and progress updates.
func Update(rt Runtime, m *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentOutboxMsg:
		if msg.Closed && len(msg.Batch) == 0 {
			m.Stream.Stop()
			return rt.ProcessAgentStop(nil), true
		}
		if len(msg.Batch) > 0 {
			return handleAgentEventBatch(rt, m, msg.Batch, msg.Closed), true
		}
		return handleAgentEvent(rt, m, msg.Event), true
	case PermBridgeMsg:
		return rt.HandlePermBridge(msg.Request), true
	case CompactResultMsg:
		return rt.HandleCompactResult(msg), true
	case kit.TokenLimitResultMsg:
		return rt.HandleTokenLimitResult(msg), true
	case ProgressUpdateMsg:
		return m.HandleProgress(msg), true
	case ProgressCheckTickMsg:
		return m.HandleProgressTick(rt.HasRunningTasks()), true
	default:
		return nil, false
	}
}

// --- Agent event dispatch ---

func handleAgentEvent(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	log.QueueLog("handleAgentEvent: %s", ev.Type)
	switch ev.Type {
	case core.OnTurn:
		result, _ := ev.Result()
		m.Stream.Stop()
		return rt.ProcessTurnEnd(result)
	case core.OnStop:
		err, _ := ev.Error()
		m.Stream.Stop()
		return rt.ProcessAgentStop(err)
	case core.OnCompact:
		info, _ := ev.CompactInfo()
		return rt.HandleAgentCompact(info)
	default:
		if extra := applyAgentEvent(rt, m, ev); extra != nil {
			return tea.Batch(extra, rt.ContinueOutbox())
		}
		return rt.ContinueOutbox()
	}
}

func handleAgentEventBatch(rt Runtime, m *Model, events []core.Event, closed bool) tea.Cmd {
	var cmds []tea.Cmd
	needsContinue := true

	for _, ev := range events {
		log.QueueLog("handleAgentEventBatch: %s", ev.Type)
		switch ev.Type {
		case core.OnTurn:
			result, _ := ev.Result()
			m.Stream.Stop()
			cmds = append(cmds, rt.ProcessTurnEnd(result))
			needsContinue = false
		case core.OnStop:
			err, _ := ev.Error()
			m.Stream.Stop()
			cmds = append(cmds, rt.ProcessAgentStop(err))
			needsContinue = false
		case core.OnCompact:
			info, _ := ev.CompactInfo()
			cmds = append(cmds, rt.HandleAgentCompact(info))
			needsContinue = false
		default:
			if extra := applyAgentEvent(rt, m, ev); extra != nil {
				cmds = append(cmds, extra)
			}
			continue
		}
		break // terminal event — don't process further events in this batch
	}

	if closed {
		m.Stream.Stop()
		cmds = append(cmds, rt.ProcessAgentStop(nil))
		needsContinue = false
	}

	if needsContinue {
		cmds = append(cmds, rt.ContinueOutbox())
	}

	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

// --- Event side-effect handlers (no ContinueOutbox) ---

func applyAgentEvent(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	switch ev.Type {
	case core.OnStart, core.OnMessage:
		return nil
	case core.PreInfer:
		return applyPreInfer(rt, m)
	case core.OnChunk:
		return applyChunk(rt, m, ev)
	case core.PostInfer:
		return applyPostInfer(rt, m, ev)
	case core.PreTool:
		applyPreTool(m, ev)
		return nil
	case core.PostTool:
		return applyPostTool(rt, m, ev)
	default:
		return nil
	}
}

func applyPreInfer(rt Runtime, m *Model) tea.Cmd {
	m.Stream.Active = true
	m.Stream.BuildingTool = ""
	commitCmds := rt.CommitMessages()
	m.Append(core.ChatMessage{Role: core.RoleAssistant, Content: ""})
	cmds := append(commitCmds, m.Spinner.Tick)
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

func applyChunk(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	chunk, ok := ev.Chunk()
	if !ok {
		return nil
	}
	if chunk.Text != "" || chunk.Thinking != "" {
		m.AppendToLast(chunk.Text, chunk.Thinking)
	}
	if chunk.Done && chunk.Response != nil && len(chunk.Response.ToolCalls) == 0 {
		m.Stream.Active = false
		commitCmds := rt.CommitMessages()
		if len(commitCmds) > 0 {
			return tea.Batch(commitCmds...)
		}
	}
	return nil
}

func applyPostInfer(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	resp, ok := ev.Response()
	if !ok {
		return nil
	}
	rt.SetTokenCounts(resp.TokensIn, resp.TokensOut)
	m.Compact.WarningSuppressed = false
	if resp.ThinkingSignature != "" {
		m.SetLastThinkingSignature(resp.ThinkingSignature)
	}
	if len(resp.ToolCalls) > 0 {
		m.SetLastToolCalls(resp.ToolCalls)
	}
	m.Stream.BuildingTool = ""
	return nil
}

func applyPreTool(m *Model, ev core.Event) {
	if tc, ok := ev.ToolCall(); ok {
		m.Stream.BuildingTool = tc.Name
	}
}

func applyPostTool(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	tr, ok := ev.ToolResult()
	if !ok {
		return nil
	}
	m.Stream.BuildingTool = ""
	if tool.IsAgentToolName(tr.ToolName) {
		m.TaskProgress = nil
	}
	result := rt.ProcessToolResult(tr)
	m.Append(core.ChatMessage{
		Role:       core.RoleUser,
		ToolName:   tr.ToolName,
		ToolResult: result,
	})
	return nil
}

// --- Progress handling (operates on output Model directly) ---

func (m *OutputModel) drainProgress() {
	if m.ProgressHub == nil {
		return
	}
	m.TaskProgress = m.ProgressHub.Drain(m.TaskProgress)
}

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

func (m *OutputModel) HandleProgressTick(hasRunningTasks bool) tea.Cmd {
	if m.ProgressHub != nil {
		if hasRunningTasks {
			return tea.Batch(m.Spinner.Tick, m.ProgressHub.Check())
		}
		return m.ProgressHub.Check()
	}
	if hasRunningTasks {
		return m.Spinner.Tick
	}
	return nil
}

