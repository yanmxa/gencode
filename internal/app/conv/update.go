// Handler logic for core.Agent outbox events.
package conv

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/tool"
)

// Update routes all output-path messages: agent outbox, permission bridge,
// compaction results, and progress updates.
func Update(rt Runtime, m *Model, msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case AgentOutboxMsg:
		if msg.Closed {
			m.Stream.Stop()
			return rt.ProcessAgentStop(nil), true
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
	switch ev.Type {
	case core.OnStart, core.OnMessage:
		return rt.ContinueOutbox()
	case core.PreInfer:
		return handlePreInfer(rt, m)
	case core.OnChunk:
		return handleChunk(rt, m, ev)
	case core.PostInfer:
		return handlePostInfer(rt, m, ev)
	case core.PreTool:
		return handlePreTool(rt, m, ev)
	case core.PostTool:
		return handlePostTool(rt, m, ev)
	case core.OnTurn:
		result, _ := ev.Result()
		m.Stream.Stop()
		return rt.ProcessTurnEnd(result)
	case core.OnStop:
		err, _ := ev.Error()
		m.Stream.Stop()
		return rt.ProcessAgentStop(err)
	default:
		return rt.ContinueOutbox()
	}
}

// --- Event handlers ---

func handlePreInfer(rt Runtime, m *Model) tea.Cmd {
	m.Stream.Active = true
	m.Stream.BuildingTool = ""
	commitCmds := rt.CommitMessages()
	m.Append(core.ChatMessage{Role: core.RoleAssistant, Content: ""})
	cmds := append(commitCmds, m.Spinner.Tick, rt.ContinueOutbox())
	return tea.Batch(cmds...)
}

func handleChunk(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	chunk, ok := ev.Chunk()
	if !ok {
		return rt.ContinueOutbox()
	}
	if chunk.Text != "" || chunk.Thinking != "" {
		m.AppendToLast(chunk.Text, chunk.Thinking)
	}
	if chunk.Done && chunk.Response != nil && len(chunk.Response.ToolCalls) == 0 {
		m.Stream.Active = false
	}
	return rt.ContinueOutbox()
}

func handlePostInfer(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	resp, ok := ev.Response()
	if !ok {
		return rt.ContinueOutbox()
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
	return rt.ContinueOutbox()
}

func handlePreTool(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	if tc, ok := ev.ToolCall(); ok {
		m.Stream.BuildingTool = tc.Name
	}
	return rt.ContinueOutbox()
}

func handlePostTool(rt Runtime, m *Model, ev core.Event) tea.Cmd {
	tr, ok := ev.ToolResult()
	if !ok {
		return rt.ContinueOutbox()
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
	return rt.ContinueOutbox()
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
	if hasRunningTasks {
		if m.ProgressHub == nil {
			return m.Spinner.Tick
		}
		return tea.Batch(m.Spinner.Tick, m.ProgressHub.Check())
	}
	return nil
}

