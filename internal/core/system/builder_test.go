package system

import (
	"strings"
	"testing"
)

func TestFormatEnvIncludesSessionWorkingDirectoryFacts(t *testing.T) {
	env := formatEnvStatic("/tmp/project", true, "test-model")
	if !strings.Contains(env, "Session working directory: /tmp/project") {
		t.Fatalf("formatEnvStatic() = %q, want session working directory", env)
	}
	if !strings.Contains(env, "Is git repo: Yes") {
		t.Fatalf("formatEnvStatic() = %q, want git status", env)
	}
	if strings.Contains(env, "Bash commands start in the session working directory.") {
		t.Fatalf("formatEnvStatic() = %q, should keep tool guidance out of env", env)
	}
}

func TestBuildPromptCaching(t *testing.T) {
	sys := Build(Config{
		Cwd:   "/tmp/test",
		IsGit: true,
	})

	first := sys.Prompt()
	if first == "" {
		t.Error("First Prompt() call should return non-empty string")
	}

	second := sys.Prompt()
	if first != second {
		t.Error("Second Prompt() call should return same cached result")
	}

	sys.Invalidate()
	third := sys.Prompt()
	if third == "" {
		t.Error("Prompt() after Invalidate() should return non-empty string")
	}
}

func TestBuildPromptContainsInstructions(t *testing.T) {
	sys := Build(Config{
		Cwd:                 "/tmp/test",
		UserInstructions:    "Always use tabs for indentation.",
		ProjectInstructions: "This is a Go project using Bubble Tea.",
	})

	prompt := sys.Prompt()

	if !strings.Contains(prompt, "<user-instructions>") {
		t.Error("prompt should contain <user-instructions> tag")
	}
	if !strings.Contains(prompt, "Always use tabs for indentation.") {
		t.Error("prompt should contain user instructions content")
	}
	if !strings.Contains(prompt, "<project-instructions>") {
		t.Error("prompt should contain <project-instructions> tag")
	}
	if !strings.Contains(prompt, "This is a Go project using Bubble Tea.") {
		t.Error("prompt should contain project instructions content")
	}
}

func TestBuildPromptDirectFields(t *testing.T) {
	sys := Build(Config{
		Cwd:            "/tmp/test",
		SessionSummary: "<session-summary>\nRefactored core.\n</session-summary>",
		Skills:         "<available-skills>\n- commit\n</available-skills>",
		Agents:         "<available-agents>\n- Explore\n</available-agents>",
	})

	prompt := sys.Prompt()

	if !strings.Contains(prompt, "<session-summary>") {
		t.Error("prompt should contain session-summary")
	}
	if !strings.Contains(prompt, "<available-skills>") {
		t.Error("prompt should contain skills")
	}
	if !strings.Contains(prompt, "<available-agents>") {
		t.Error("prompt should contain agents")
	}
}

func TestBuildPromptExtra(t *testing.T) {
	sys := Build(Config{
		Cwd:   "/tmp/test",
		Extra: []ExtraLayer{{Name: "test-extra", Content: "agent identity content here"}},
	})

	prompt := sys.Prompt()

	if !strings.Contains(prompt, "agent identity content here") {
		t.Error("prompt should contain Extra content")
	}
}

