// Package system builds system prompts for GenCode agents.
// It assembles prompts from embedded templates, runtime environment,
// user/project instructions, and dynamic capabilities.
package system

import (
	"embed"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/yanmxa/gencode/internal/core"
)

//go:embed prompts/*.txt
var promptFS embed.FS

// ExtraLayer is a named piece of extra system prompt content (priority 700+).
type ExtraLayer struct {
	Name    string
	Content string
}

var (
	cachedBase           string
	cachedToolsCore      string
	cachedToolsGit       string
	cachedToolsQuestions string
	cachedToolsTasks     string
	cachedPlanMode       string
	cachedCoordinator    string
)

func init() {
	cachedBase = load("base.txt")
	cachedToolsCore = load("tools-core.txt")
	cachedToolsGit = load("tools-git.txt")
	cachedToolsQuestions = load("tools-questions.txt")
	cachedToolsTasks = load("tools-tasks.txt")
	cachedPlanMode = load("planmode.txt")
	cachedCoordinator = load("coordinator.txt")
}

// CoordinatorGuidance returns the coordinator prompt for the main session.
func CoordinatorGuidance() string {
	return cachedCoordinator
}

// Config holds all inputs needed to build a layered core.System.
type Config struct {
	ProviderName        string
	ModelID             string
	Cwd                 string
	IsGit               bool
	IsSubagent          bool
	PlanMode            bool
	UserInstructions    string
	ProjectInstructions string
	Skills              string
	Agents              string
	DeferredTools       string
	Extra               []ExtraLayer
}

// Build creates a core.System with properly separated layers.
//
// Layer structure (7 layers max):
//
//	identity        (0)   — base.txt (who you are, how you behave)
//	provider        (100) — provider-specific overrides (optional, only if file exists)
//	environment     (110) — cwd, git, platform, model
//	instructions    (200) — user + project instructions
//	capabilities    (400) — skills, agents, deferred tools
//	guidelines      (500) — tool usage, git safety
//	mode            (600) — plan mode
//	extra-*         (700) — coordinator, skill invocation, agent identity
func Build(cfg Config) core.System {
	sys := core.NewSystem()

	// Identity — base behavior and conventions
	sys.Set(core.Layer{
		Name: "identity", Priority: 0,
		Content: cachedBase, Source: core.Predefined,
	})

	// Provider-specific overrides (only if a file like prompts/anthropic.txt exists)
	if p := loadProvider(cfg.ProviderName); p != "" {
		sys.Set(core.Layer{
			Name: "provider", Priority: 100,
			Content: p, Source: core.Predefined,
		})
	}

	// Runtime environment
	sys.Set(core.Layer{
		Name: "environment", Priority: 110,
		Content: formatEnvStatic(cfg.Cwd, cfg.IsGit, cfg.ModelID), Source: core.Dynamic,
	})

	// Instructions — user-level + project-level merged into one layer
	if instr := formatInstructions(cfg.UserInstructions, cfg.ProjectInstructions); instr != "" {
		sys.Set(core.Layer{
			Name: "instructions", Priority: 200,
			Content: instr, Source: core.FromFile,
		})
	}

	// Capabilities — skills, agents, deferred tools merged into one layer
	if caps := join([]string{cfg.Skills, cfg.Agents, cfg.DeferredTools}); caps != "" {
		sys.Set(core.Layer{
			Name: "capabilities", Priority: 400,
			Content: caps, Source: core.FromFile,
		})
	}

	// Tool guidelines — conditional on context
	guidelines := []string{cachedToolsCore}
	if cfg.IsGit {
		guidelines = append(guidelines, cachedToolsGit)
	}
	if !cfg.IsSubagent {
		guidelines = append(guidelines, cachedToolsQuestions)
	}
	if !cfg.IsSubagent && !cfg.PlanMode {
		guidelines = append(guidelines, cachedToolsTasks)
	}
	sys.Set(core.Layer{
		Name: "guidelines", Priority: 500,
		Content: join(guidelines), Source: core.Predefined,
	})

	// Plan mode
	if cfg.PlanMode {
		sys.Set(core.Layer{
			Name: "mode", Priority: 600,
			Content: cachedPlanMode, Source: core.Predefined,
		})
	}

	// Extra layers — coordinator guidance, skill invocation, agent identity
	for i, extra := range cfg.Extra {
		if strings.TrimSpace(extra.Content) != "" {
			name := extra.Name
			if name == "" {
				name = fmt.Sprintf("extra-%d", i)
			}
			sys.Set(core.Layer{
				Name:     name,
				Priority: 700 + i,
				Content:  extra.Content, Source: core.Injected,
			})
		}
	}

	return sys
}

func formatEnvStatic(cwd string, isGit bool, model string) string {
	gitStatus := "No"
	if isGit {
		gitStatus = "Yes"
	}
	today := time.Now().Format("2006-01-02")
	return fmt.Sprintf("# currentDate\nToday's date is %s.\n\n<env>\nSession working directory: %s\nIs git repo: %s\nPlatform: %s/%s\nModel: %s\n</env>",
		today, cwd, gitStatus, runtime.GOOS, runtime.GOARCH, model)
}

func load(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name)
	if err != nil {
		return ""
	}
	return string(data)
}

// loadProvider returns provider-specific prompt content, or "" if none exists.
func loadProvider(provider string) string {
	if provider == "" {
		return ""
	}
	data, err := promptFS.ReadFile("prompts/" + provider + ".txt")
	if err != nil {
		return ""
	}
	return string(data)
}

// formatInstructions merges user and project instructions into one block.
func formatInstructions(user, project string) string {
	var parts []string
	if user != "" {
		parts = append(parts, "<user-instructions>\n"+user+"\n</user-instructions>")
	}
	if project != "" {
		parts = append(parts, "<project-instructions>\n"+project+"\n</project-instructions>")
	}
	return strings.Join(parts, "\n\n")
}

func join(parts []string) string {
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, "\n\n")
}

// CompactPrompt returns the prompt for conversation compaction.
func CompactPrompt() string {
	return load("compact.txt")
}
