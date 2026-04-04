// Package system provides system prompt construction for GenCode.
// It assembles prompts from modular components: base identity, provider-specific
// instructions, and dynamic environment information.
package system

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/client"
	"github.com/yanmxa/gencode/internal/log"
	"go.uber.org/zap"
)

const (
	// maxImportDepth is the maximum recursion depth for @import resolution
	maxImportDepth = 5
)

//go:embed prompts/*.txt
var promptFS embed.FS

// System manages system prompt generation with runtime customization.
type System struct {
	Client              *client.Client // reference for provider name + model
	Cwd                 string
	IsGit               bool
	PlanMode            bool
	UserInstructions    string   // ~/.gen/GEN.md + rules
	ProjectInstructions string   // .gen/GEN.md + rules + local
	SessionSummary      string   // <session-summary> from compaction
	Skills              string   // <available-skills> from active skills
	Agents              string   // <available-agents> from enabled agents
	DeferredTools       string   // <available-deferred-tools> from progressive disclosure
	Extra               []string // ad-hoc content (agent-identity, skill-invocation, etc.)

	cached string
	dirty  bool
}

// Pre-cached embedded prompt files (loaded once at init).
var (
	cachedBase     string
	cachedTools    string
	cachedPlanMode string
)

func init() {
	cachedBase = load("base.txt")
	cachedTools = join([]string{
		load("tools-core.txt"),
		load("tools-git.txt"),
		load("tools-questions.txt"),
		load("tools-tasks.txt"),
	})
	cachedPlanMode = load("planmode.txt")
}

// Prompt builds the complete system prompt (with caching).
func (s *System) Prompt() string {
	if s.cached != "" && !s.dirty {
		return s.cached
	}
	result := s.buildPrompt()
	s.cached = result
	s.dirty = false
	return result
}

// Invalidate marks the cached prompt as stale so the next Prompt() call rebuilds it.
func (s *System) Invalidate() {
	s.dirty = true
}

// buildPrompt assembles the complete system prompt in 7-layer narrative order:
//  1. Identity (base.txt)
//  2. Environment (provider/generic + <env>)
//  3. Instructions (<user-instructions>, <project-instructions>)
//  4. Summary (<session-summary>)
//  5. Capabilities (<available-skills>, <available-agents>)
//  6. Guidelines (tools-*.txt)
//  7. Mode (planmode.txt, only when PlanMode=true)
func (s *System) buildPrompt() string {
	providerName := ""
	modelID := ""
	if s.Client != nil {
		providerName = s.Client.Name()
		modelID = s.Client.ModelID()
	}

	log.Logger().Info("=== System Prompt Loading ===",
		zap.Int("base_len", len(cachedBase)),
		zap.Int("tools_len", len(cachedTools)),
		zap.String("provider", providerName),
		zap.String("model", modelID),
	)

	var parts []string

	// 1. Identity
	parts = append(parts, cachedBase)

	// 2. Environment
	providerPrompt := providerOrGeneric(providerName)
	parts = append(parts, providerPrompt)
	parts = append(parts, s.formatEnv(modelID))

	// 3. Instructions
	if s.UserInstructions != "" {
		parts = append(parts, "<user-instructions>\n"+s.UserInstructions+"\n</user-instructions>")
	}
	if s.ProjectInstructions != "" {
		parts = append(parts, "<project-instructions>\n"+s.ProjectInstructions+"\n</project-instructions>")
	}

	// 4. Summary
	if s.SessionSummary != "" {
		parts = append(parts, s.SessionSummary)
	}

	// 5. Capabilities
	if s.Skills != "" {
		parts = append(parts, s.Skills)
	}
	if s.Agents != "" {
		parts = append(parts, s.Agents)
	}
	if s.DeferredTools != "" {
		parts = append(parts, s.DeferredTools)
	}

	// 6. Guidelines
	parts = append(parts, cachedTools)

	// 7. Mode
	if s.PlanMode {
		parts = append(parts, cachedPlanMode)
	}

	// Extra content (agent-identity, skill-invocation, etc.)
	for _, e := range s.Extra {
		parts = append(parts, e)
	}

	result := join(parts)

	preview := result
	if len(preview) > 100 {
		preview = preview[:100]
	}
	log.Logger().Info("System prompt assembled",
		zap.Int("total_len", len(result)),
		zap.String("first_100", preview),
	)

	return result
}

// formatEnv generates the dynamic environment section.
func (s *System) formatEnv(model string) string {
	gitStatus := "No"
	if s.IsGit {
		gitStatus = "Yes"
	}
	return fmt.Sprintf(`<env>
Working directory: %s
Is git repo: %s
Platform: %s
Date: %s
Model: %s
</env>`, s.Cwd, gitStatus, runtime.GOOS,
		time.Now().Format("2006-01-02"), model)
}

// load reads a prompt file from the embedded filesystem.
func load(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name)
	if err != nil {
		return ""
	}
	return string(data)
}

