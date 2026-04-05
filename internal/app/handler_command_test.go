package app

import (
	"context"
	"testing"

	appcommand "github.com/yanmxa/gencode/internal/app/command"
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
