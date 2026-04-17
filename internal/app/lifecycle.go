package app

import (
	"context"

	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/setting"
	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/hook"
	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/app/kit/suggest"
)

func (m *model) refreshMemoryContext(loadReason string) {
	files := system.LoadMemoryFiles(m.cwd)
	var userParts, projectParts []string
	for _, f := range files {
		switch f.Level {
		case "global":
			userParts = append(userParts, f.Content)
		case "project", "local":
			projectParts = append(projectParts, f.Content)
		}
		if m.runtime.HookEngine != nil {
			m.runtime.HookEngine.ExecuteAsync(hook.InstructionsLoaded, hook.HookInput{
				FilePath:   f.Path,
				MemoryType: memoryTypeForLevel(f.Level),
				LoadReason: loadReason,
			})
		}
	}

	m.runtime.CachedUserInstructions = joinSections(userParts)
	m.runtime.CachedProjectInstructions = joinSections(projectParts)
}

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

	m.runtime.CachedUserInstructions = ""
	m.runtime.CachedProjectInstructions = ""
	m.refreshMemoryContext("cwd_changed")
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
	m.runtime.Settings = setting.DefaultSetup
	if m.runtime.DisabledTools == nil {
		m.runtime.DisabledTools = make(map[string]bool)
	} else {
		for k := range m.runtime.DisabledTools {
			delete(m.runtime.DisabledTools, k)
		}
	}
	for k, v := range setting.DefaultSetup.DisabledTools {
		m.runtime.DisabledTools[k] = v
	}
	if m.runtime.HookEngine != nil {
		plugin.MergePluginHooksIntoSettings(setting.DefaultSetup)
		m.runtime.HookEngine.SetSettings(setting.DefaultSetup)
	}
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
		m.fileWatcher = appsystem.NewFileWatcher(m.runtime.HookEngine, func(outcome hook.HookOutcome) {
			// Route through AsyncHookQueue to avoid mutating model from
			// the file watcher's background goroutine. The Bubble Tea
			// tick handler processes these safely in the Update loop.
			if queue != nil && outcome.InitialUserMessage != "" {
				queue.Push(appsystem.AsyncHookRewake{
					Notice:  "File watcher hook triggered",
					Context: []string{outcome.InitialUserMessage},
				})
			}
		})
	}
	m.fileWatcher.SetPaths(outcome.WatchPaths)
}

func memoryTypeForLevel(level string) string {
	switch level {
	case "global":
		return "User"
	case "local":
		return "Local"
	default:
		return "Project"
	}
}

func joinSections(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "\n\n" + parts[i]
	}
	return result
}
