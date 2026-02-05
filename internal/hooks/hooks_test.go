package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yanmxa/gencode/internal/config"
)

func TestMatchesEvent(t *testing.T) {
	tests := []struct {
		name       string
		matcher    string
		matchValue string
		want       bool
	}{
		{"empty matcher matches everything", "", "anything", true},
		{"wildcard matcher matches everything", "*", "anything", true},
		{"exact match", "Bash", "Bash", true},
		{"exact match fails", "Bash", "Edit", false},
		{"regex or pattern", "Write|Edit", "Write", true},
		{"regex or pattern second", "Write|Edit", "Edit", true},
		{"regex or pattern fails", "Write|Edit", "Bash", false},
		{"regex prefix", "Bash.*", "BashTool", true},
		{"regex prefix fails", "Bash.*", "XBash", false},
		{"invalid regex falls back to exact", "[invalid", "[invalid", true},
		{"invalid regex fails", "[invalid", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesEvent(tt.matcher, tt.matchValue)
			if got != tt.want {
				t.Errorf("MatchesEvent(%q, %q) = %v, want %v", tt.matcher, tt.matchValue, got, tt.want)
			}
		})
	}
}

func TestGetMatchValue(t *testing.T) {
	tests := []struct {
		event EventType
		input HookInput
		want  string
	}{
		{PreToolUse, HookInput{ToolName: "Bash"}, "Bash"},
		{PostToolUse, HookInput{ToolName: "Edit"}, "Edit"},
		{PostToolUseFailure, HookInput{ToolName: "Write"}, "Write"},
		{PermissionRequest, HookInput{ToolName: "Task"}, "Task"},
		{SessionStart, HookInput{Source: "startup"}, "startup"},
		{SessionEnd, HookInput{Reason: "quit"}, "quit"},
		{Notification, HookInput{NotificationType: "permission_prompt"}, "permission_prompt"},
		{SubagentStart, HookInput{AgentType: "Explore"}, "Explore"},
		{SubagentStop, HookInput{AgentType: "Plan"}, "Plan"},
		{PreCompact, HookInput{Trigger: "auto"}, "auto"},
		{UserPromptSubmit, HookInput{Prompt: "hello"}, ""}, // No matcher support
		{Stop, HookInput{}, ""},                             // No matcher support
	}

	for _, tt := range tests {
		t.Run(string(tt.event), func(t *testing.T) {
			got := GetMatchValue(tt.event, tt.input)
			if got != tt.want {
				t.Errorf("GetMatchValue(%v, %+v) = %q, want %q", tt.event, tt.input, got, tt.want)
			}
		})
	}
}

func TestEventSupportsMatcher(t *testing.T) {
	supported := []EventType{
		PreToolUse, PostToolUse, PostToolUseFailure, PermissionRequest,
		SessionStart, SessionEnd, Notification,
		SubagentStart, SubagentStop, PreCompact,
	}

	notSupported := []EventType{
		UserPromptSubmit, Stop,
	}

	for _, event := range supported {
		if !EventSupportsMatcher(event) {
			t.Errorf("EventSupportsMatcher(%v) = false, want true", event)
		}
	}

	for _, event := range notSupported {
		if EventSupportsMatcher(event) {
			t.Errorf("EventSupportsMatcher(%v) = true, want false", event)
		}
	}
}

func TestEngineNoHooks(t *testing.T) {
	settings := config.NewSettings()
	engine := NewEngine(settings, "test-session", "/tmp", "")

	outcome := engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})

	if !outcome.ShouldContinue {
		t.Error("Expected ShouldContinue=true when no hooks configured")
	}
	if outcome.ShouldBlock {
		t.Error("Expected ShouldBlock=false when no hooks configured")
	}
}

func TestEngineNilSettings(t *testing.T) {
	engine := NewEngine(nil, "test-session", "/tmp", "")

	outcome := engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})

	if !outcome.ShouldContinue {
		t.Error("Expected ShouldContinue=true with nil settings")
	}
}

func TestEngineHasHooks(t *testing.T) {
	settings := config.NewSettings()
	settings.Hooks["Stop"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: "echo done"}}},
	}

	engine := NewEngine(settings, "test-session", "/tmp", "")

	if !engine.HasHooks(Stop) {
		t.Error("Expected HasHooks(Stop)=true")
	}
	if engine.HasHooks(PreToolUse) {
		t.Error("Expected HasHooks(PreToolUse)=false")
	}
}

