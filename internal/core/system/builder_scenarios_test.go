package system

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files in testdata/")

type scenario struct {
	name   string
	config Config
}

func testScenarios() []scenario {
	return []scenario{
		{
			name: "minimal",
			config: Config{
				Cwd:   "/tmp/project",
				IsGit: false,
			},
		},
		{
			name: "main_session",
			config: Config{
				ProviderName:        "anthropic",
				ModelID:             "claude-sonnet-4-20250514",
				Cwd:                 "/home/user/myproject",
				IsGit:               true,
				UserInstructions:    "Always use tabs for indentation.\nPrefer short variable names.",
				ProjectInstructions: "This is a Go project using Bubble Tea.\nRun tests with: go test ./...",
				Skills:              "<available-skills>\nUse the Skill tool to invoke these capabilities:\n\n- git: Git workflow automation\n- review: Review a pull request\n- init: Initialize a new CLAUDE.md file\n\nInvoke with: Skill(skill=\"name\", args=\"optional args\")\n</available-skills>",
				Agents:              "<available-agents>\nAvailable agent types for the Agent tool:\n\n- Explore: Fast codebase exploration\n  Use when: need to find files, search code, answer questions about codebase\n  Tools: Read, Glob, Grep\n- Plan: Software architect for implementation plans\n  Use when: need to plan implementation strategy\n  Tools: Read, Glob, Grep, Agent\n- general-purpose: All tools including nested Agent\n  Tools: *\n</available-agents>",
				DeferredTools:       "<available-deferred-tools>\n- CronCreate: Schedule a prompt to run on a cron schedule\n- CronDelete: Delete a scheduled cron prompt\n- CronList: List all scheduled cron prompts\n- EnterWorktree: Create an isolated git worktree for agent work\n- ExitWorktree: Leave and clean up a git worktree\n</available-deferred-tools>",
				Extra: []ExtraLayer{
					{Name: "coordinator", Content: CoordinatorGuidance()},
				},
			},
		},
		{
			name: "no_git",
			config: Config{
				ProviderName:        "anthropic",
				ModelID:             "claude-sonnet-4-20250514",
				Cwd:                 "/home/user/myproject",
				IsGit:               false,
				UserInstructions:    "Always use tabs for indentation.",
				ProjectInstructions: "This is a Go project.",
				Skills:              "<available-skills>\nUse the Skill tool to invoke these capabilities:\n\n- review: Review a pull request\n\nInvoke with: Skill(skill=\"name\", args=\"optional args\")\n</available-skills>",
				Agents:              "<available-agents>\nAvailable agent types for the Agent tool:\n\n- Explore: Fast codebase exploration\n  Tools: Read, Glob, Grep\n</available-agents>",
			},
		},
		{
			name: "plan_mode",
			config: Config{
				ProviderName:        "anthropic",
				ModelID:             "claude-sonnet-4-20250514",
				Cwd:                 "/home/user/myproject",
				IsGit:               true,
				PlanMode:            true,
				UserInstructions:    "Always use tabs.",
				ProjectInstructions: "Go project with Bubble Tea.",
			},
		},
		{
			name: "subagent_readonly",
			config: Config{
				ProviderName:        "anthropic",
				ModelID:             "claude-sonnet-4-20250514",
				Cwd:                 "/home/user/myproject",
				IsGit:               true,
				IsSubagent:          true,
				UserInstructions:    "Short variable names.",
				ProjectInstructions: "Go project.",
				Extra: []ExtraLayer{
					{Name: "agent-identity", Content: "## Agent Type: Explore\nFast agent specialized for exploring codebases.\n\n## Mode: Read-Only\nYou are in read-only mode. You can only use tools that read information (Read, Glob, Grep, WebFetch, WebSearch). Do not attempt to modify any files.\n\n## Guidelines\n- Focus on completing your assigned task efficiently\n- Return a clear summary when your task is complete\n- If you encounter errors, report them clearly\n"},
				},
			},
		},
		{
			name: "subagent_general",
			config: Config{
				ProviderName:        "anthropic",
				ModelID:             "claude-sonnet-4-20250514",
				Cwd:                 "/home/user/myproject",
				IsGit:               true,
				IsSubagent:          true,
				UserInstructions:    "Short variable names.",
				ProjectInstructions: "Go project.",
				Skills:              "<available-skills>\nUse the Skill tool to invoke these capabilities:\n\n- git: Git workflow automation\n\nInvoke with: Skill(skill=\"name\", args=\"optional args\")\n</available-skills>",
				Agents:              "<available-agents>\nAvailable agent types for the Agent tool:\n\n- Explore: Fast codebase exploration\n  Tools: Read, Glob, Grep\n</available-agents>",
				Extra: []ExtraLayer{
					{Name: "agent-identity", Content: "## Agent Type: general-purpose\nGeneral-purpose agent for researching complex questions and executing multi-step tasks.\n\n## Mode: Autonomous\nYou have full autonomy to complete your task.\n\n## Guidelines\n- Focus on completing your assigned task efficiently\n- Return a clear summary when your task is complete\n- If you encounter errors, report them clearly\n"},
				},
			},
		},
	}
}