func TestBuildPromptNarrativeOrder(t *testing.T) {
	sys := Build(Config{
		Cwd:                 "/tmp/test",
		UserInstructions:    "USER_INSTRUCTIONS_MARKER",
		ProjectInstructions: "PROJECT_INSTRUCTIONS_MARKER",
		SessionSummary:      "<session-summary>\nSESSION_SUMMARY_MARKER\n</session-summary>",
		Skills:              "<available-skills>\nSKILLS_MARKER\n</available-skills>",
		Agents:              "<available-agents>\nAGENTS_MARKER\n</available-agents>",
		Extra:               []ExtraLayer{{Name: "test", Content: "EXTRA_MARKER"}},
	})

	prompt := sys.Prompt()

	envIdx := strings.Index(prompt, "<env>")
	userIdx := strings.Index(prompt, "USER_INSTRUCTIONS_MARKER")
	projectIdx := strings.Index(prompt, "PROJECT_INSTRUCTIONS_MARKER")
	summaryIdx := strings.Index(prompt, "SESSION_SUMMARY_MARKER")
	skillsIdx := strings.Index(prompt, "SKILLS_MARKER")
	agentsIdx := strings.Index(prompt, "AGENTS_MARKER")
	extraIdx := strings.Index(prompt, "EXTRA_MARKER")

	if envIdx < 0 || userIdx < 0 || projectIdx < 0 || summaryIdx < 0 ||
		skillsIdx < 0 || agentsIdx < 0 || extraIdx < 0 {
		t.Fatal("prompt is missing one or more expected sections")
	}

	if envIdx >= userIdx {
		t.Error("environment should appear before user instructions")
	}
	if userIdx >= projectIdx {
		t.Error("user instructions should appear before project instructions")
	}
	if projectIdx >= summaryIdx {
		t.Error("project instructions should appear before session summary")
	}
	if summaryIdx >= skillsIdx {
		t.Error("session summary should appear before skills")
	}
	if skillsIdx >= agentsIdx {
		t.Error("skills should appear before agents")
	}
	if agentsIdx >= extraIdx {
		t.Error("agents should appear before extra content")
	}
}

func TestBuildPromptPlanMode(t *testing.T) {
	sys := Build(Config{
		Cwd:      "/tmp/test",
		PlanMode: true,
	})
	prompt := sys.Prompt()

	if !strings.Contains(prompt, "plan") && !strings.Contains(prompt, "Plan") {
		t.Error("PlanMode=true should include plan mode content in prompt")
	}

	sys2 := Build(Config{
		Cwd:      "/tmp/test",
		PlanMode: false,
	})
	prompt2 := sys2.Prompt()

	if len(prompt) <= len(prompt2) {
		t.Error("PlanMode=true prompt should be longer than PlanMode=false prompt")
	}
}

func TestBuildPromptEmptyFieldsExcluded(t *testing.T) {
	sys := Build(Config{
		Cwd: "/tmp/test",
	})
	prompt := sys.Prompt()

	if strings.Contains(prompt, "<user-instructions>") {
		t.Error("empty UserInstructions should not produce <user-instructions> tag")
	}
	if strings.Contains(prompt, "<project-instructions>") {
		t.Error("empty ProjectInstructions should not produce <project-instructions> tag")
	}
	if strings.Contains(prompt, "<session-summary>") {
		t.Error("empty SessionSummary should not produce <session-summary> tag")
	}
	if strings.Contains(prompt, "<available-skills>") {
		t.Error("empty Skills should not produce <available-skills> tag")
	}
	if strings.Contains(prompt, "<available-agents>") {
		t.Error("empty Agents should not produce <available-agents> tag")
	}
}

func TestPromptInitCachedFiles(t *testing.T) {
	if cachedBase == "" {
		t.Error("cachedBase should be non-empty after init()")
	}
	if cachedToolsCore == "" {
		t.Error("cachedToolsCore should be non-empty after init()")
	}
	if cachedPlanMode == "" {
		t.Error("cachedPlanMode should be non-empty after init()")
	}

	sys := Build(Config{Cwd: "/tmp/test"})
	prompt := sys.Prompt()

	if !strings.Contains(prompt, cachedBase[:50]) {
		t.Error("prompt should contain base.txt content")
	}
	if !strings.Contains(prompt, cachedToolsCore[:50]) {
		t.Error("prompt should contain tools content")
	}
}

func TestCompactPrompt(t *testing.T) {
	result := CompactPrompt()
	if result == "" {
		t.Error("CompactPrompt() should return non-empty string")
	}
}