func TestEngineMatcherFiltering(t *testing.T) {
	// Create a temp script that outputs JSON
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "hook.sh")
	err := os.WriteFile(scriptPath, []byte(`#!/bin/bash
echo '{"systemMessage":"hook executed"}'
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	settings.Hooks["PreToolUse"] = []config.Hook{
		{
			Matcher: "Bash",
			Hooks:   []config.HookCmd{{Type: "command", Command: scriptPath}},
		},
	}

	engine := NewEngine(settings, "test-session", tmpDir, "")

	// Should match
	outcome := engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})
	if outcome.AdditionalContext != "hook executed" {
		t.Errorf("Expected context from hook, got %q", outcome.AdditionalContext)
	}

	// Should not match
	outcome = engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Edit"})
	if outcome.AdditionalContext != "" {
		t.Errorf("Expected no context for non-matching tool, got %q", outcome.AdditionalContext)
	}
}

func TestEngineBlockingHook(t *testing.T) {
	// Create a script that exits with code 2 (blocking)
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "block.sh")
	err := os.WriteFile(scriptPath, []byte(`#!/bin/bash
echo "Blocked by policy" >&2
exit 2
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	settings.Hooks["PreToolUse"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: scriptPath}}},
	}

	engine := NewEngine(settings, "test-session", tmpDir, "")

	outcome := engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})

	if outcome.ShouldContinue {
		t.Error("Expected ShouldContinue=false for blocking hook")
	}
	if !outcome.ShouldBlock {
		t.Error("Expected ShouldBlock=true for blocking hook")
	}
	if outcome.BlockReason != "Blocked by policy" {
		t.Errorf("Expected BlockReason='Blocked by policy', got %q", outcome.BlockReason)
	}
}

func TestEngineJSONBlockingOutput(t *testing.T) {
	// Create a script that outputs JSON with continue=false
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "deny.sh")
	err := os.WriteFile(scriptPath, []byte(`#!/bin/bash
echo '{"continue":false,"stopReason":"Denied by hook"}'
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	settings.Hooks["PreToolUse"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: scriptPath}}},
	}

	engine := NewEngine(settings, "test-session", tmpDir, "")

	outcome := engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})

	if outcome.ShouldContinue {
		t.Error("Expected ShouldContinue=false")
	}
	if !outcome.ShouldBlock {
		t.Error("Expected ShouldBlock=true")
	}
	if outcome.BlockReason != "Denied by hook" {
		t.Errorf("Expected BlockReason='Denied by hook', got %q", outcome.BlockReason)
	}
}

func TestEngineUpdatedInput(t *testing.T) {
	// Create a script that modifies tool input
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "modify.sh")
	updatedInput := map[string]any{"command": "safe-command"}
	updatedJSON, _ := json.Marshal(updatedInput)
	script := `#!/bin/bash
echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":` + string(updatedJSON) + `}}'
`
	err := os.WriteFile(scriptPath, []byte(script), 0755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	settings.Hooks["PreToolUse"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: scriptPath}}},
	}

	engine := NewEngine(settings, "test-session", tmpDir, "")

	outcome := engine.Execute(context.Background(), PreToolUse, HookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "rm -rf /"},
	})

	if outcome.UpdatedInput == nil {
		t.Fatal("Expected UpdatedInput to be set")
	}
	if outcome.UpdatedInput["command"] != "safe-command" {
		t.Errorf("Expected command='safe-command', got %v", outcome.UpdatedInput["command"])
	}
}

func TestEngineEnvironmentVariables(t *testing.T) {
	// Create a script that outputs env vars
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "env.sh")
	err := os.WriteFile(scriptPath, []byte(`#!/bin/bash
echo "{\"systemMessage\":\"GEN=$GEN_PROJECT_DIR CLAUDE=$CLAUDE_PROJECT_DIR\"}"
`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	settings.Hooks["Stop"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: scriptPath}}},
	}

	engine := NewEngine(settings, "test-session", tmpDir, "")

	outcome := engine.Execute(context.Background(), Stop, HookInput{})

	expected := "GEN=" + tmpDir + " CLAUDE=" + tmpDir
	if outcome.AdditionalContext != expected {
		t.Errorf("Expected context=%q, got %q", expected, outcome.AdditionalContext)
	}
}

func TestEnginePermissionMode(t *testing.T) {
	engine := NewEngine(config.NewSettings(), "test-session", "/tmp", "")

	engine.SetPermissionMode("auto")

	// Just verify it doesn't panic - we'd need a hook that reads stdin to fully test
}
