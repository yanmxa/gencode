package subagent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/skill"
)

type stubSubagentSessionStore struct {
	saveParentID string
	saveTitle    string
	saveModelID  string
	saveCwd      string
	saveMessages []core.Message
	loadMessages []core.Message
	loadErr      error
}

func (s *stubSubagentSessionStore) SaveSubagentConversation(parentSessionID, title, modelID, cwd string, messages []core.Message) (string, string, error) {
	s.saveParentID = parentSessionID
	s.saveTitle = title
	s.saveModelID = modelID
	s.saveCwd = cwd
	s.saveMessages = append([]core.Message(nil), messages...)
	return "agent-1", "/tmp/transcripts/agent-1.jsonl", nil
}

func (s *stubSubagentSessionStore) LoadSubagentMessages(agentID string) ([]core.Message, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return append([]core.Message(nil), s.loadMessages...), nil
}

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
		Messages:   []core.Message{{Role: core.RoleAssistant, Content: "partial"}},
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

func TestFormatToolProgressUsesReadableAgentLabel(t *testing.T) {
	got := formatToolProgress("Agent", map[string]any{
		"subagent_type": "Explore",
		"description":   "HA code structure",
		"prompt":        "Inspect the codebase",
	})

	if got != "Agent: Explore HA code structure" {
		t.Fatalf("formatToolProgress() = %q, want %q", got, "Agent: Explore HA code structure")
	}
}

func TestFormatToolProgressFallsBackToTaskOutputID(t *testing.T) {
	got := formatToolProgress("TaskOutput", map[string]any{
		"task_id": "task-123",
	})

	if got != "TaskOutput(task-123)" {
		t.Fatalf("formatToolProgress() = %q, want %q", got, "TaskOutput(task-123)")
	}
}

func TestBuildSystemPrompt_IncludesAdditionalInstructionsAndPreloadedSkills(t *testing.T) {
	prev := skill.DefaultIfInit()
	t.Cleanup(func() { skill.SetDefault(prev) })

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

	skill.SetDefault(skill.NewRegistryForTest(map[string]*skill.Skill{
		"git:commit": {
			Name:      "commit",
			Namespace: "git",
			FilePath:  skillFile,
			SkillDir:  tmpDir,
			State:     skill.StateActive,
		},
	}, userStore, projectStore))

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

func TestPlanAgentsExposeOnlyReadOnlyTools(t *testing.T) {
	tests := []string{"Explore", "Plan"}

	for _, agentName := range tests {
		t.Run(agentName, func(t *testing.T) {
			cfg, ok := defaultRegistry.Get(agentName)
			if !ok {
				t.Fatalf("agent %q not found", agentName)
			}

			if cfg.PermissionMode != PermissionPlan {
				t.Fatalf("expected %q to use plan permissions, got %q", agentName, cfg.PermissionMode)
			}
			if slices.Contains([]string(cfg.Tools), "Bash") {
				t.Fatalf("plan-mode agent %q must not expose Bash", agentName)
			}

			want := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch"}
			if !slices.Equal([]string(cfg.Tools), want) {
				t.Fatalf("unexpected tool list for %q: got %v want %v", agentName, cfg.Tools, want)
			}
		})
	}
}

func TestPersistSubagentSessionUsesSessionStore(t *testing.T) {
	store := &stubSubagentSessionStore{}
	executor := &Executor{
		cwd:             "/tmp/project",
		sessionStore:    store,
		parentSessionID: "parent-1",
	}

	sessionID, transcriptPath := executor.persistSubagentSession("Explore", "test-model", "Inspect code", []core.Message{
		{Role: core.RoleUser, Content: "hello"},
	})

	if sessionID != "agent-1" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "agent-1")
	}
	if transcriptPath != "/tmp/transcripts/agent-1.jsonl" {
		t.Fatalf("transcriptPath = %q", transcriptPath)
	}
	if store.saveParentID != "parent-1" || store.saveTitle != "Inspect code" || store.saveModelID != "test-model" || store.saveCwd != "/tmp/project" {
		t.Fatalf("unexpected save args: %+v", store)
	}
	if len(store.saveMessages) != 1 || store.saveMessages[0].Content != "hello" {
		t.Fatalf("unexpected saved messages: %+v", store.saveMessages)
	}
}

func TestResumeFromSessionUsesSessionStore(t *testing.T) {
	store := &stubSubagentSessionStore{
		loadMessages: []core.Message{
			{Role: core.RoleUser, Content: "previous"},
			{Role: core.RoleAssistant, Content: "response"},
		},
	}
	executor := &Executor{sessionStore: store}

	// Create a minimal core.Agent for testing
	ag := core.NewAgent(core.Config{
		LLM:    &stubLLM{},
		System: &stubSystem{},
		Tools:  core.NewTools(),
	})
	ctx := context.Background()

	if err := executor.resumeFromSession(ag, ctx, "agent-1", "continue"); err != nil {
		t.Fatalf("resumeFromSession(): %v", err)
	}

	msgs := ag.Messages()
	if len(msgs) != 3 {
		t.Fatalf("len(messages) = %d, want 3", len(msgs))
	}
	if msgs[2].Role != core.RoleUser || msgs[2].Content != "continue" {
		t.Fatalf("unexpected continuation message: %+v", msgs[2])
	}
}

func TestResumeFromSessionRequiresSessionStore(t *testing.T) {
	executor := &Executor{}
	ag := core.NewAgent(core.Config{
		LLM:    &stubLLM{},
		System: &stubSystem{},
		Tools:  core.NewTools(),
	})
	err := executor.resumeFromSession(ag, context.Background(), "agent-1", "continue")
	if err == nil || !strings.Contains(err.Error(), "session store not configured") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResumeFromSessionPropagatesLoadError(t *testing.T) {
	executor := &Executor{
		sessionStore: &stubSubagentSessionStore{loadErr: errors.New("boom")},
	}
	ag := core.NewAgent(core.Config{
		LLM:    &stubLLM{},
		System: &stubSystem{},
		Tools:  core.NewTools(),
	})
	err := executor.resumeFromSession(ag, context.Background(), "agent-1", "continue")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// stubLLM is a minimal core.LLM for tests that don't call inference.
type stubLLM struct{}

func (s *stubLLM) Infer(_ context.Context, _ core.InferRequest) (<-chan core.Chunk, error) {
	ch := make(chan core.Chunk)
	close(ch)
	return ch, nil
}
func (s *stubLLM) InputLimit() int { return 0 }

// stubSystem is a minimal core.System for tests.
type stubSystem struct{}

func (s *stubSystem) Prompt() string                  { return "" }
func (s *stubSystem) Set(_ core.Layer)                {}
func (s *stubSystem) Remove(_ string)                 {}
func (s *stubSystem) Get(_ string) (core.Layer, bool) { return core.Layer{}, false }
func (s *stubSystem) Layers() []core.Layer            { return nil }
func (s *stubSystem) Invalidate()                     {}
