// Memory and initialization commands (/init, /memory) for project memory file management.
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/system"
)

func handleInitCommand(ctx context.Context, m *model, args string) (string, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	isClaude := strings.Contains(args, "--claude")

	subCmd := ""
	if len(parts) > 0 && !strings.HasPrefix(parts[0], "--") {
		subCmd = strings.ToLower(parts[0])
	}

	switch subCmd {
	case "local":
		return handleInitLocal(m)
	case "rules":
		return handleInitRules(m, isClaude)
	default:
		return handleInitProject(m, isClaude)
	}
}

func handleInitProject(m *model, isClaude bool) (string, error) {
	var targetDir, fileName string
	if isClaude {
		targetDir = filepath.Join(m.cwd, ".claude")
		fileName = "CLAUDE.md"
	} else {
		targetDir = filepath.Join(m.cwd, ".gen")
		fileName = "GEN.md"
	}
	filePath := filepath.Join(targetDir, fileName)

	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("File already exists: %s\nUse /memory edit to modify it.", filePath), nil
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}
	if err := os.WriteFile(filePath, []byte(getProjectTemplate(m.cwd)), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return fmt.Sprintf("Created %s\n\nEdit with: /memory edit", filePath), nil
}

func handleInitLocal(m *model) (string, error) {
	targetDir := filepath.Join(m.cwd, ".gen")
	filePath := filepath.Join(targetDir, "GEN.local.md")

	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("File already exists: %s\nUse /memory edit local to modify it.", filePath), nil
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}
	if err := os.WriteFile(filePath, []byte(getLocalTemplate()), 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	addToGitignore(m.cwd, "GEN.local.md")

	return fmt.Sprintf("Created %s (added to .gitignore)\n\nEdit with: /memory edit local", filePath), nil
}

func handleInitRules(m *model, isClaude bool) (string, error) {
	var rulesDir string
	if isClaude {
		rulesDir = filepath.Join(m.cwd, ".claude", "rules")
	} else {
		rulesDir = filepath.Join(m.cwd, ".gen", "rules")
	}

	if _, err := os.Stat(rulesDir); err == nil {
		return fmt.Sprintf("Directory already exists: %s", rulesDir), nil
	}

	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", rulesDir, err)
	}

	examplePath := filepath.Join(rulesDir, "example.md")
	if err := os.WriteFile(examplePath, []byte(getRulesTemplate()), 0644); err != nil {
		return "", fmt.Errorf("failed to write example rule: %w", err)
	}

	return fmt.Sprintf("Created %s\n\nAdd .md files to this directory to define rules.\nExample created: %s", rulesDir, examplePath), nil
}

func addToGitignore(cwd, entry string) {
	gitignorePath := filepath.Join(cwd, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		return
	}

	content := string(data)
	if strings.Contains(content, entry) {
		return
	}

	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	os.WriteFile(gitignorePath, []byte(content), 0644)
}

func handleMemoryCommand(ctx context.Context, m *model, args string) (string, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		m.memorySelector.EnterMemorySelect(m.cwd, m.width, m.height)
		return "", nil
	}

	subCmd := strings.ToLower(parts[0])

	scope := "project"
	if len(parts) > 1 {
		scope = strings.ToLower(parts[1])
	}

	switch subCmd {
	case "list":
		return handleMemoryList(m)
	case "show":
		return handleMemoryShow(m)
	case "edit":
		return handleMemoryEdit(m, scope)
	default:
		return "Usage: /memory [list|show|edit] [global|project|local]", nil
	}
}

type memoryListState struct {
	cwd        string
	totalFiles int
	totalSize  int64
}

const (
	memoryBoxWidth = 53
	memoryMaxPath  = 36
)

func handleMemoryList(m *model) (string, error) {
	paths := system.GetAllMemoryPaths(m.cwd)
	state := &memoryListState{cwd: m.cwd}

	var sb strings.Builder

	sb.WriteString("╭─ Memory Files ─────────────────────────────────────╮\n")
	sb.WriteString(formatBoxLine(""))

	state.writeSection(&sb, "Global", paths.Global, paths.GlobalRules, paths.Global[0], false)
	state.writeSection(&sb, "Project", paths.Project, paths.ProjectRules, "/init", true)
	state.writeLocalSection(&sb, paths.Local)

	sb.WriteString("╰────────────────────────────────────────────────────╯\n")

	if state.totalFiles > 0 {
		fmt.Fprintf(&sb, "  Total: %d file(s) loaded (%s)\n", state.totalFiles, system.FormatFileSize(state.totalSize))
	} else {
		sb.WriteString("  No memory files loaded. Create with /init\n")
	}

	sb.WriteString("\n  Tip: Use @path/to/file.md in memory files to import other files.\n")

	return sb.String(), nil
}

