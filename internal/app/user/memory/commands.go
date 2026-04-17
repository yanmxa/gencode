// Memory and initialization commands (/init, /memory) for project memory file management.
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/yanmxa/gencode/internal/core/system"
	"github.com/yanmxa/gencode/internal/app/kit"
)

// HandleInitCommand handles the /init command.
// cwd is the current working directory.
func HandleInitCommand(cwd, args string) (string, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	isClaude := strings.Contains(args, "--claude")

	subCmd := ""
	if len(parts) > 0 && !strings.HasPrefix(parts[0], "--") {
		subCmd = strings.ToLower(parts[0])
	}

	switch subCmd {
	case "local":
		return handleInitLocal(cwd)
	case "rules":
		return handleInitRules(cwd, isClaude)
	default:
		return handleInitProject(cwd, isClaude)
	}
}

func handleInitProject(cwd string, isClaude bool) (string, error) {
	var targetDir, fileName string
	if isClaude {
		targetDir = filepath.Join(cwd, ".claude")
		fileName = "CLAUDE.md"
	} else {
		targetDir = filepath.Join(cwd, ".gen")
		fileName = "GEN.md"
	}
	filePath := filepath.Join(targetDir, fileName)

	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("File already exists: %s\nUse /memory edit to modify it.", filePath), nil
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}
	if err := os.WriteFile(filePath, []byte(getProjectTemplate(cwd)), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	return fmt.Sprintf("Created %s\n\nEdit with: /memory edit", filePath), nil
}

func handleInitLocal(cwd string) (string, error) {
	targetDir := filepath.Join(cwd, ".gen")
	filePath := filepath.Join(targetDir, "GEN.local.md")

	if _, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("File already exists: %s\nUse /memory edit local to modify it.", filePath), nil
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", targetDir, err)
	}
	if err := os.WriteFile(filePath, []byte(getLocalTemplate()), 0o644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %w", filePath, err)
	}

	addToGitignore(cwd, "GEN.local.md")

	return fmt.Sprintf("Created %s (added to .gitignore)\n\nEdit with: /memory edit local", filePath), nil
}

func handleInitRules(cwd string, isClaude bool) (string, error) {
	var rulesDir string
	if isClaude {
		rulesDir = filepath.Join(cwd, ".claude", "rules")
	} else {
		rulesDir = filepath.Join(cwd, ".gen", "rules")
	}

	if _, err := os.Stat(rulesDir); err == nil {
		return fmt.Sprintf("Directory already exists: %s", rulesDir), nil
	}

	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", rulesDir, err)
	}

	examplePath := filepath.Join(rulesDir, "example.md")
	if err := os.WriteFile(examplePath, []byte(getRulesTemplate()), 0o644); err != nil {
		return "", fmt.Errorf("failed to write example rule: %w", err)
	}

	return fmt.Sprintf("Created %s\n\nAdd .md files to this directory to define rules.\nExample created: %s", rulesDir, examplePath), nil
}

// addToGitignore adds an entry to .gitignore in the given directory if not already present.
// Creates the file if it doesn't exist.
func addToGitignore(cwd, entry string) {
	gitignorePath := filepath.Join(cwd, ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return
	}

	content := string(data)
	// Check line-by-line to avoid substring false positives
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == entry {
			return
		}
	}

	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	_ = os.WriteFile(gitignorePath, []byte(content), 0o644)
}

// HandleMemoryCommand handles the /memory command.
// selector is the memory selector model. cwd, width, height are from the app model.
// Returns (result string, editFilePath string, error).
// When editFilePath is non-empty, the caller should open an external editor for that file.
func HandleMemoryCommand(selector *Model, cwd string, width, height int, args string) (string, string, error) {
	args = strings.TrimSpace(args)
	parts := strings.Fields(args)

	if len(parts) == 0 {
		selector.EnterSelect(cwd, width, height)
		return "", "", nil
	}

	subCmd := strings.ToLower(parts[0])

	scope := "project"
	if len(parts) > 1 {
		scope = strings.ToLower(parts[1])
	}

	switch subCmd {
	case "list":
		result, err := handleMemoryList(cwd)
		return result, "", err
	case "show":
		result, err := handleMemoryShow(cwd)
		return result, "", err
	case "edit":
		editPath, err := handleMemoryEdit(cwd, scope)
		if err != nil {
			return "", "", err
		}
		if editPath == "" {
			return "No project memory file found.\n\nCreate with: /init", "", nil
		}
		return "", editPath, nil
	default:
		return "Usage: /memory [list|show|edit] [global|project|local]", "", nil
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

// handleMemoryList lists all memory files.
func handleMemoryList(cwd string) (string, error) {
	paths := system.GetAllMemoryPaths(cwd)
	state := &memoryListState{cwd: cwd}

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
	return kit.ShortenPath(path)
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

// handleMemoryShow shows the current loaded memory content.
func handleMemoryShow(cwd string) (string, error) {
	content := system.LoadMemory(cwd)
	if content == "" {
		return "No memory files loaded.\n\nCreate project memory with: /init", nil
	}

	const maxShow = 2000
	if len(content) > maxShow {
		content = content[:maxShow] + "\n\n... (truncated)"
	}

	return fmt.Sprintf("Current Memory:\n\n%s", content), nil
}

// handleMemoryEdit resolves the file to edit for the given scope.
// Returns the file path to edit, or an empty string with a message if no file was found.
func handleMemoryEdit(cwd, scope string) (string, error) {
	paths := system.GetAllMemoryPaths(cwd)

	switch scope {
	case "global", "user":
		filePath, err := ensureMemoryFile(paths.Global, getGlobalTemplate())
		if err != nil {
			return "", err
		}
		return filePath, nil

	case "local":
		filePath, err := ensureMemoryFile(paths.Local, getLocalTemplate())
		if err != nil {
			return "", err
		}
		addToGitignore(cwd, "GEN.local.md")
		return filePath, nil

	default:
		filePath := system.FindMemoryFile(paths.Project)
		if filePath == "" {
			// Return empty path; caller should display the message.
			return "", nil
		}
		return filePath, nil
	}
}

// ensureMemoryFile finds or creates a memory file from the given search paths.
func ensureMemoryFile(searchPaths []string, template string) (string, error) {
	filePath := system.FindMemoryFile(searchPaths)
	if filePath != "" {
		return filePath, nil
	}

	filePath = searchPaths[0]
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(template), 0o644); err != nil {
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

`+"`"+`bash
# Add your build commands here
`+"`"+`

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

// CreateMemoryFile creates a memory file if it doesn't exist.
func CreateMemoryFile(filePath, level, cwd string) error {
	template := getTemplateForLevel(level, cwd)
	if _, err := ensureMemoryFile([]string{filePath}, template); err != nil {
		return err
	}
	if level == "local" {
		addToGitignore(cwd, "GEN.local.md")
	}
	return nil
}

// getTemplateForLevel returns the template content for a given memory level.
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

// FormatMemoryDisplayPath formats a memory file path for display.
func FormatMemoryDisplayPath(filePath, level, cwd string) string {
	if level == "project" || level == "local" {
		if rel, err := filepath.Rel(cwd, filePath); err == nil {
			return rel
		}
	}
	return kit.ShortenPath(filePath)
}
