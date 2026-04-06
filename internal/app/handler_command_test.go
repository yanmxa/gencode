package app

import (
	"context"
	"strings"
	"testing"

	appcommand "github.com/yanmxa/gencode/internal/app/command"
	appmode "github.com/yanmxa/gencode/internal/app/mode"
	appsession "github.com/yanmxa/gencode/internal/app/session"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/session"
)

func TestHandlerRegistryMatchesBuiltinCommands(t *testing.T) {
	handlers := handlerRegistry()
	builtins := appcommand.BuiltinNames()

	if len(handlers) != len(builtins) {
		t.Fatalf("handler registry size mismatch: got %d, want %d", len(handlers), len(builtins))
	}

	for name := range builtins {
		if _, ok := handlers[name]; !ok {
			t.Fatalf("missing handler for builtin command %q", name)
		}
	}
}

func TestExecuteCommandExit(t *testing.T) {
	m := &model{}

	result, cmd, handled := ExecuteCommand(context.Background(), m, "/exit")
	if !handled {
		t.Fatal("expected /exit to be handled")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("expected quit command to produce a message")
	}
}

func TestExecuteCommandUnknown(t *testing.T) {
	m := &model{}

	result, cmd, handled := ExecuteCommand(context.Background(), m, "/definitely-unknown")
	if !handled {
		t.Fatal("expected unknown command to be handled")
	}
	if cmd != nil {
		t.Fatal("did not expect follow-up command")
	}
	if result != "Unknown command: /definitely-unknown\nType /help for available commands." {
		t.Fatalf("unexpected result: %q", result)
	}
}

func TestExecuteCommandPlanUsageAndState(t *testing.T) {
	t.Run("usage shown when task missing", func(t *testing.T) {
		m := &model{}

		result, cmd, handled := ExecuteCommand(context.Background(), m, "/plan")
		if !handled {
			t.Fatal("expected /plan to be handled")
		}
		if cmd != nil {
			t.Fatal("did not expect follow-up command")
		}
		if result == "" || result[:12] != "Usage: /plan" {
			t.Fatalf("unexpected usage result: %q", result)
		}
	})

	t.Run("plan mode enabled when task provided", func(t *testing.T) {
		m := &model{
			mode: appmode.State{
				SessionPermissions: config.NewSessionPermissions(),
			},
		}

		result, cmd, handled := ExecuteCommand(context.Background(), m, "/plan audit regression coverage")
		if !handled {
			t.Fatal("expected /plan to be handled")
		}
		if cmd != nil {
			t.Fatal("did not expect follow-up command")
		}
		if m.mode.Operation != appmode.Plan || !m.mode.Enabled {
			t.Fatalf("expected plan mode enabled, got operation=%v enabled=%v", m.mode.Operation, m.mode.Enabled)
		}
		if m.mode.Task != "audit regression coverage" {
			t.Fatalf("unexpected plan task %q", m.mode.Task)
		}
		if m.mode.Store == nil {
			t.Fatal("expected plan store to be initialized")
		}
		if !strings.Contains(result, "Entering plan mode for: audit regression coverage") {
			t.Fatalf("unexpected result: %q", result)
		}
		if m.mode.SessionPermissions.AllowAllEdits || m.mode.SessionPermissions.AllowAllWrites || m.mode.SessionPermissions.AllowAllBash || m.mode.SessionPermissions.AllowAllSkills {
			t.Fatal("plan mode should reset permissive session flags")
		}
	})
}

func TestExecuteCommandOpenSelectors(t *testing.T) {
	t.Run("tools opens selector", func(t *testing.T) {
		m := &model{width: 80, height: 24}

		result, cmd, handled := ExecuteCommand(context.Background(), m, "/tools")
		if !handled {
			t.Fatal("expected /tools to be handled")
		}
		if result != "" || cmd != nil {
			t.Fatalf("unexpected command outputs: result=%q cmd=%v", result, cmd != nil)
		}
		if !m.tool.Selector.IsActive() {
			t.Fatal("expected tool selector to become active")
		}
	})

	t.Run("resume opens session selector when sessions exist", func(t *testing.T) {
		tmpHome := t.TempDir()
		tmpDir := t.TempDir()
		t.Setenv("HOME", tmpHome)

		store, err := session.NewStore(tmpDir)
		if err != nil {
			t.Fatalf("NewStore(): %v", err)
		}
		err = store.Save(&session.Session{
			Metadata: session.SessionMetadata{
				Title: "Resume me",
				Cwd:   tmpDir,
			},
			Entries: []session.Entry{
				{
					Type: session.EntryUser,
					Message: &session.EntryMessage{
						Role: "user",
						Content: []session.ContentBlock{
							{Type: "text", Text: "hello"},
						},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("Save(): %v", err)
		}

		m := &model{
			cwd:     tmpDir,
			width:   80,
			height:  24,
			session: appsession.State{Store: store},
		}

		result, cmd, handled := ExecuteCommand(context.Background(), m, "/resume")
		if !handled {
			t.Fatal("expected /resume to be handled")
		}
		if result != "" || cmd != nil {
			t.Fatalf("unexpected command outputs: result=%q cmd=%v", result, cmd != nil)
		}
		if !m.session.Selector.IsActive() {
			t.Fatal("expected session selector to become active")
		}
	})
}