// normalizePrompt replaces dynamic content (date, platform) with stable placeholders.
func normalizePrompt(s string) string {
	// Replace date like 2026-04-19 with placeholder
	dateRe := regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	s = dateRe.ReplaceAllString(s, "YYYY-MM-DD")

	// Replace platform like darwin/arm64 with placeholder
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	s = strings.ReplaceAll(s, platform, "PLATFORM/ARCH")

	return s
}

func TestUpdateGoldenFiles(t *testing.T) {
	if !*update {
		t.Skip("use -update to regenerate golden files")
	}

	for _, sc := range testScenarios() {
		sys := Build(sc.config)
		prompt := normalizePrompt(sys.Prompt())

		path := filepath.Join("testdata", sc.name+".txt")
		if err := os.WriteFile(path, []byte(prompt), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s (%d bytes)", path, len(prompt))
	}
}

func TestGoldenFiles(t *testing.T) {
	for _, sc := range testScenarios() {
		t.Run(sc.name, func(t *testing.T) {
			path := filepath.Join("testdata", sc.name+".txt")
			golden, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden file %s: %v (run with -update to generate)", path, err)
			}

			sys := Build(sc.config)
			got := normalizePrompt(sys.Prompt())

			if got != string(golden) {
				t.Errorf("prompt mismatch for scenario %q (run with -update to regenerate)\n\ngot length:  %d\nwant length: %d",
					sc.name, len(got), len(golden))
			}
		})
	}
}

// --- Section presence/absence integration tests ---

func TestScenarioMinimal_NoGitGuidelines(t *testing.T) {
	sys := Build(Config{Cwd: "/tmp/project", IsGit: false})
	prompt := sys.Prompt()

	if strings.Contains(prompt, "Git safety (Bash)") {
		t.Error("non-git scenario should NOT contain git safety guidelines")
	}
	if !strings.Contains(prompt, "# Tool usage") {
		t.Error("should always contain core tool guidelines")
	}
	if strings.Contains(prompt, "<user-instructions>") {
		t.Error("should NOT contain user-instructions when empty")
	}
	if strings.Contains(prompt, "<project-instructions>") {
		t.Error("should NOT contain project-instructions when empty")
	}
	if strings.Contains(prompt, "<available-skills>") {
		t.Error("should NOT contain skills when empty")
	}
	if strings.Contains(prompt, "<available-agents>") {
		t.Error("should NOT contain agents when empty")
	}
	if strings.Contains(prompt, "<coordinator-guidance>") {
		t.Error("should NOT contain coordinator guidance when not injected")
	}
	if strings.Contains(prompt, "Plan Mode") {
		t.Error("should NOT contain plan mode when disabled")
	}
}

