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
	PlanMode            bool
	UserInstructions    string
	ProjectInstructions string
	SessionSummary      string
	Skills              string
	Agents              string
	DeferredTools       string
	Extra               []ExtraLayer
}

// Build creates a core.System with properly separated layers.
func Build(cfg Config) core.System {
	sys := core.NewSystem()

	sys.Set(core.Layer{
		Name: "identity", Priority: 0,
		Content: cachedBase, Source: core.Predefined,
	})

	sys.Set(core.Layer{
		Name: "environment-provider", Priority: 100,
		Content: providerOrGeneric(cfg.ProviderName), Source: core.Predefined,
	})

	sys.Set(core.Layer{
		Name: "environment-runtime", Priority: 110,
		Content: formatEnvStatic(cfg.Cwd, cfg.IsGit, cfg.ModelID), Source: core.Dynamic,
	})

	if cfg.UserInstructions != "" {
		sys.Set(core.Layer{
			Name: "user-instructions", Priority: 200,
			Content: "<user-instructions>\n" + cfg.UserInstructions + "\n</user-instructions>",
			Source:  core.FromFile,
		})
	}

	if cfg.ProjectInstructions != "" {
		sys.Set(core.Layer{
			Name: "project-instructions", Priority: 210,
			Content: "<project-instructions>\n" + cfg.ProjectInstructions + "\n</project-instructions>",
			Source:  core.FromFile,
		})
	}

	if cfg.SessionSummary != "" {
		sys.Set(core.Layer{
			Name: "session-summary", Priority: 300,
			Content: cfg.SessionSummary, Source: core.Dynamic,
		})
	}

	if cfg.Skills != "" {
		sys.Set(core.Layer{
			Name: "capabilities-skills", Priority: 400,
			Content: cfg.Skills, Source: core.FromFile,
		})
	}

	if cfg.Agents != "" {
		sys.Set(core.Layer{
			Name: "capabilities-agents", Priority: 410,
			Content: cfg.Agents, Source: core.FromFile,
		})
	}

	if cfg.DeferredTools != "" {
		sys.Set(core.Layer{
			Name: "capabilities-deferred-tools", Priority: 420,
			Content: cfg.DeferredTools, Source: core.FromFile,
		})
	}

	guidelines := []string{cachedToolsCore}
	if cfg.IsGit {
		guidelines = append(guidelines, cachedToolsGit)
	}
	guidelines = append(guidelines, cachedToolsQuestions, cachedToolsTasks)
	sys.Set(core.Layer{
		Name: "guidelines", Priority: 500,
		Content: join(guidelines), Source: core.Predefined,
	})

	if cfg.PlanMode {
		sys.Set(core.Layer{
			Name: "mode-plan", Priority: 600,
			Content: cachedPlanMode, Source: core.Predefined,
		})
	}

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