// providerOrGeneric returns provider-specific prompt if available,
// otherwise falls back to generic.txt.
func providerOrGeneric(provider string) string {
	if provider == "" {
		return load("generic.txt")
	}
	data, err := promptFS.ReadFile("prompts/" + provider + ".txt")
	if err != nil {
		return load("generic.txt")
	}
	return string(data)
}

// join concatenates non-empty parts with double newlines.
func join(parts []string) string {
	var filtered []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, "\n\n")
}

// MemoryFile represents a loaded memory file with metadata.
type MemoryFile struct {
	Path    string // Full path to the file
	Size    int64  // File size in bytes
	Content string // File content
	Level   string // "global", "project", or "local"
	Source  string // "rules" for rules directory files, empty otherwise
}

// LoadInstructions loads user-level and project-level instructions separately.
// Uses LoadMemoryFiles internally and groups by Level.
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

// LoadMemory loads memory content from standard locations (legacy convenience wrapper).
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
	seen := make(map[string]bool) // Track imported files to prevent cycles

	// User level: try GEN.md first, fallback to CLAUDE.md
	userSources := []string{
		filepath.Join(homeDir, ".gen", "GEN.md"),
		filepath.Join(homeDir, ".claude", "CLAUDE.md"),
	}
	if f := loadMemoryFile(userSources, "global", "", seen); f != nil {
		files = append(files, *f)
	}

	// User rules directory
	userRulesDir := filepath.Join(homeDir, ".gen", "rules")
	files = append(files, loadRulesDirectory(userRulesDir, "global", seen)...)

	// Project level: try GEN.md first, fallback to CLAUDE.md
	projectSources := []string{
		filepath.Join(cwd, ".gen", "GEN.md"),
		filepath.Join(cwd, "GEN.md"),
		filepath.Join(cwd, ".claude", "CLAUDE.md"),
		filepath.Join(cwd, "CLAUDE.md"),
	}
	if f := loadMemoryFile(projectSources, "project", "", seen); f != nil {
		files = append(files, *f)
	}

	// Project rules directory
	projectRulesDir := filepath.Join(cwd, ".gen", "rules")
	files = append(files, loadRulesDirectory(projectRulesDir, "project", seen)...)

	// Project local file (not committed to git)
	localSources := []string{
		filepath.Join(cwd, ".gen", "GEN.local.md"),
	}
	if f := loadMemoryFile(localSources, "local", "", seen); f != nil {
		files = append(files, *f)
	}

	return files
}

// loadMemoryFile loads the first existing file from sources with @import resolution.
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
		// Resolve @imports
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

// loadRulesDirectory loads all .md files from a rules directory.
func loadRulesDirectory(dir string, level string, seen map[string]bool) []MemoryFile {
	var files []MemoryFile

	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}

	// Sort entries for consistent ordering
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

// resolveImports processes @import statements in content.
// Syntax: @path/to/file.md or @./relative/path.md
// Max depth is limited to prevent infinite recursion.
func resolveImports(content string, basePath string, depth int, seen map[string]bool) string {
	if depth >= maxImportDepth {
		return content
	}

	// Match @path/to/file or @./relative/path (but not email addresses)
	importRe := regexp.MustCompile(`(?m)^@([^\s@]+\.md)\s*$`)

	return importRe.ReplaceAllStringFunc(content, func(match string) string {
		// Extract path (remove leading @)
		importPath := strings.TrimPrefix(strings.TrimSpace(match), "@")

		// Resolve relative to basePath
		var fullPath string
		if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
			fullPath = filepath.Join(basePath, importPath)
		} else {
			// Absolute from home or project root
			fullPath = filepath.Join(basePath, importPath)
		}

		// Clean the path
		fullPath = filepath.Clean(fullPath)

		// Check for cycles
		if seen[fullPath] {
			return fmt.Sprintf("<!-- Skipped (cycle): @%s -->", importPath)
		}

		// Read the imported file
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

		// Recursively resolve imports in the imported content
		importedContent = resolveImports(importedContent, filepath.Dir(fullPath), depth+1, seen)

		return fmt.Sprintf("<!-- Imported: %s -->\n%s", importPath, importedContent)
	})
}

// CompactPrompt returns the prompt for conversation compaction.
func CompactPrompt() string {
	return load("compact.txt")
}

// MemoryPaths holds categorized memory file paths.
type MemoryPaths struct {
	Global       []string // User-level memory files
	GlobalRules  string   // User-level rules directory
	Project      []string // Project-level memory files
	ProjectRules string   // Project-level rules directory
	Local        []string // Local memory files (not committed)
}

// GetMemoryPaths returns the search paths for memory files.
// Returns user-level paths and project-level paths separately (legacy compatibility).
func GetMemoryPaths(cwd string) (userPaths, projectPaths []string) {
	paths := GetAllMemoryPaths(cwd)
	return paths.Global, paths.Project
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
// Returns empty string if no file exists.
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
