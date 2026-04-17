// Package app provides the unified entry point for interactive and non-interactive modes.
package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/app/kit"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/hook"
)

// Run routes to either print mode or interactive TUI.
func Run(opts setting.RunOptions) error {
	if opts.Print != "" {
		return runPrint(opts.Print)
	}

	if userQuit, err := kit.ResolveTheme(setting.LoadTheme(), setting.SaveTheme); userQuit || err != nil {
		return err
	}

	m, err := initModel(opts)
	if err != nil {
		return err
	}

	finalModel, err := tea.NewProgram(m).Run()
	if err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	if fm, ok := finalModel.(*model); ok {
		printExitMessage(fm)
	}
	return nil
}

func initModel(opts setting.RunOptions) (*model, error) {
	if err := initInfrastructure(); err != nil {
		return nil, err
	}
	m, err := newModel(opts)
	if err != nil {
		return nil, err
	}
	m.fireStartupHooks()
	return m, nil
}

func (m *model) configureAsyncHookCallback() {
	if m.runtime.HookEngine == nil || m.systemInput.AsyncHookQueue == nil {
		return
	}
	queue := m.systemInput.AsyncHookQueue
	m.runtime.HookEngine.SetAsyncHookCallback(func(result hook.AsyncHookResult) {
		reason := result.BlockReason
		if reason == "" {
			reason = "asynchronous hook requested a rewake"
		}
		queue.Push(appsystem.AsyncHookRewake{
			Notice:             fmt.Sprintf("Async hook blocked: %s", reason),
			Context:            []string{formatAsyncHookContinuationContext(result, reason)},
			ContinuationPrompt: "A background policy hook reported a blocking condition. Re-evaluate the plan and choose a safer next step.",
		})
	})
}

func (m *model) fireStartupHooks() {
	if m.runtime.HookEngine == nil {
		return
	}
	m.runtime.HookEngine.ExecuteAsync(hook.Setup, hook.HookInput{
		Trigger: "init",
	})

	source := "startup"
	if m.runtime.SessionID != "" {
		source = "resume"
	}
	outcome := m.runtime.HookEngine.Execute(context.Background(), hook.SessionStart, hook.HookInput{
		Source: source,
		Model:  m.getModelID(),
	})
	m.applyRuntimeHookOutcome(outcome)
	if outcome.AdditionalContext != "" {
		m.conv.Append(core.ChatMessage{
			Role:    core.RoleUser,
			Content: outcome.AdditionalContext,
		})
	}
}

func printExitMessage(m *model) {
	if m.runtime.SessionID != "" {
		dim := kit.DimStyle()
		fmt.Println()
		fmt.Println(dim.Render("Resume this session with:"))
		fmt.Println(dim.Render("gen -r " + m.runtime.SessionID))
		fmt.Println()
	}
}
