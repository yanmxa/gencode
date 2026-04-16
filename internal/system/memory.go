package system

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yanmxa/gencode/internal/util/log"
	"go.uber.org/zap"
)

const (
	maxImportDepth = 5
)

// MemoryFile represents a loaded memory file with metadata.
type MemoryFile struct {
	Path    string
	Size    int64
	Content string
	Level   string // "global", "project", or "local"
	Source  string // "rules" for rules directory files, empty otherwise
}

// LoadInstructions loads user-level and project-level instructions separately.
func LoadInstructions(cwd string) (user, project string) {
	files := LoadMemoryFiles(cwd)
	var userParts, projectParts []string
	for _, f := range files {
		switch f.Level {
		case "global":
			userParts = append(userParts, f.Content)
		case "project", "local":
			projectParts = append(projectParts, f.Content)
		}
	}
	return strings.Join(userParts, "\n\n"), strings.Join(projectParts, "\n\n")
}

// LoadMemory loads memory content from standard locations.
func LoadMemory(cwd string) string {
	files := LoadMemoryFiles(cwd)
	if len(files) == 0 {
		return ""
	}
	var parts []string
	for _, f := range files {
		parts = append(parts, f.Content)
	}
	return strings.Join(parts, "\n\n")
}

// LoadMemoryFiles loads all memory files with metadata.
// Returns files in order: global, global rules, project, project rules, local.
func LoadMemoryFiles(cwd string) []MemoryFile {
	var files []MemoryFile
	homeDir, _ := os.UserHomeDir()
	seen := make(map[string]bool)

	userSources := []string{
		filepath.Join(homeDir, ".gen", "GEN.md"),
		filepath.Join(homeDir, ".claude", "CLAUDE.md"),
	}
	if f := loadMemoryFile(userSources, "global", "", seen); f != nil {
		files = append(files, *f)
	}

	userRulesDir := filepath.Join(homeDir, ".gen", "rules")
	files = append(files, loadRulesDirectory(userRulesDir, "global", seen)...)

	projectSources := []string{
		filepath.Join(cwd, ".gen", "GEN.md"),
		filepath.Join(cwd, "GEN.md"),
		filepath.Join(cwd, ".claude", "CLAUDE.md"),
		filepath.Join(cwd, "CLAUDE.md"),
	}
	if f := loadMemoryFile(projectSources, "project", "", seen); f != nil {
		files = append(files, *f)
	}

	projectRulesDir := filepath.Join(cwd, ".gen", "rules")
	files = append(files, loadRulesDirectory(projectRulesDir, "project", seen)...)

	localSources := []string{
		filepath.Join(cwd, ".gen", "GEN.local.md"),
	}
	if f := loadMemoryFile(localSources, "local", "", seen); f != nil {
		files = append(files, *f)
	}

	return files
}

func loadMemoryFile(sources []string, level, source string, seen map[string]bool) *MemoryFile {
	for _, src := range sources {
		info, err := os.Stat(src)
		if err != nil {
			continue
		}
		if seen[src] {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		seen[src] = true
		content = resolveImports(content, filepath.Dir(src), 0, seen)

		log.Logger().Info("Loaded memory file",
			zap.String("path", src),
			zap.Int64("bytes", info.Size()),
			zap.String("level", level))

		return &MemoryFile{
			Path:    src,
			Size:    info.Size(),
			Content: fmt.Sprintf("<!-- Source: %s -->\n%s", src, content),
			Level:   level,
			Source:  source,
		}
	}
	return nil
}

func loadRulesDirectory(dir string, level string, seen map[string]bool) []MemoryFile {
	var files []MemoryFile
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}
	var mdFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			mdFiles = append(mdFiles, filepath.Join(dir, name))
		}
	}
	sort.Strings(mdFiles)
	for _, path := range mdFiles {
		if f := loadMemoryFile([]string{path}, level, "rules", seen); f != nil {
			files = append(files, *f)
		}
	}
	return files
}