func (s *memoryListState) writeSection(sb *strings.Builder, label string, mainPaths []string, rulesDir, createHint string, isProject bool) {
	mainFound := system.FindMemoryFile(mainPaths)
	rulesFiles := system.ListRulesFiles(rulesDir)

	if mainFound != "" || len(rulesFiles) > 0 {
		sb.WriteString(formatBoxLine(fmt.Sprintf(" ● %s", label)))
		if mainFound != "" {
			s.writeFileLine(sb, mainFound, isProject)
		}
		for _, rf := range rulesFiles {
			s.writeFileLine(sb, rf, isProject)
		}
	} else {
		sb.WriteString(formatBoxLine(fmt.Sprintf(" ○ %s (not found)", label)))
		sb.WriteString(formatBoxLine(fmt.Sprintf("   Create: %s", createHint)))
	}
	sb.WriteString(formatBoxLine(""))
}

func (s *memoryListState) writeLocalSection(sb *strings.Builder, localPaths []string) {
	localFound := system.FindMemoryFile(localPaths)
	if localFound != "" {
		sb.WriteString(formatBoxLine(" ● Local (git-ignored)"))
		s.writeFileLine(sb, localFound, true)
	} else {
		sb.WriteString(formatBoxLine(" ○ Local (not found)"))
		sb.WriteString(formatBoxLine("   Create: /init local"))
	}
	sb.WriteString(formatBoxLine(""))
}

func (s *memoryListState) writeFileLine(sb *strings.Builder, path string, isProject bool) {
	size := system.GetFileSize(path)
	s.totalFiles++
	s.totalSize += size

	displayPath := shortenPathForDisplay(path, s.cwd, isProject)
	displayPath = truncatePathKeepFilename(displayPath, memoryMaxPath)
	sizeStr := fmt.Sprintf("(%s)", system.FormatFileSize(size))
	sb.WriteString(formatBoxLine(fmt.Sprintf("   %s %s", padRight(displayPath, memoryMaxPath), sizeStr)))
}

func formatBoxLine(content string) string {
	visibleLen := utf8.RuneCountInString(content)
	padding := max(memoryBoxWidth-visibleLen-2, 0)
	return fmt.Sprintf("│ %s%s│\n", content, strings.Repeat(" ", padding))
}

func shortenPathForDisplay(path, cwd string, isProject bool) string {
	if isProject {
		if rel, err := filepath.Rel(cwd, path); err == nil {
			return rel
		}
	}
	return ShortenPath(path)
}

func truncatePathKeepFilename(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}

	base := filepath.Base(path)
	if len(base) >= maxLen-3 {
		return base[:maxLen-3] + "..."
	}

	remaining := maxLen - len(base) - 4
	if remaining > 0 {
		dir := filepath.Dir(path)
		if len(dir) > remaining {
			dir = dir[len(dir)-remaining:]
		}
		return "..." + dir + "/" + base
	}
	return base
}

func padRight(s string, length int) string {
	if len(s) >= length {
		return s[:length]
	}
	return s + strings.Repeat(" ", length-len(s))
}

func handleMemoryShow(m *model) (string, error) {
	content := system.LoadMemory(m.cwd)
	if content == "" {
		return "No memory files loaded.\n\nCreate project memory with: /init", nil
	}

	const maxShow = 2000
	if len(content) > maxShow {
		content = content[:maxShow] + "\n\n... (truncated)"
	}

	return fmt.Sprintf("Current Memory:\n\n%s", content), nil
}