func TestScenarioMainSession_HasAllSections(t *testing.T) {
	scenarios := testScenarios()
	var cfg Config
	for _, sc := range scenarios {
		if sc.name == "main_session" {
			cfg = sc.config
			break
		}
	}

	sys := Build(cfg)
	prompt := sys.Prompt()

	required := []struct {
		label   string
		content string
	}{
		{"identity", "interactive AI assistant"},
		{"environment", "<env>"},
		{"git status", "Is git repo: Yes"},
		{"platform", "/"},
		{"user instructions", "<user-instructions>"},
		{"project instructions", "<project-instructions>"},
		{"skills", "<available-skills>"},
		{"agents", "<available-agents>"},
		{"deferred tools", "<available-deferred-tools>"},
		{"core tool guidelines", "# Tool usage"},
		{"git guidelines", "Git safety (Bash)"},
		{"question guidelines", "AskUserQuestion"},
		{"task guidelines", "TaskCreate"},
		{"coordinator", "<coordinator-guidance>"},
	}

	for _, r := range required {
		if !strings.Contains(prompt, r.content) {
			t.Errorf("main session should contain %s (%q)", r.label, r.content)
		}
	}
}

func TestScenarioNoGit_ExcludesGitGuidelines(t *testing.T) {
	scenarios := testScenarios()
	var cfg Config
	for _, sc := range scenarios {
		if sc.name == "no_git" {
			cfg = sc.config
			break
		}
	}

	sys := Build(cfg)
	prompt := sys.Prompt()

	if strings.Contains(prompt, "Git safety (Bash)") {
		t.Error("no-git scenario should NOT contain git safety guidelines")
	}
	if !strings.Contains(prompt, "Is git repo: No") {
		t.Error("should show git repo as No")
	}
	if !strings.Contains(prompt, "# Tool usage") {
		t.Error("should still contain core tool guidelines")
	}
}

func TestScenarioPlanMode_HasPlanContent(t *testing.T) {
	scenarios := testScenarios()
	var cfg Config
	for _, sc := range scenarios {
		if sc.name == "plan_mode" {
			cfg = sc.config
			break
		}
	}

	sys := Build(cfg)
	prompt := sys.Prompt()

	if !strings.Contains(prompt, "PLAN MODE") {
		t.Error("plan mode scenario should contain PLAN MODE marker")
	}
	if !strings.Contains(prompt, "ExitPlanMode") {
		t.Error("plan mode scenario should reference ExitPlanMode")
	}
	if strings.Contains(prompt, "TaskCreate") {
		t.Error("plan mode should NOT have task management guidelines")
	}
}

func TestScenarioSubagentReadonly_NoCapabilities(t *testing.T) {
	scenarios := testScenarios()
	var cfg Config
	for _, sc := range scenarios {
		if sc.name == "subagent_readonly" {
			cfg = sc.config
			break
		}
	}

	sys := Build(cfg)
	prompt := sys.Prompt()

	if !strings.Contains(prompt, "Agent Type: Explore") {
		t.Error("should contain agent type header")
	}
	if !strings.Contains(prompt, "Read-Only") {
		t.Error("should contain read-only mode")
	}
	if strings.Contains(prompt, "<available-skills>") {
		t.Error("read-only subagent should NOT have skills section")
	}
	if strings.Contains(prompt, "<available-agents>") {
		t.Error("read-only subagent should NOT have agents section")
	}
	if strings.Contains(prompt, "<coordinator-guidance>") {
		t.Error("subagent should NOT have coordinator guidance")
	}
	if strings.Contains(prompt, "AskUserQuestion") {
		t.Error("subagent should NOT have question guidelines")
	}
	if strings.Contains(prompt, "TaskCreate") {
		t.Error("subagent should NOT have task management guidelines")
	}
}

func TestScenarioSubagentGeneral_HasCapabilities(t *testing.T) {
	scenarios := testScenarios()
	var cfg Config
	for _, sc := range scenarios {
		if sc.name == "subagent_general" {
			cfg = sc.config
			break
		}
	}

	sys := Build(cfg)
	prompt := sys.Prompt()

	if !strings.Contains(prompt, "Agent Type: general-purpose") {
		t.Error("should contain agent type header")
	}
	if !strings.Contains(prompt, "Autonomous") {
		t.Error("should contain autonomous mode")
	}
	if !strings.Contains(prompt, "<available-skills>") {
		t.Error("general-purpose subagent should have skills section")
	}
	if !strings.Contains(prompt, "<available-agents>") {
		t.Error("general-purpose subagent should have agents section")
	}
}