// importRe matches @import directives in memory files (e.g., @file.md).
var importRe = regexp.MustCompile(`(?m)^@([^\s@]+\.md)\s*$`)

func resolveImports(content string, basePath string, depth int, seen map[string]bool) string {
	if depth >= maxImportDepth {
		return content
	}
	return importRe.ReplaceAllStringFunc(content, func(match string) string {
		importPath := strings.TrimPrefix(strings.TrimSpace(match), "@")
		fullPath := filepath.Clean(filepath.Join(basePath, importPath))

		// Path traversal guard: resolved path must stay under basePath.
		// Use trailing separator to prevent prefix collisions (e.g., /tmp/project vs /tmp/projectile).
		baseWithSep := basePath + string(filepath.Separator)
		if fullPath != basePath && !strings.HasPrefix(fullPath, baseWithSep) {
			return fmt.Sprintf("<!-- Import blocked (outside base): @%s -->", importPath)
		}

		// Symlink guard: resolve symlinks and re-check to prevent escapes
		// via symlinks that point outside the base directory.
		if realPath, err := filepath.EvalSymlinks(fullPath); err == nil {
			realBase, _ := filepath.EvalSymlinks(basePath)
			if realBase != "" {
				realBaseWithSep := realBase + string(filepath.Separator)
				if realPath != realBase && !strings.HasPrefix(realPath, realBaseWithSep) {
					return fmt.Sprintf("<!-- Import blocked (symlink escape): @%s -->", importPath)
				}
			}
		}

		if seen[fullPath] {
			return fmt.Sprintf("<!-- Skipped (cycle): @%s -->", importPath)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Sprintf("<!-- Import not found: @%s -->", importPath)
		}
		seen[fullPath] = true
		importedContent := strings.TrimSpace(string(data))

		log.Logger().Info("Resolved import",
			zap.String("import", importPath),
			zap.String("fullPath", fullPath),
			zap.Int("depth", depth))

		importedContent = resolveImports(importedContent, filepath.Dir(fullPath), depth+1, seen)
		return fmt.Sprintf("<!-- Imported: %s -->\n%s", importPath, importedContent)
	})
}

// MemoryPaths holds categorized memory file paths.
type MemoryPaths struct {
	Global       []string
	GlobalRules  string
	Project      []string
	ProjectRules string
	Local        []string
}

// GetAllMemoryPaths returns all memory paths organized by category.
func GetAllMemoryPaths(cwd string) MemoryPaths {
	homeDir, _ := os.UserHomeDir()
	return MemoryPaths{
		Global: []string{
			filepath.Join(homeDir, ".gen", "GEN.md"),
			filepath.Join(homeDir, ".claude", "CLAUDE.md"),
		},
		GlobalRules: filepath.Join(homeDir, ".gen", "rules"),
		Project: []string{
			filepath.Join(cwd, ".gen", "GEN.md"),
			filepath.Join(cwd, "GEN.md"),
			filepath.Join(cwd, ".claude", "CLAUDE.md"),
			filepath.Join(cwd, "CLAUDE.md"),
		},
		ProjectRules: filepath.Join(cwd, ".gen", "rules"),
		Local: []string{
			filepath.Join(cwd, ".gen", "GEN.local.md"),
		},
	}
}

// FindMemoryFile returns the first existing file path from the given list.
func FindMemoryFile(paths []string) string {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ListRulesFiles returns all .md files in a rules directory.
func ListRulesFiles(rulesDir string) []string {
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			files = append(files, filepath.Join(rulesDir, name))
		}
	}
	sort.Strings(files)
	return files
}

// GetFileSize returns the size of a file in bytes, or 0 if not found.
func GetFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// FormatFileSize formats a file size for display.
func FormatFileSize(size int64) string {
	if size >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
	}
	if size >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(size)/1024)
	}
	return fmt.Sprintf("%dB", size)
}
