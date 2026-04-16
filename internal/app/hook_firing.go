package app

import (
	"context"

	appsystem "github.com/yanmxa/gencode/internal/app/system"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/extension/mcp"
	"github.com/yanmxa/gencode/internal/hook"
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
		if m.hookEngine != nil {
			m.hookEngine.ExecuteAsync(hooks.InstructionsLoaded, hooks.HookInput{
				FilePath:   f.Path,
				MemoryType: memoryTypeForLevel(f.Level),
				LoadReason: loadReason,
			})
		}
	}

	m.cachedUserInstructions = joinSections(userParts)
	m.cachedProjectInstructions = joinSections(projectParts)
}

func (m *model) fireFileChanged(filePath, source string) {
	if m.hookEngine == nil || filePath == "" {
		return
	}
	outcome := m.hookEngine.Execute(context.Background(), hooks.FileChanged, hooks.HookInput{
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
	m.isGit = config.IsGitRepo(newCwd)
	m.userInput.Suggestions.SetCwd(newCwd)
	if m.userInput.Suggestions.GetSuggestionType() == suggest.TypeFile {
		m.userInput.Suggestions.Hide()
	}

	m.cachedUserInstructions = ""
	m.cachedProjectInstructions = ""
	m.refreshMemoryContext("cwd_changed")
	m.reloadProjectContext(newCwd)
	m.reconfigureAgentTool()

	if m.hookEngine != nil {
		m.hookEngine.SetCwd(newCwd)
		m.hookEngine.SetAgentRunner(NewHookAgentRunner(m.llmProvider, m.settings, newCwd, m.isGit, mcp.DefaultRegistry, m.getModelID()))
		outcome := m.hookEngine.Execute(context.Background(), hooks.CwdChanged, hooks.HookInput{
			OldCwd: oldCwd,
			NewCwd: newCwd,
		})
		m.applyRuntimeHookOutcome(outcome)
	}
}

func (m *model) reloadProjectContext(cwd string) {
	initExt(cwd)

	settings := initSettings(cwd)
	m.settings = settings
	if m.disabledTools == nil {
		m.disabledTools = make(map[string]bool)
	} else {
		for k := range m.disabledTools {
			delete(m.disabledTools, k)
		}
	}
	for k, v := range settings.DisabledTools {
		m.disabledTools[k] = v
	}
	if m.hookEngine != nil {
		m.hookEngine.SetSettings(settings)
	}
}

func (m *model) applyRuntimeHookOutcome(outcome hooks.HookOutcome) {
	if outcome.InitialUserMessage != "" && m.initialPrompt == "" && len(m.conv.Messages) == 0 {
		m.initialPrompt = outcome.InitialUserMessage
	}
	if len(outcome.WatchPaths) == 0 {
		return
	}
	if m.fileWatcher == nil {
		queue := m.systemInput.AsyncHookQueue
		m.fileWatcher = appsystem.NewFileWatcher(m.hookEngine, func(outcome hooks.HookOutcome) {
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
