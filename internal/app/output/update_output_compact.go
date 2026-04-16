// Token limit management and conversation compaction handlers.
package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	appcompact "github.com/yanmxa/gencode/internal/app/output/compact"
	"github.com/yanmxa/gencode/internal/app/output/render"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/filecache"
	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/runtime"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// updateCompact routes compaction and token limit messages.
func (m *model) updateCompact(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appcompact.ResultMsg:
		c := m.handleCompactResult(msg)
		return c, true
	case appcompact.TokenLimitResultMsg:
		c := m.handleTokenLimitResult(msg)
		return c, true
	}
	return nil, false
}

func handleTokenLimitCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if m.provider.CurrentModel == nil {
		return "No model selected. Use /model to select a model first.", nil, nil
	}

	modelID := m.provider.CurrentModel.ModelID
	args = strings.TrimSpace(args)

	if args != "" {
		return setTokenLimits(m, modelID, args)
	}

	return showOrFetchTokenLimits(m, modelID)
}

func setTokenLimits(m *model, modelID, args string) (string, tea.Cmd, error) {
	var inputLimit, outputLimit int
	if _, err := fmt.Sscanf(args, "%d %d", &inputLimit, &outputLimit); err != nil {
		return "Usage:\n  /tokenlimit              - Show or auto-fetch limits\n  /tokenlimit <input> <output> - Set custom limits", nil, nil
	}

	if inputLimit <= 0 || outputLimit <= 0 {
		return "Token limits must be positive integers", nil, nil
	}

	if m.provider.Store != nil {
		if err := m.provider.Store.SetTokenLimit(modelID, inputLimit, outputLimit); err != nil {
			return "", nil, fmt.Errorf("failed to set token limits: %w", err)
		}
	}

	return fmt.Sprintf("Set token limits for %s:\n  Input:  %s tokens\n  Output: %s tokens",
		modelID, render.FormatTokenCount(inputLimit), render.FormatTokenCount(outputLimit)), nil, nil
}

func showOrFetchTokenLimits(m *model, modelID string) (string, tea.Cmd, error) {
	if m.provider.Store != nil {
		if customInput, customOutput, ok := m.provider.Store.GetTokenLimit(modelID); ok {
			return appcompact.FormatTokenLimitDisplay(modelID, customInput, customOutput, true, m.provider.InputTokens), nil, nil
		}
	}

	inputLimit, outputLimit := appcompact.GetModelTokenLimits(m.provider.Store, m.provider.CurrentModel)
	if inputLimit > 0 || outputLimit > 0 {
		return appcompact.FormatTokenLimitDisplay(modelID, inputLimit, outputLimit, false, m.provider.InputTokens), nil, nil
	}

	m.provider.FetchingLimits = true
	return "", tea.Batch(m.agentOutput.Spinner.Tick, fetchTokenLimitsCmd(m.buildTokenLimitFetchRequest())), nil
}

func handleCompactCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if m.provider.LLM == nil {
		return "No provider connected. Use /provider to connect.", nil, nil
	}
	if len(m.conv.Messages) == 0 {
		return "No active LLM session. Send a message first to initialize the client.", nil, nil
	}
	if !runtime.CanCompactMessages(len(m.conv.Messages)) {
		return "Not enough conversation history to compact.", nil, nil
	}
	if m.conv.Stream.Active {
		return "Cannot compact while streaming.", nil, nil
	}
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = strings.TrimSpace(args)
	m.conv.Compact.Phase = appcompact.PhaseSummarizing
	return "", tea.Batch(m.agentOutput.Spinner.Tick, compactCmd(m.buildCompactRequest(m.conv.Compact.Focus, "manual"))), nil
}

func (m *model) getEffectiveInputLimit() int {
	return appcompact.GetEffectiveInputLimit(m.provider.Store, m.provider.CurrentModel)
}

func (m *model) getMaxTokens() int {
	return appcompact.GetMaxTokens(m.provider.Store, m.provider.CurrentModel, config.DefaultMaxTokens)
}

func (m *model) getContextUsagePercent() float64 {
	return appcompact.GetContextUsagePercent(m.provider.InputTokens, m.provider.Store, m.provider.CurrentModel)
}

