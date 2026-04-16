package hooks_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/hooks"
)

func TestHooks_BlockToolCall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows (no sh)")
	}

	settings := &config.Settings{
		Hooks: map[string][]config.Hook{
			"PreToolUse": {
				{
					Matcher: "Bash",
					Hooks: []config.HookCmd{
						{Type: "command", Command: "echo 'blocked' >&2; exit 2"},
					},
				},
			},
		},
	}

	engine := hooks.NewEngine(settings, "test-session", t.TempDir(), "")

	input := hooks.HookInput{
		ToolName:  "Bash",
		ToolInput: map[string]any{"command": "ls"},
		ToolUseID: "tc1",
	}

	outcome := engine.Execute(context.Background(), hooks.PreToolUse, input)

	if !outcome.ShouldBlock {
		t.Error("expected hook to block execution")
	}
	if outcome.BlockReason == "" {
		t.Error("expected non-empty block reason")
	}
}

func TestHooks_ModifyToolInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows (no sh)")
	}

	settings := &config.Settings{
		Hooks: map[string][]config.Hook{
			"PreToolUse": {
				{
					Matcher: "Read",
					Hooks: []config.HookCmd{
						{Type: "command", Command: `echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"file_path":"/modified"}}}'`},
					},
				},
			},
		},
	}

	engine := hooks.NewEngine(settings, "test-session", t.TempDir(), "")

	input := hooks.HookInput{
		ToolName:  "Read",
		ToolInput: map[string]any{"file_path": "/original"},
		ToolUseID: "tc1",
	}

	outcome := engine.Execute(context.Background(), hooks.PreToolUse, input)

	if outcome.ShouldBlock {
		t.Error("should not block")
	}
	if outcome.UpdatedInput == nil {
		t.Fatal("expected updated input")
	}
	if outcome.UpdatedInput["file_path"] != "/modified" {
		t.Errorf("expected modified path '/modified', got %v", outcome.UpdatedInput["file_path"])
	}
}

func TestHooks_NoHooks_PassThrough(t *testing.T) {
	// No hooks configured
	engine := hooks.NewEngine(&config.Settings{}, "test-session", t.TempDir(), "")

	input := hooks.HookInput{
		ToolName:  "Read",
		ToolInput: map[string]any{"file_path": "/test"},
		ToolUseID: "tc1",
	}

	outcome := engine.Execute(context.Background(), hooks.PreToolUse, input)

	if outcome.ShouldBlock {
		t.Error("no hooks should mean no blocking")
	}
	if !outcome.ShouldContinue {
		t.Error("should continue when no hooks configured")
	}
}

func TestHooks_NilSettings(t *testing.T) {
	engine := hooks.NewEngine(nil, "test-session", t.TempDir(), "")

	if engine.HasHooks(hooks.PreToolUse) {
		t.Error("nil settings should have no hooks")
	}

	outcome := engine.Execute(context.Background(), hooks.PreToolUse, hooks.HookInput{})
	if outcome.ShouldBlock {
		t.Error("nil settings should not block")
	}
}

func TestHooks_HasHooks(t *testing.T) {
	settings := &config.Settings{
		Hooks: map[string][]config.Hook{
			"PreToolUse": {
				{Hooks: []config.HookCmd{{Command: "echo ok"}}},
			},
		},
	}

	engine := hooks.NewEngine(settings, "test-session", t.TempDir(), "")

	if !engine.HasHooks(hooks.PreToolUse) {
		t.Error("expected HasHooks=true for PreToolUse")
	}
	if engine.HasHooks(hooks.PostToolUse) {
		t.Error("expected HasHooks=false for PostToolUse")
	}
}
