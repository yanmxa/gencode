package app

import (
	"context"

	"github.com/yanmxa/gencode/internal/app/trigger"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/setting"
)

func (m *model) fireFileChanged(filePath, source string) {
	if m.runtime.HookEngine == nil || filePath == "" {
		return
	}
	outcome := m.runtime.HookEngine.Execute(context.Background(), hook.FileChanged, hook.HookInput{
		FilePath: filePath,
		Source:   source,
		Event:    "change",
	})
	m.applyRuntimeHookOutcome(outcome)
}

func (m *model) changeCwd(newCwd string) {
	if newCwd == "" || newCwd == m.cwd {
		return
	}

	oldCwd := m.cwd
	m.cwd = newCwd
	m.isGit = setting.IsGitRepo(newCwd)
	m.userInput.Suggestions.SetCwd(newCwd)
	if m.userInput.Suggestions.GetSuggestionType() == suggest.TypeFile {
		m.userInput.Suggestions.Hide()
	}

	m.runtime.ClearCachedInstructions()
	m.runtime.RefreshMemoryContext(newCwd, "cwd_changed")
	m.reloadProjectContext(newCwd)
	m.reconfigureAgentTool()

	if m.runtime.HookEngine != nil {
		m.runtime.HookEngine.SetCwd(newCwd)
		outcome := m.runtime.HookEngine.Execute(context.Background(), hook.CwdChanged, hook.HookInput{
			OldCwd: oldCwd,
			NewCwd: newCwd,
		})
		m.applyRuntimeHookOutcome(outcome)
	}
}

func (m *model) reloadProjectContext(cwd string) {
	initExtensions(cwd)
	setting.Initialize(cwd)
	if m.runtime.HookEngine != nil {
		plugin.MergePluginHooksIntoSettings(setting.DefaultSetup)
	}
	m.runtime.ApplySettings(setting.DefaultSetup)
}

func (m *model) applyRuntimeHookOutcome(outcome hook.HookOutcome) {
	if outcome.InitialUserMessage != "" && m.initialPrompt == "" && len(m.conv.Messages) == 0 {
		m.initialPrompt = outcome.InitialUserMessage
	}
	if len(outcome.WatchPaths) == 0 {
		return
	}
	if m.fileWatcher == nil {
		queue := m.systemInput.AsyncHookQueue
		m.fileWatcher = trigger.NewFileWatcher(m.runtime.HookEngine, func(outcome hook.HookOutcome) {
			// Route through AsyncHookQueue to avoid mutating model from
			// the file watcher's background goroutine. The Bubble Tea
			// tick handler processes these safely in the Update loop.
			if queue != nil && outcome.InitialUserMessage != "" {
				queue.Push(trigger.AsyncHookRewake{
					Notice:  "File watcher hook triggered",
					Context: []string{outcome.InitialUserMessage},
				})
			}
		})
	}
	m.fileWatcher.SetPaths(outcome.WatchPaths)
}

