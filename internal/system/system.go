// Package system provides system prompt construction for GenCode.
// It assembles prompts from modular components: base identity, provider-specific
// instructions, and dynamic environment information.
package system

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/log"
	"go.uber.org/zap"
)

//go:embed prompts/*.txt
var promptFS embed.FS

// Config holds configuration for system prompt generation.
type Config struct {
	Provider string // Provider name: anthropic, openai, google
	Model    string // Model identifier
	Cwd      string // Current working directory
	IsGit    bool   // Whether cwd is a git repository

	// Extension points (reserved for future use)
	Memory   string   // CLAUDE.md or similar memory content
	PlanMode bool     // Whether in plan mode
	Extra    []string // Additional prompt sections
}

// Prompt builds the complete system prompt.
// Assembly order: base + tools + provider/generic + environment
func Prompt(cfg Config) string {
	base := load("base.txt")
	tools := load("tools.txt")
	providerPrompt := providerOrGeneric(cfg.Provider)
	env := formatEnv(cfg)

	// DEBUG: Verify each part is loaded correctly
	log.Logger().Info("=== System Prompt Loading ===",
		zap.Int("base_len", len(base)),
		zap.Int("tools_len", len(tools)),
		zap.Int("provider_len", len(providerPrompt)),
		zap.Int("env_len", len(env)),
		zap.String("provider", cfg.Provider),
		zap.String("model", cfg.Model),
	)

	if len(base) == 0 {
		log.Logger().Warn("WARNING: base.txt is empty!")
	}
	if len(tools) == 0 {
		log.Logger().Warn("WARNING: tools.txt is empty!")
	}
	if len(providerPrompt) == 0 {
		log.Logger().Warn("WARNING: provider/generic prompt is empty!")
	}

	parts := []string{base, tools, providerPrompt, env}

	// Plan mode: add plan mode instructions
	if cfg.PlanMode {
		planPrompt := load("planmode.txt")
		if planPrompt != "" {
			parts = append(parts, planPrompt)
		}
	}

	// Extension points
	if cfg.Memory != "" {
		parts = append(parts, formatMemory(cfg.Memory))
	}
	for _, e := range cfg.Extra {
		parts = append(parts, e)
	}

	result := join(parts)

	// Log final assembled prompt info
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

// formatEnv generates the dynamic environment section.
func formatEnv(cfg Config) string {
	gitStatus := "No"
	if cfg.IsGit {
		gitStatus = "Yes"
	}
	return fmt.Sprintf(`<env>
Working directory: %s
Is git repo: %s
Platform: %s
Date: %s
Model: %s
</env>`, cfg.Cwd, gitStatus, runtime.GOOS,
		time.Now().Format("2006-01-02"), cfg.Model)
}

// formatMemory wraps memory content in XML tags.
func formatMemory(m string) string {
	return "<memory>\n" + m + "\n</memory>"
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

// LoadMemory loads memory content from standard locations.
// Priority: GEN.md files first, falling back to CLAUDE.md if not found.
//
// User level (first found wins):
//  1. ~/.gen/GEN.md (preferred)
//  2. ~/.claude/CLAUDE.md (fallback)
//
// Project level (first found wins):
//  1. .gen/GEN.md or GEN.md (preferred)
//  2. .claude/CLAUDE.md or CLAUDE.md (fallback)
//
// Both user and project level content are concatenated.
func LoadMemory(cwd string) string {
	var parts []string
	homeDir, _ := os.UserHomeDir()

	// User level: try GEN.md first, fallback to CLAUDE.md
	userSources := []string{
		filepath.Join(homeDir, ".gen", "GEN.md"),
		filepath.Join(homeDir, ".claude", "CLAUDE.md"),
	}
	if content := loadFirstFound(userSources); content != "" {
		parts = append(parts, content)
	}

	// Project level: try GEN.md first, fallback to CLAUDE.md
	projectSources := []string{
		filepath.Join(cwd, ".gen", "GEN.md"),
		filepath.Join(cwd, "GEN.md"),
		filepath.Join(cwd, ".claude", "CLAUDE.md"),
		filepath.Join(cwd, "CLAUDE.md"),
	}
	if content := loadFirstFound(projectSources); content != "" {
		parts = append(parts, content)
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}

// CompactPrompt returns the prompt for conversation compaction.
func CompactPrompt() string {
	return load("compact.txt")
}

// loadFirstFound tries to load content from the first file that exists.
func loadFirstFound(sources []string) string {
	for _, src := range sources {
		if data, err := os.ReadFile(src); err == nil {
			content := strings.TrimSpace(string(data))
			if content != "" {
				log.Logger().Info("Loaded memory file",
					zap.String("path", src),
					zap.Int("bytes", len(content)))
				return fmt.Sprintf("<!-- Source: %s -->\n%s", src, content)
			}
		}
	}
	return ""
}

// GetMemoryPaths returns the search paths for memory files.
// Returns user-level paths and project-level paths separately.
func GetMemoryPaths(cwd string) (userPaths, projectPaths []string) {
	homeDir, _ := os.UserHomeDir()

	userPaths = []string{
		filepath.Join(homeDir, ".gen", "GEN.md"),
		filepath.Join(homeDir, ".claude", "CLAUDE.md"),
	}

	projectPaths = []string{
		filepath.Join(cwd, ".gen", "GEN.md"),
		filepath.Join(cwd, "GEN.md"),
		filepath.Join(cwd, ".claude", "CLAUDE.md"),
		filepath.Join(cwd, "CLAUDE.md"),
	}

	return userPaths, projectPaths
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
