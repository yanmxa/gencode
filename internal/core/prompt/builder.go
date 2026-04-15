// Package prompt builds system prompts for GenCode agents.
// It assembles prompts from embedded templates, runtime environment,
// user/project instructions, and dynamic capabilities.
package prompt

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
	Extra               []string
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

	sys.Set(core.Layer{
		Name: "guidelines", Priority: 500,
		Content: cachedTools, Source: core.Predefined,
	})

	if cfg.PlanMode {
		sys.Set(core.Layer{
			Name: "mode-plan", Priority: 600,
			Content: cachedPlanMode, Source: core.Predefined,
		})
	}

	for i, extra := range cfg.Extra {
		if strings.TrimSpace(extra) != "" {
			sys.Set(core.Layer{
				Name:     fmt.Sprintf("extra-%d", i),
				Priority: 700 + i,
				Content:  extra, Source: core.Injected,
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
	return fmt.Sprintf("# currentDate\nToday's date is %s.\n\n<env>\nSession working directory: %s\nIs git repo: %s\nPlatform: %s\nModel: %s\n</env>",
		today, cwd, gitStatus, runtime.GOOS, model)
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
	var filtered []string
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
