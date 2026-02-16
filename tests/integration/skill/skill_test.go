package skill_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yanmxa/gencode/internal/skill"
)

// newTestRegistry creates a skill.Registry backed by temp stores for testing.
func newTestRegistry(t *testing.T, skills map[string]*skill.Skill) *skill.Registry {
	t.Helper()
	tmpDir := t.TempDir()
	userStore, _ := skill.NewStore(filepath.Join(tmpDir, "user-skills.json"))
	projectStore, _ := skill.NewStore(filepath.Join(tmpDir, "project-skills.json"))
	return skill.NewRegistryForTest(skills, userStore, projectStore)
}

func TestSkill_StateTransitions(t *testing.T) {
	s := skill.StateDisable

	// disable -> enable
	s = s.NextState()
	if s != skill.StateEnable {
		t.Errorf("expected StateEnable, got %q", s)
	}

	// enable -> active
	s = s.NextState()
	if s != skill.StateActive {
		t.Errorf("expected StateActive, got %q", s)
	}

	// active -> disable
	s = s.NextState()
	if s != skill.StateDisable {
		t.Errorf("expected StateDisable, got %q", s)
	}
}

func TestSkill_StateIcons(t *testing.T) {
	tests := []struct {
		state skill.SkillState
		icon  string
	}{
		{skill.StateDisable, "○"},
		{skill.StateEnable, "◐"},
		{skill.StateActive, "●"},
	}

	for _, tt := range tests {
		if got := tt.state.Icon(); got != tt.icon {
			t.Errorf("state %q icon = %q, want %q", tt.state, got, tt.icon)
		}
	}
}

func TestSkill_BooleanHelpers(t *testing.T) {
	s := &skill.Skill{Name: "test"}

	s.State = skill.StateDisable
	if s.IsEnabled() {
		t.Error("disabled skill should not be enabled")
	}
	if s.IsActive() {
		t.Error("disabled skill should not be active")
	}

	s.State = skill.StateEnable
	if !s.IsEnabled() {
		t.Error("enabled skill should be enabled")
	}
	if s.IsActive() {
		t.Error("enabled skill should not be active")
	}

	s.State = skill.StateActive
	if !s.IsEnabled() {
		t.Error("active skill should be enabled")
	}
	if !s.IsActive() {
		t.Error("active skill should be active")
	}
}

func TestSkill_RegistryLookup(t *testing.T) {
	skills := map[string]*skill.Skill{
		"git:commit":  {Name: "commit", Namespace: "git", State: skill.StateEnable, Description: "create commit"},
		"git:push":    {Name: "push", Namespace: "git", State: skill.StateActive, Description: "push changes"},
		"review":      {Name: "review", State: skill.StateEnable, Description: "review code"},
	}

	registry := newTestRegistry(t, skills)

	// Exact match
	s, ok := registry.Get("git:commit")
	if !ok {
		t.Fatal("expected to find git:commit")
	}
	if s.Name != "commit" {
		t.Errorf("expected name 'commit', got %q", s.Name)
	}

	// Partial match
	s = registry.FindByPartialName("push")
	if s == nil {
		t.Fatal("expected to find 'push' by partial name")
	}
	if s.FullName() != "git:push" {
		t.Errorf("expected 'git:push', got %q", s.FullName())
	}

	// Not found
	s = registry.FindByPartialName("nonexistent")
	if s != nil {
		t.Error("expected nil for nonexistent skill")
	}

	// List
	all := registry.List()
	if len(all) != 3 {
		t.Errorf("expected 3 skills, got %d", len(all))
	}
}

func TestSkill_AvailablePrompt(t *testing.T) {
	skills := map[string]*skill.Skill{
		"git:commit": {Name: "commit", Namespace: "git", State: skill.StateActive, Description: "create commit"},
		"review":     {Name: "review", State: skill.StateEnable, Description: "review code"},
	}

	registry := newTestRegistry(t, skills)

	prompt := registry.GetAvailableSkillsPrompt()

	// Only active skills should be in the prompt
	if !strings.Contains(prompt, "git:commit") {
		t.Error("expected active skill 'git:commit' in prompt")
	}
	if strings.Contains(prompt, "review") {
		t.Error("enabled (non-active) skill 'review' should not be in prompt")
	}
	if !strings.Contains(prompt, "<available-skills>") {
		t.Error("expected XML wrapper tags")
	}
}

func TestSkill_InvocationPrompt(t *testing.T) {
	skills := map[string]*skill.Skill{
		"test-skill": {
			Name:         "test-skill",
			State:        skill.StateActive,
			Description:  "a test skill",
			Instructions: "Do the thing step by step",
		},
	}

	registry := newTestRegistry(t, skills)

	prompt := registry.GetSkillInvocationPrompt("test-skill")

	if !strings.Contains(prompt, "<skill-invocation") {
		t.Error("expected XML wrapper")
	}
	if !strings.Contains(prompt, "Do the thing step by step") {
		t.Error("expected instructions in prompt")
	}
}

func TestSkill_ScopePriority(t *testing.T) {
	// Lower scope value = lower priority
	if skill.ScopeClaudeUser >= skill.ScopeUser {
		t.Error("ScopeClaudeUser should be lower priority than ScopeUser")
	}
	if skill.ScopeUser >= skill.ScopeProject {
		t.Error("ScopeUser should be lower priority than ScopeProject")
	}
	if skill.ScopeUserPlugin >= skill.ScopeProjectPlugin {
		t.Error("ScopeUserPlugin should be lower priority than ScopeProjectPlugin")
	}
}

func TestSkill_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "skills.json")

	// Create store, set state
	store, err := skill.NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}

	err = store.SetState("git:commit", skill.StateActive)
	if err != nil {
		t.Fatalf("SetState() error: %v", err)
	}

	// Verify file was written
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Fatal("expected skills.json to exist")
	}

	// Create new store from same file — state should persist
	store2, err := skill.NewStore(storePath)
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}

	state, ok := store2.GetState("git:commit")
	if !ok {
		t.Fatal("expected persisted state for git:commit")
	}
	if state != skill.StateActive {
		t.Errorf("expected StateActive, got %q", state)
	}
}

func TestSkill_FullName(t *testing.T) {
	s1 := &skill.Skill{Name: "commit", Namespace: "git"}
	if s1.FullName() != "git:commit" {
		t.Errorf("expected 'git:commit', got %q", s1.FullName())
	}

	s2 := &skill.Skill{Name: "review"}
	if s2.FullName() != "review" {
		t.Errorf("expected 'review', got %q", s2.FullName())
	}
}