func TestLayerOrdering(t *testing.T) {
	sys := Build(Config{
		Cwd:                 "/tmp/test",
		IsGit:               true,
		UserInstructions:    "USER_MARKER",
		ProjectInstructions: "PROJECT_MARKER",
		Skills:              "SKILLS_MARKER",
		Agents:              "AGENTS_MARKER",
		DeferredTools:       "DEFERRED_MARKER",
		Extra:               []ExtraLayer{{Name: "coord", Content: "COORDINATOR_MARKER"}},
	})

	prompt := sys.Prompt()

	indices := map[string]int{
		"identity":     strings.Index(prompt, "interactive AI assistant"),
		"env":          strings.Index(prompt, "<env>"),
		"user":         strings.Index(prompt, "USER_MARKER"),
		"project":      strings.Index(prompt, "PROJECT_MARKER"),
		"skills":       strings.Index(prompt, "SKILLS_MARKER"),
		"agents":       strings.Index(prompt, "AGENTS_MARKER"),
		"deferred":     strings.Index(prompt, "DEFERRED_MARKER"),
		"guidelines":   strings.Index(prompt, "# Tool usage"),
		"coordinator":  strings.Index(prompt, "COORDINATOR_MARKER"),
	}

	for name, idx := range indices {
		if idx < 0 {
			t.Fatalf("section %q not found in prompt", name)
		}
	}

	order := []string{"identity", "env", "user", "project", "skills", "agents", "deferred", "guidelines", "coordinator"}
	for i := 1; i < len(order); i++ {
		if indices[order[i-1]] >= indices[order[i]] {
			t.Errorf("%s (idx=%d) should appear before %s (idx=%d)", order[i-1], indices[order[i-1]], order[i], indices[order[i]])
		}
	}
}

func TestExtraLayerSemanticNaming(t *testing.T) {
	sys := Build(Config{
		Cwd: "/tmp/test",
		Extra: []ExtraLayer{
			{Name: "coordinator", Content: "COORD_CONTENT"},
			{Name: "skill-invocation", Content: "SKILL_CONTENT"},
		},
	})

	// Verify layers can be retrieved by semantic name
	if layer, ok := sys.Get("coordinator"); !ok {
		t.Error("should be able to get layer by semantic name 'coordinator'")
	} else if layer.Content != "COORD_CONTENT" {
		t.Errorf("coordinator content = %q, want COORD_CONTENT", layer.Content)
	}

	if layer, ok := sys.Get("skill-invocation"); !ok {
		t.Error("should be able to get layer by semantic name 'skill-invocation'")
	} else if layer.Content != "SKILL_CONTENT" {
		t.Errorf("skill-invocation content = %q, want SKILL_CONTENT", layer.Content)
	}
}

func TestConditionalGitGuidelines(t *testing.T) {
	withGit := Build(Config{Cwd: "/tmp/test", IsGit: true})
	withoutGit := Build(Config{Cwd: "/tmp/test", IsGit: false})

	promptWithGit := withGit.Prompt()
	promptWithoutGit := withoutGit.Prompt()

	if !strings.Contains(promptWithGit, "Git safety (Bash)") {
		t.Error("IsGit=true should include git safety guidelines")
	}
	if strings.Contains(promptWithoutGit, "Git safety (Bash)") {
		t.Error("IsGit=false should NOT include git safety guidelines")
	}

	if len(promptWithGit) <= len(promptWithoutGit) {
		t.Error("git prompt should be longer than non-git prompt")
	}
}

func TestDeferredToolsWithDescriptions(t *testing.T) {
	deferred := "<available-deferred-tools>\n- CronCreate: Schedule a prompt\n- EnterWorktree: Create worktree\n</available-deferred-tools>"
	sys := Build(Config{
		Cwd:           "/tmp/test",
		DeferredTools: deferred,
	})
	prompt := sys.Prompt()

	if !strings.Contains(prompt, "CronCreate: Schedule a prompt") {
		t.Error("deferred tools should include descriptions")
	}
}
