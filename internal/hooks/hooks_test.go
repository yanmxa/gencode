package hooks

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		{Stop, HookInput{}, ""},                            // No matcher support
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
		PermissionDenied, SessionStart, SessionEnd, Notification,
		SubagentStart, SubagentStop, PreCompact,
	}

	notSupported := []EventType{
		UserPromptSubmit, Stop, StopFailure,
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
`), 0o755)
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
`), 0o755)
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
`), 0o755)
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
	err := os.WriteFile(scriptPath, []byte(script), 0o755)
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
`), 0o755)
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

func TestHooks_Timeout_TerminatesHook(t *testing.T) {
	// Create a script that uses exec to replace the shell process so
	// exec.CommandContext can kill it directly (no orphaned children).
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "sleep.sh")
	err := os.WriteFile(scriptPath, []byte(`#!/bin/bash
exec sleep 30
`), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	// Timeout of 1 second — the sleep process will be killed after 1s
	settings.Hooks["PreToolUse"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: scriptPath, Timeout: 1}}},
	}

	engine := NewEngine(settings, "test-session", tmpDir, "")

	start := time.Now()
	outcome := engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})
	elapsed := time.Since(start)

	// Should terminate well before the sleep duration (30s)
	if elapsed > 5*time.Second {
		t.Errorf("expected hook to be killed within ~1s, took %v", elapsed)
	}

	// A timeout-killed process: engine should still continue (not exit 2)
	if !outcome.ShouldContinue {
		t.Error("expected main loop to continue after hook timeout")
	}
	if outcome.ShouldBlock {
		t.Error("expected ShouldBlock=false after hook timeout (not exit 2)")
	}
}

func TestHooks_Once_ExecutesExactlyOnce(t *testing.T) {
	tmpDir := t.TempDir()
	counterFile := filepath.Join(tmpDir, "count.txt")

	// Script increments a counter file each time it runs
	scriptPath := filepath.Join(tmpDir, "counter.sh")
	err := os.WriteFile(scriptPath, []byte(`#!/bin/bash
echo -n "x" >> `+counterFile+`
`), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	settings.Hooks["PreToolUse"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: scriptPath, Once: true}}},
	}

	engine := NewEngine(settings, "test-session", tmpDir, "")

	// Fire the hook twice
	engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})
	engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})

	// Read the counter — should be exactly 1 character (fired once)
	content, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("counter file not created: %v", err)
	}
	if len(content) != 1 {
		t.Errorf("expected hook to fire exactly once, but counter=%d (content=%q)", len(content), content)
	}
}

func TestHooks_InputContains_SessionContext(t *testing.T) {
	tmpDir := t.TempDir()
	captureFile := filepath.Join(tmpDir, "input.json")

	// Script captures stdin (the hook input JSON) to a file
	scriptPath := filepath.Join(tmpDir, "capture.sh")
	err := os.WriteFile(scriptPath, []byte(`#!/bin/bash
cat > `+captureFile+`
`), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	settings := config.NewSettings()
	settings.Hooks["PreToolUse"] = []config.Hook{
		{Hooks: []config.HookCmd{{Type: "command", Command: scriptPath}}},
	}

	const sessionID = "test-session-xyz"
	engine := NewEngine(settings, sessionID, tmpDir, "")

	engine.Execute(context.Background(), PreToolUse, HookInput{ToolName: "Bash"})

	// Read captured JSON
	data, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("capture file not created: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("captured input is not valid JSON: %v\nContent: %s", err, data)
	}

	// Must include session_id
	sid, ok := parsed["session_id"].(string)
	if !ok || sid != sessionID {
		t.Errorf("expected session_id=%q in hook input, got %v", sessionID, parsed["session_id"])
	}

	// Must include cwd
	cwd, ok := parsed["cwd"].(string)
	if !ok || cwd != tmpDir {
		t.Errorf("expected cwd=%q in hook input, got %v", tmpDir, parsed["cwd"])
	}
}
