package app

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	appplugin "github.com/yanmxa/gencode/internal/app/plugin"
	apptool "github.com/yanmxa/gencode/internal/app/tool"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/ui/progress"
)

// updateTool routes tool execution and progress messages.
func (m *model) updateTool(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case apptool.ExecResultMsg:
		c := m.handleToolResult(msg)
		return c, true
	case apptool.ExecDoneMsg:
		c := m.handleAllToolsCompleted()
		return c, true
	case progress.UpdateMsg:
		c := m.handleTaskProgress(msg)
		return c, true
	case progress.CheckTickMsg:
		c := m.handleTaskProgressTick()
		return c, true
	}
	return nil, false
}

const (
	// ToolResultOverflowThreshold is the size in bytes above which tool results
	// are persisted to disk and replaced with a truncated preview.
	ToolResultOverflowThreshold = 100_000 // 100KB

	// toolResultPreviewSize is the number of bytes to keep as an inline preview
	// when a tool result exceeds the overflow threshold.
	toolResultPreviewSize = 10_000 // 10KB
)

func (m *model) handleToolResult(msg apptool.ExecResultMsg) tea.Cmd {
	// Check if we're in parallel mode
	if m.tool.Parallel {
		return m.handleParallelToolResult(msg)
	}

	// Clear task progress for this agent when Task tool completes
	if msg.ToolName == "Task" {
		delete(m.output.TaskProgress, msg.Index)
	}

	// Execute PostToolUse or PostToolUseFailure hook asynchronously
	if m.hookEngine != nil {
		eventType := hooks.PostToolUse
		if msg.Result.IsError {
			eventType = hooks.PostToolUseFailure
		}
		input := hooks.HookInput{
			ToolName:     msg.ToolName,
			ToolResponse: msg.Result.Content,
		}
		if msg.Result.IsError {
			input.Error = msg.Result.Content
		}
		m.hookEngine.ExecuteAsync(eventType, input)
	}

	// Sequential mode - original behavior
	r := msg.Result
	m.persistToolResultOverflow(&r)
	m.conv.Append(message.ChatMessage{
		Role:       message.RoleUser,
		ToolResult: &r,
		ToolName:   msg.ToolName,
	})
	m.tool.CurrentIdx++
	commitCmds := m.commitMessages()
	nextTool := apptool.ProcessNext(m.tool.PendingCalls, m.tool.CurrentIdx, m.cwd, m.settings, m.mode.SessionPermissions)
	return tea.Batch(append(commitCmds, nextTool)...)
}

func (m *model) handleParallelToolResult(msg apptool.ExecResultMsg) tea.Cmd {
	// Store result in the parallel results map
	if m.tool.ParallelResults == nil {
		m.tool.ParallelResults = make(map[int]message.ToolResult)
	}
	m.tool.ParallelResults[msg.Index] = msg.Result
	m.tool.ParallelCount++

	// Check if all results are in
	if m.tool.ParallelCount >= len(m.tool.PendingCalls) {
		return m.completeParallelExecution()
	}

	// More results pending
	return nil
}

func (m *model) completeParallelExecution() tea.Cmd {
	for i := 0; i < len(m.tool.PendingCalls); i++ {
		tc := m.tool.PendingCalls[i]
		if result, ok := m.tool.ParallelResults[i]; ok {
			m.persistToolResultOverflow(&result)
			m.conv.Append(message.ChatMessage{
				Role:       message.RoleUser,
				ToolResult: &result,
				ToolName:   tc.Name,
			})
		}
	}

	m.output.TaskProgress = nil // clear all agent progress
	m.tool.Reset()
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.startContinueStream())
	return tea.Batch(commitCmds...)
}

func (m *model) handleStartToolExecution(toolCalls []message.ToolCall) tea.Cmd {
	m.tool.PendingCalls = m.filterToolCallsWithHooks(toolCalls)
	m.tool.CurrentIdx = 0

	if len(m.tool.PendingCalls) == 0 {
		return m.startContinueStream()
	}

	cmd := apptool.ExecuteParallel(m.tool.PendingCalls, m.cwd, m.settings, m.mode.SessionPermissions, m.mode.Enabled)

	if len(m.tool.PendingCalls) > 1 && m.canRunToolsInParallel(m.tool.PendingCalls) {
		m.tool.Parallel = true
		m.tool.ParallelResults = make(map[int]message.ToolResult)
		m.tool.ParallelCount = 0
	}

	return cmd
}