func (m *model) shouldAutoCompact() bool {
	return appcompact.ShouldAutoCompact(m.provider.LLM, len(m.conv.Messages), m.provider.InputTokens, m.provider.Store, m.provider.CurrentModel)
}

func (m *model) triggerAutoCompact() tea.Cmd {
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = ""
	m.conv.Compact.Phase = appcompact.PhaseSummarizing
	m.conv.AddNotice(fmt.Sprintf("\u26a1 Auto-compacting conversation (%.0f%% context used)...", m.getContextUsagePercent()))
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.agentOutput.Spinner.Tick, compactCmd(m.buildCompactRequest("", "auto")))
	return tea.Batch(commitCmds...)
}

// handleCompactResult processes the result of a compaction operation.
func (m *model) handleCompactResult(msg appcompact.ResultMsg) tea.Cmd {
	shouldContinue := m.conv.Compact.AutoContinue

	if msg.Error != nil {
		m.conv.Compact.Complete(fmt.Sprintf("Compaction could not be completed: %v", msg.Error), true)
		return tea.Batch(m.commitMessages()...)
	}

	m.conv.Compact.Complete(fmt.Sprintf("Condensed %d earlier messages.", msg.OriginalCount), false)

	// Commit all existing messages to terminal scrollback BEFORE clearing,
	// so the user can still scroll up to see their conversation history.
	scrollbackCmds := m.commitAllMessages()

	// Add a visual boundary marker to scrollback
	boundaryStyle := lipgloss.NewStyle().Foreground(kit.CurrentTheme.Muted)
	boundary := boundaryStyle.Render(fmt.Sprintf("✻ Conversation compacted — %d messages summarized (scroll up for history)", msg.OriginalCount))

	// Clear messages — the summary lives in transcript state, not in the message list.
	m.resetAfterCompact()

	// Restore recently accessed files as post-compact context
	var restoredFiles []filecache.RestoredFile
	var restoredContext string
	if m.fileCache != nil {
		restoredFiles, _ = m.fileCache.RestoreRecent()
		if len(restoredFiles) > 0 {
			restoredContext = filecache.FormatRestoredFiles(restoredFiles)
		}
	}

	// Persist the compaction summary as session memory
	if m.session.Store != nil && m.session.CurrentID != "" {
		_ = m.session.Store.SaveSessionMemory(m.session.CurrentID, msg.Summary)
	}
	m.session.Summary = msg.Summary

	// Fire PostCompact hook (fire-and-forget; no blocking semantics)
	if m.hookEngine != nil {
		m.hookEngine.ExecuteAsync(hooks.PostCompact, hooks.HookInput{
			Trigger: msg.Trigger,
		})
	}

	// Use tea.Sequence for scrollback+boundary+clear to guarantee ordering.
	// tea.Batch would run them concurrently, potentially clearing the screen
	// before scrollback commits finish.
	scrollPart := tea.Sequence(append(scrollbackCmds, tea.Println(boundary), tea.ClearScreen)...)
	cmds := []tea.Cmd{scrollPart}
	if shouldContinue {
		m.conv.Compact.ClearResult()
		if restoredContext != "" {
			m.conv.Append(core.ChatMessage{
				Role:    core.RoleUser,
				Content: restoredContext,
			})
		}
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: runtime.AutoCompactResumePrompt,
		})
		cmds = append(cmds, m.sendToAgent(runtime.AutoCompactResumePrompt, nil))
	} else if restoredContext != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: restoredContext,
		})
		m.conv.AddNotice(fmt.Sprintf("Restored %d recently accessed file(s) for context.", len(restoredFiles)))
		cmds = append(cmds, m.commitMessages()...)
	}
	return tea.Batch(cmds...)
}

// resetAfterCompact clears messages and resets token counters after compaction.
func (m *model) resetAfterCompact() {
	m.conv.Clear()
	m.provider.InputTokens = 0
	m.provider.OutputTokens = 0
}

// handleTokenLimitResult processes the result of a token limit fetch.
func (m *model) handleTokenLimitResult(msg appcompact.TokenLimitResultMsg) tea.Cmd {
	m.provider.FetchingLimits = false

	var content string
	if msg.Error != nil {
		content = "Error: " + msg.Error.Error()
	} else {
		content = msg.Result
	}
	m.conv.AddNotice(content)

	return tea.Batch(m.commitMessages()...)
}
