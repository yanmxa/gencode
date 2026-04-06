package app

import (
	"context"
	"path/filepath"

	"github.com/yanmxa/gencode/internal/hooks"
	"github.com/yanmxa/gencode/internal/system"
	"github.com/yanmxa/gencode/internal/ui/suggest"
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

	m.memory.CachedUser = joinSections(userParts)
	m.memory.CachedProject = joinSections(projectParts)
}

func (m *model) fireConfigChange(source, filePath string) {
	if m.hookEngine == nil {
		return
	}
	outcome := m.hookEngine.Execute(context.Background(), hooks.ConfigChange, hooks.HookInput{
		Source:   source,
		FilePath: filePath,
	})
	m.applyRuntimeHookOutcome(outcome)
	m.fireFileChanged(filePath, source)
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
	m.isGit = isGitRepo(newCwd)
	m.input.Suggestions.SetCwd(newCwd)
	if m.input.Suggestions.GetSuggestionType() == suggest.TypeFile {
		m.input.Suggestions.Hide()
	}

	m.memory.CachedUser = ""
	m.memory.CachedProject = ""
	m.refreshMemoryContext("cwd_changed")
	m.reconfigureAgentTool()

	if m.hookEngine != nil {
		m.hookEngine.SetCwd(newCwd)
		m.hookEngine.SetAgentRunner(newHookAgentRunner(m.provider.LLM, m.settings, newCwd, m.isGit, m.mcp.Registry))
		outcome := m.hookEngine.Execute(context.Background(), hooks.CwdChanged, hooks.HookInput{
			OldCwd: oldCwd,
			NewCwd: newCwd,
		})
		m.applyRuntimeHookOutcome(outcome)
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
		m.fileWatcher = newFileWatcher(m.hookEngine, func(outcome hooks.HookOutcome) {
			m.applyRuntimeHookOutcome(outcome)
		})
	}
	m.fileWatcher.SetPaths(outcome.WatchPaths)
}

func configSourceFromPath(path string) string {
	base := filepath.Base(path)
	switch {
	case base == "settings.local.json":
		return "local_settings"
	case base == "settings.json":
		return "project_settings"
	default:
		return "user_settings"
	}
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