// canRunToolsInParallel checks if all tools can run without user interaction
func (m *model) canRunToolsInParallel(toolCalls []message.ToolCall) bool {
	for _, tc := range toolCalls {
		if apptool.RequiresUserInteraction(tc, m.settings, m.mode.SessionPermissions, m.mode.Enabled) {
			return false
		}
	}
	return true
}

func (m *model) handleAllToolsCompleted() tea.Cmd {
	m.tool.Reset()
	return m.startContinueStream()
}

// filterToolCallsWithHooks runs PreToolUse hooks and filters blocked tools.
func (m *model) filterToolCallsWithHooks(toolCalls []message.ToolCall) []message.ToolCall {
	allowed, blocked := m.loop.FilterToolCalls(context.Background(), toolCalls)

	// Add blocked results as chat messages
	for _, br := range blocked {
		m.conv.Append(message.ChatMessage{
			Role:     message.RoleUser,
			ToolName: br.ToolName,
			ToolResult: &message.ToolResult{
				ToolCallID: br.ToolCallID,
				Content:    br.Content,
				IsError:    br.IsError,
			},
		})
	}

	return allowed
}

func (m *model) handleTaskProgress(msg progress.UpdateMsg) tea.Cmd {
	return m.output.HandleProgress(msg)
}

func (m *model) handleTaskProgressTick() tea.Cmd {
	return m.output.HandleProgressTick(m.hasRunningTaskTools())
}

func (m *model) hasRunningTaskTools() bool {
	if m.tool.Parallel {
		return m.hasRunningParallelTaskTools()
	}
	return m.hasRunningSequentialTaskTool()
}

// hasRunningParallelTaskTools checks for unfinished Task tools in parallel mode.
func (m *model) hasRunningParallelTaskTools() bool {
	for i, tc := range m.tool.PendingCalls {
		if tc.Name == "Task" {
			if _, done := m.tool.ParallelResults[i]; !done {
				return true
			}
		}
	}
	return false
}

// hasRunningSequentialTaskTool checks if the current sequential tool is a Task.
func (m *model) hasRunningSequentialTaskTool() bool {
	if m.tool.PendingCalls == nil || m.tool.CurrentIdx >= len(m.tool.PendingCalls) {
		return false
	}
	return m.tool.PendingCalls[m.tool.CurrentIdx].Name == "Task"
}

// installPlugin creates a tea.Cmd that installs the requested plugin.
func (m model) installPlugin(msg appplugin.InstallMsg) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		installer := plugin.NewInstaller(plugin.DefaultRegistry, m.cwd)
		if err := installer.LoadMarketplaces(); err != nil {
			return appplugin.InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		pluginRef := msg.PluginName
		if msg.Marketplace != "" {
			pluginRef = msg.PluginName + "@" + msg.Marketplace
		}

		if err := installer.Install(ctx, pluginRef, msg.Scope); err != nil {
			return appplugin.InstallResultMsg{PluginName: msg.PluginName, Success: false, Error: err}
		}

		return appplugin.InstallResultMsg{PluginName: msg.PluginName, Success: true}
	}
}

// persistToolResultOverflow checks if a tool result exceeds the overflow threshold
// and, if so, persists the full content to disk and replaces it with a truncated preview.
func (m *model) persistToolResultOverflow(result *message.ToolResult) {
	if len(result.Content) <= ToolResultOverflowThreshold {
		return
	}

	if err := m.ensureSessionStore(); err != nil {
		return
	}

	if m.session.CurrentID == "" {
		return
	}

	if err := m.session.Store.PersistToolResult(m.session.CurrentID, result.ToolCallID, result.Content); err != nil {
		return
	}

	preview := result.Content[:toolResultPreviewSize]
	result.Content = fmt.Sprintf("%s\n\n[Full output persisted to tool-results/%s]", preview, result.ToolCallID)
}
