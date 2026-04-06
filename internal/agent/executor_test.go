package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/skill"
)

func TestPrepareRunConfigRespectsOverrides(t *testing.T) {
	executor := &Executor{parentModelID: "parent-model"}

	rc, err := executor.prepareRunConfig(AgentRequest{
		Agent:    "Explore",
		Name:     "Scout",
		Model:    "override-model",
		MaxTurns: 7,
		Mode:     string(PermissionDontAsk),
	})
	if err != nil {
		t.Fatalf("prepareRunConfig() error: %v", err)
	}

	if rc.displayName != "Scout" {
		t.Fatalf("expected display name override, got %q", rc.displayName)
	}
	if rc.modelID != "override-model" {
		t.Fatalf("expected model override, got %q", rc.modelID)
	}
	if rc.maxTurns != 7 {
		t.Fatalf("expected max turns override, got %d", rc.maxTurns)
	}
	if rc.permMode != PermissionDontAsk {
		t.Fatalf("expected permission mode override, got %q", rc.permMode)
	}
	if !strings.Contains(rc.agentPrompt, "## Mode: Autonomous") {
		t.Fatalf("expected autonomous mode prompt, got %q", rc.agentPrompt)
	}
}

func TestPrepareRunConfigUsesResolvedPlanModePrompt(t *testing.T) {
	executor := &Executor{}

	rc, err := executor.prepareRunConfig(AgentRequest{
		Agent: "general-purpose",
		Mode:  string(PermissionPlan),
	})
	if err != nil {
		t.Fatalf("prepareRunConfig() error: %v", err)
	}

	if rc.permMode != PermissionPlan {
		t.Fatalf("expected plan mode, got %q", rc.permMode)
	}
	if !strings.Contains(rc.agentPrompt, "## Mode: Read-Only") {
		t.Fatalf("expected read-only mode prompt, got %q", rc.agentPrompt)
	}
}

func TestBuildCancelledAgentResultUsesPreparedRunMetadata(t *testing.T) {
	executor := &Executor{}
	run := &preparedRun{
		req: AgentRequest{Agent: "Explore"},
		cfg: &runConfig{
			displayName: "Scout",
			modelID:     "test-model",
		},
		startedAt: time.Now().Add(-time.Second),
		progress:  []string{"Read(main.go)"},
	}

	result := executor.buildCancelledAgentResult(run, &core.Result{
		Content:    "partial",
		Messages:   []message.Message{{Role: message.RoleAssistant, Content: "partial"}},
		Turns:      2,
		ToolUses:   1,
		StopReason: core.StopCancelled,
	})
	if result == nil {
		t.Fatal("expected cancelled result")
	}
	if result.AgentName != "Scout" {
		t.Fatalf("expected prepared display name, got %q", result.AgentName)
	}
	if result.Model != "test-model" {
		t.Fatalf("expected prepared model, got %q", result.Model)
	}
	if len(result.Progress) != 1 || result.Progress[0] != "Read(main.go)" {
		t.Fatalf("unexpected progress: %#v", result.Progress)
	}
	if result.Error != "agent cancelled" {
		t.Fatalf("unexpected error: %q", result.Error)
	}
}

func TestBuildSystemPrompt_IncludesAdditionalInstructionsAndPreloadedSkills(t *testing.T) {
	prev := skill.DefaultRegistry
	t.Cleanup(func() { skill.DefaultRegistry = prev })

	tmpDir := t.TempDir()
	skillFile := filepath.Join(tmpDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(`---
name: commit
description: Write commit messages
---
Use conventional commits.
`), 0o644); err != nil {
		t.Fatalf("WriteFile(skill): %v", err)
	}

	userStore, err := skill.NewStore(filepath.Join(tmpDir, "user-skills.json"))
	if err != nil {
		t.Fatalf("NewStore(user): %v", err)
	}
	projectStore, err := skill.NewStore(filepath.Join(tmpDir, "project-skills.json"))
	if err != nil {
		t.Fatalf("NewStore(project): %v", err)
	}

	skill.DefaultRegistry = skill.NewRegistryForTest(map[string]*skill.Skill{
		"git:commit": {
			Name:      "commit",
			Namespace: "git",
			FilePath:  skillFile,
			SkillDir:  tmpDir,
			State:     skill.StateActive,
		},
	}, userStore, projectStore)

	executor := &Executor{}
	prompt := executor.buildSystemPrompt(&AgentConfig{
		Name:         "Reviewer",
		Description:  "Reviews code changes.",
		SystemPrompt: "Prefer minimal, surgical fixes.",
		Skills:       []string{"git:commit"},
	}, PermissionDontAsk)

	if !strings.Contains(prompt, "## Additional Instructions") {
		t.Fatal("expected additional instructions section in prompt")
	}
	if !strings.Contains(prompt, "Prefer minimal, surgical fixes.") {
		t.Fatal("expected custom system prompt content")
	}
	if !strings.Contains(prompt, `<skill-invocation name="git:commit">`) {
		t.Fatal("expected preloaded skill invocation prompt")
	}
	if !strings.Contains(prompt, "Use conventional commits.") {
		t.Fatal("expected skill instructions in agent prompt")
	}
}