func handleMemoryEdit(m *model, scope string) (string, error) {
	paths := system.GetAllMemoryPaths(m.cwd)

	switch scope {
	case "global", "user":
		filePath, err := ensureMemoryFile(paths.Global, getGlobalTemplate())
		if err != nil {
			return "", err
		}
		m.editingMemoryFile = filePath
		return "", nil

	case "local":
		filePath, err := ensureMemoryFile(paths.Local, getLocalTemplate())
		if err != nil {
			return "", err
		}
		addToGitignore(m.cwd, "GEN.local.md")
		m.editingMemoryFile = filePath
		return "", nil

	default:
		filePath := system.FindMemoryFile(paths.Project)
		if filePath == "" {
			return "No project memory file found.\n\nCreate with: /init", nil
		}
		m.editingMemoryFile = filePath
		return "", nil
	}
}

func ensureMemoryFile(searchPaths []string, template string) (string, error) {
	filePath := system.FindMemoryFile(searchPaths)
	if filePath != "" {
		return filePath, nil
	}

	filePath = searchPaths[0]
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(template), 0644); err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	return filePath, nil
}

func getProjectTemplate(cwd string) string {
	projectName := filepath.Base(cwd)
	return fmt.Sprintf(`# GEN.md

This file provides guidance to GenCode when working with code in this repository.

## Project Overview

%s - Describe what this project does.

## Build & Run

`+"```bash"+`
# Add your build commands here
`+"```"+`

## Architecture

<!-- Key directories and their purpose -->

## Key Patterns

<!-- Important conventions to follow -->
`, projectName)
}

func getGlobalTemplate() string {
	return `# GEN.md

Global instructions for GenCode (applies to all projects).

## Coding Preferences

<!-- Your preferred coding style -->

## Security

<!-- Security practices to follow -->
`
}

func getLocalTemplate() string {
	return `# GEN.local.md

Local instructions for this project (not committed to git).

Use this file for:
- Personal notes and reminders
- Environment-specific settings
- Credentials and secrets (keep these safe!)
- Work-in-progress ideas

## Notes

<!-- Your local notes here -->
`
}

func getRulesTemplate() string {
	return `# Example Rule

This file defines specific rules for GenCode to follow.

## Guidelines

- Add specific guidelines here
- Each rule file should focus on one topic
- Rules are loaded alphabetically by filename

## Example

<!-- Remove this example and add your actual rules -->
`
}

func startExternalEditor(filePath string) tea.Cmd {
	editor := getEditor()
	cmd := exec.Command(editor, filePath)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return EditorFinishedMsg{Err: err}
	})
}

func getEditor() string {
	if editor := os.Getenv("EDITOR"); editor != "" {
		return editor
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		return editor
	}
	for _, e := range []string{"vim", "nano", "vi"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return "vi"
}

func (m model) handleMemorySelected(msg MemorySelectedMsg) (tea.Model, tea.Cmd) {
	filePath := msg.Path

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := createMemoryFile(filePath, msg.Level, m.cwd); err != nil {
			m.messages = append(m.messages, chatMessage{
				role:    roleNotice,
				content: fmt.Sprintf("Error: %v", err),
			})
			return m, tea.Batch(m.commitMessages()...)
		}
	}

	m.editingMemoryFile = filePath

	displayPath := formatMemoryDisplayPath(filePath, msg.Level, m.cwd)

	m.messages = append(m.messages, chatMessage{
		role:    roleNotice,
		content: fmt.Sprintf("Opening %s memory: %s", msg.Level, displayPath),
	})

	commitCmds := m.commitMessages()
	commitCmds = append(commitCmds, startExternalEditor(filePath))
	return m, tea.Batch(commitCmds...)
}

func createMemoryFile(filePath, level, cwd string) error {
	template := getTemplateForLevel(level, cwd)
	if _, err := ensureMemoryFile([]string{filePath}, template); err != nil {
		return err
	}
	if level == "local" {
		addToGitignore(cwd, "GEN.local.md")
	}
	return nil
}

func getTemplateForLevel(level, cwd string) string {
	switch level {
	case "global":
		return getGlobalTemplate()
	case "project":
		return getProjectTemplate(cwd)
	case "local":
		return getLocalTemplate()
	default:
		return ""
	}
}

func formatMemoryDisplayPath(filePath, level, cwd string) string {
	if level == "project" || level == "local" {
		if rel, err := filepath.Rel(cwd, filePath); err == nil {
			return rel
		}
	}
	return ShortenPath(filePath)
}
