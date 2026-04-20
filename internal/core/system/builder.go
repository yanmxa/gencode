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

// ExtraLayer is a named piece of content appended after standard layers.
type ExtraLayer struct {
	Name    string
	Content string
}

var (
	cachedBase      string
	cachedToolsCore string
	cachedToolsGit  string
	cachedToolsQA   string
	cachedToolsTask string
	cachedCompact   string
)

func init() {
	cachedBase = load("base.txt")
	cachedToolsCore = load("tools-core.txt")
	cachedToolsGit = load("tools-git.txt")
	cachedToolsQA = load("tools-questions.txt")
	cachedToolsTask = load("tools-tasks.txt")
	cachedCompact = load("compact.txt")
}

// Config holds inputs for building a system prompt.
type Config struct {
	ProviderName        string
	ModelID             string
	Cwd                 string
	IsGit               bool
	IsSubagent          bool
	UserInstructions    string
	ProjectInstructions string
	Skills              string
	Agents              string
	DeferredTools       string
	Extra               []ExtraLayer
}

// Build assembles a layered system prompt.
// Layer priorities determine render order — see core.Layer for the scheme.
func Build(cfg Config) core.System {
	sys := core.NewSystem()

	sys.Set(core.Layer{
		Name: "identity", Priority: 0,
		Content: cachedBase, Source: core.Predefined,
	})

	if p := loadProvider(cfg.ProviderName); p != "" {
		sys.Set(core.Layer{
			Name: "provider", Priority: 100,
			Content: p, Source: core.Predefined,
		})
	}

	sys.Set(core.Layer{
		Name: "environment", Priority: 110,
		Content: formatEnv(cfg.Cwd, cfg.IsGit, cfg.ModelID), Source: core.Dynamic,
	})

	if instr := mergeInstructions(cfg.UserInstructions, cfg.ProjectInstructions); instr != "" {
		sys.Set(core.Layer{
			Name: "instructions", Priority: 200,
			Content: instr, Source: core.FromFile,
		})
	}

	if caps := joinNonEmpty(cfg.Skills, cfg.Agents, cfg.DeferredTools); caps != "" {
		sys.Set(core.Layer{
			Name: "capabilities", Priority: 400,
			Content: caps, Source: core.FromFile,
		})
	}

	sys.Set(core.Layer{
		Name: "guidelines", Priority: 500,
		Content: guidelines(cfg.IsGit, cfg.IsSubagent), Source: core.Predefined,
	})

	for i, extra := range cfg.Extra {
		if strings.TrimSpace(extra.Content) == "" {
			continue
		}
		name := extra.Name
		if name == "" {
			name = fmt.Sprintf("extra-%d", i)
		}
		sys.Set(core.Layer{
			Name: name, Priority: 700 + i,
			Content: extra.Content, Source: core.Injected,
		})
	}

	return sys
}

func guidelines(isGit, isSubagent bool) string {
	parts := []string{cachedToolsCore}
	if isGit {
		parts = append(parts, cachedToolsGit)
	}
	if !isSubagent {
		parts = append(parts, cachedToolsQA, cachedToolsTask)
	}
	return joinNonEmpty(parts...)
}

func formatEnv(cwd string, isGit bool, model string) string {
	git := "No"
	if isGit {
		git = "Yes"
	}
	return fmt.Sprintf(
		"# currentDate\nToday's date is %s.\n\n<env>\nSession working directory: %s\nIs git repo: %s\nPlatform: %s/%s\nModel: %s\n</env>",
		time.Now().Format("2006-01-02"), cwd, git, runtime.GOOS, runtime.GOARCH, model,
	)
}

func load(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name)
	if err != nil {
		return ""
	}
	return string(data)
}

func loadProvider(name string) string {
	if name == "" {
		return ""
	}
	data, err := promptFS.ReadFile("prompts/" + name + ".txt")
	if err != nil {
		return ""
	}
	return string(data)
}

func mergeInstructions(user, project string) string {
	var parts []string
	if user != "" {
		parts = append(parts, "<user-instructions>\n"+user+"\n</user-instructions>")
	}
	if project != "" {
		parts = append(parts, "<project-instructions>\n"+project+"\n</project-instructions>")
	}
	return strings.Join(parts, "\n\n")
}

func joinNonEmpty(parts ...string) string {
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
	return cachedCompact
}
