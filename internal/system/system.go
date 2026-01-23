// Package system provides system prompt construction for GenCode.
// It assembles prompts from modular components: base identity, provider-specific
// instructions, and dynamic environment information.
package system

import (
	"embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/myan/gencode/internal/log"
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
// Assembly order: base + provider/generic + environment
func Prompt(cfg Config) string {
	base := load("base.txt")
	providerPrompt := providerOrGeneric(cfg.Provider)
	env := formatEnv(cfg)

	// DEBUG: Verify each part is loaded correctly
	log.Logger().Info("=== System Prompt Loading ===",
		zap.Int("base_len", len(base)),
		zap.Int("provider_len", len(providerPrompt)),
		zap.Int("env_len", len(env)),
		zap.String("provider", cfg.Provider),
		zap.String("model", cfg.Model),
	)

	if len(base) == 0 {
		log.Logger().Warn("WARNING: base.txt is empty!")
	}
	if len(providerPrompt) == 0 {
		log.Logger().Warn("WARNING: provider/generic prompt is empty!")
	}

	parts := []string{base, providerPrompt, env}

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
