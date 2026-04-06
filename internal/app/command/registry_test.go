package command

import (
	"path/filepath"
	"testing"

	"github.com/yanmxa/gencode/internal/skill"
)

func newTestSkillRegistry(t *testing.T, skills map[string]*skill.Skill) *skill.Registry {
	t.Helper()
	tmpDir := t.TempDir()
	userStore, err := skill.NewStore(filepath.Join(tmpDir, "user-skills.json"))
	if err != nil {
		t.Fatalf("NewStore(user): %v", err)
	}
	projectStore, err := skill.NewStore(filepath.Join(tmpDir, "project-skills.json"))
	if err != nil {
		t.Fatalf("NewStore(project): %v", err)
	}
	return skill.NewRegistryForTest(skills, userStore, projectStore)
}

func TestGetSkillCommands_OnlyEnabledAndIncludesArgumentHint(t *testing.T) {
	prev := skill.DefaultRegistry
	t.Cleanup(func() { skill.DefaultRegistry = prev })

	skill.DefaultRegistry = newTestSkillRegistry(t, map[string]*skill.Skill{
		"search": {
			Name:         "search",
			Description:  "Search files",
			ArgumentHint: "<pattern>",
			State:        skill.StateEnable,
		},
		"review": {
			Name:        "review",
			Description: "Review code",
			State:       skill.StateActive,
		},
		"hidden": {
			Name:        "hidden",
			Description: "Hidden skill",
			State:       skill.StateDisable,
		},
	})

	cmds := GetSkillCommands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 enabled skill commands, got %d", len(cmds))
	}

	foundSearch := false
	for _, cmd := range cmds {
		if cmd.Name == "search" {
			foundSearch = true
			if cmd.Description != "Search files <pattern>" {
				t.Errorf("search description = %q, want %q", cmd.Description, "Search files <pattern>")
			}
		}
		if cmd.Name == "hidden" {
			t.Fatal("disabled skill should not appear in slash command list")
		}
	}
	if !foundSearch {
		t.Fatal("expected enabled search skill in slash command list")
	}
}

func TestIsSkillCommand_RejectsDisabledSkill(t *testing.T) {
	prev := skill.DefaultRegistry
	t.Cleanup(func() { skill.DefaultRegistry = prev })

	skill.DefaultRegistry = newTestSkillRegistry(t, map[string]*skill.Skill{
		"enabled": {
			Name:  "enabled",
			State: skill.StateEnable,
		},
		"disabled": {
			Name:  "disabled",
			State: skill.StateDisable,
		},
	})

	if _, ok := IsSkillCommand("enabled"); !ok {
		t.Fatal("expected enabled skill to be invocable as slash command")
	}
	if _, ok := IsSkillCommand("disabled"); ok {
		t.Fatal("disabled skill should not be invocable as slash command")
	}
}
