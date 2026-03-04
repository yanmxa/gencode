// Token limit management and conversation compaction handlers.
package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	appcompact "github.com/yanmxa/gencode/internal/app/compact"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/options"
)

// updateCompact routes compaction and token limit messages.
func (m *model) updateCompact(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case appcompact.CompactResultMsg:
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
		modelID, appcompact.FormatTokenCount(inputLimit), appcompact.FormatTokenCount(outputLimit)), nil, nil
}

func showOrFetchTokenLimits(m *model, modelID string) (string, tea.Cmd, error) {
	inputLimit, outputLimit := appcompact.GetModelTokenLimits(m.provider.Store, m.provider.CurrentModel)
	if inputLimit > 0 || outputLimit > 0 {
		if m.provider.Store != nil {
			if customInput, customOutput, ok := m.provider.Store.GetTokenLimit(modelID); ok {
				return appcompact.FormatTokenLimitDisplay(modelID, customInput, customOutput, true, m.provider.InputTokens), nil, nil
			}
		}
		return appcompact.FormatTokenLimitDisplay(modelID, inputLimit, outputLimit, false, m.provider.InputTokens), nil, nil
	}

	m.provider.FetchingLimits = true
	return "", tea.Batch(m.output.Spinner.Tick, startTokenLimitFetch(m)), nil
}

func startTokenLimitFetch(m *model) tea.Cmd {
	deps := appcompact.AutoFetchTokenLimitsDeps{
		LLM:          m.provider.LLM,
		Store:        m.provider.Store,
		CurrentModel: m.provider.CurrentModel,
		ModelID:      m.getModelID(),
		Cwd:          m.cwd,
	}
	return func() tea.Msg {
		ctx := context.Background()
		result, err := appcompact.AutoFetchTokenLimits(ctx, deps)
		return appcompact.TokenLimitResultMsg{Result: result, Error: err}
	}
}

func handleCompactCommand(ctx context.Context, m *model, args string) (string, tea.Cmd, error) {
	if m.provider.LLM == nil {
		return "No provider connected. Use /provider to connect.", nil, nil
	}
	if len(m.conv.Messages) < 3 {
		return "Not enough conversation history to compact.", nil, nil
	}
	if m.conv.Stream.Active {
		return "Cannot compact while streaming.", nil, nil
	}
	m.conv.Compact.Active = true
	m.conv.Compact.Focus = strings.TrimSpace(args)
	return "", tea.Batch(m.output.Spinner.Tick, startCompact(m)), nil
}

func startCompact(m *model) tea.Cmd {
	focus := m.conv.Compact.Focus
	client := m.loop.Client
	msgs := m.conv.ConvertToProvider()
	sessionMemory := m.session.Memory
	return func() tea.Msg {
		ctx := context.Background()
		summary, count, err := appcompact.CompactConversation(ctx, client, msgs, sessionMemory, focus)
		return appcompact.CompactResultMsg{Summary: summary, OriginalCount: count, Error: err}
	}
}

func (m *model) getEffectiveInputLimit() int {
	return appcompact.GetEffectiveInputLimit(m.provider.Store, m.provider.CurrentModel)
}

func (m *model) getMaxTokens() int {
	return appcompact.GetMaxTokens(m.provider.Store, m.provider.CurrentModel, options.DefaultMaxTokens)
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
	m.conv.AddNotice(fmt.Sprintf("\u26a1 Auto-compacting conversation (%.0f%% context used)...", m.getContextUsagePercent()))
	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, m.output.Spinner.Tick, startCompact(m))
	return tea.Batch(commitCmds...)
}

// handleCompactResult processes the result of a compaction operation.
func (m *model) handleCompactResult(msg appcompact.CompactResultMsg) tea.Cmd {
	shouldContinue := m.conv.Compact.AutoContinue
	m.conv.Compact.Reset()

	if msg.Error != nil {
		m.conv.AddNotice(fmt.Sprintf("Compact failed: %v", msg.Error))
		return tea.Batch(m.commitMessages()...)
	}

	// Clear messages — the summary lives in session-memory, not in the message list.
	m.resetAfterCompact()

	// Persist the compaction summary as session memory
	if m.session.Store != nil && m.session.CurrentID != "" {
		_ = m.session.Store.SaveSessionMemory(m.session.CurrentID, msg.Summary)
	}
	m.session.Memory = msg.Summary

	cmds := []tea.Cmd{tea.ClearScreen}
	if shouldContinue {
		m.conv.Append(message.ChatMessage{
			Role:    message.RoleUser,
			Content: "Continue with the task. The conversation was auto-compacted to free up context.",
		})
		cmds = append(cmds, m.startLLMStream(m.buildExtraContext()))
	} else {
		m.conv.AddNotice(fmt.Sprintf("Compacted %d messages into session memory.", msg.OriginalCount))
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
