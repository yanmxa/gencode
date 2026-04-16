package pluginui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreplugin "github.com/yanmxa/gencode/internal/extension/plugin"
)

func TestHandleCommandMarketplaceAddListRemove(t *testing.T) {
	prev := coreplugin.DefaultRegistry
	coreplugin.DefaultRegistry = coreplugin.NewRegistry()
	t.Cleanup(func() { coreplugin.DefaultRegistry = prev })

	tmpHome := t.TempDir()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpHome)

	marketplaceDir := createTestMarketplace(t)
	selector := New()

	result, err := HandleCommand(context.Background(), &selector, tmpDir, 80, 24, "marketplace add "+marketplaceDir+" openai-codex")
	if err != nil {
		t.Fatalf("HandleCommand(add) error = %v", err)
	}
	if !strings.Contains(result, "Added marketplace 'openai-codex'.") {
		t.Fatalf("unexpected add result: %q", result)
	}

	result, err = HandleCommand(context.Background(), &selector, tmpDir, 80, 24, "marketplace list")
	if err != nil {
		t.Fatalf("HandleCommand(list) error = %v", err)
	}
	if !strings.Contains(result, "openai-codex (directory)") {
		t.Fatalf("list result missing marketplace entry: %q", result)
	}
	if !strings.Contains(result, marketplaceDir) {
		t.Fatalf("list result missing marketplace path: %q", result)
	}

	result, err = HandleCommand(context.Background(), &selector, tmpDir, 80, 24, "marketplace remove openai-codex")
	if err != nil {
		t.Fatalf("HandleCommand(remove) error = %v", err)
	}
	if !strings.Contains(result, "Removed marketplace 'openai-codex'.") {
		t.Fatalf("unexpected remove result: %q", result)
	}
}

func TestHandleCommandInstallFromMarketplace(t *testing.T) {
	prev := coreplugin.DefaultRegistry
	coreplugin.DefaultRegistry = coreplugin.NewRegistry()
	t.Cleanup(func() { coreplugin.DefaultRegistry = prev })

	tmpHome := t.TempDir()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpHome)

	marketplaceDir := createTestMarketplace(t)
	selector := New()

	if _, err := HandleCommand(context.Background(), &selector, tmpDir, 80, 24, "marketplace add "+marketplaceDir+" openai-codex"); err != nil {
		t.Fatalf("HandleCommand(add) error = %v", err)
	}

	result, err := HandleCommand(context.Background(), &selector, tmpDir, 80, 24, "install codex@openai-codex")
	if err != nil {
		t.Fatalf("HandleCommand(install) error = %v", err)
	}
	if !strings.Contains(result, "Installed plugin 'codex@openai-codex'") {
		t.Fatalf("unexpected install result: %q", result)
	}
	if !strings.Contains(result, "/reload-plugins") {
		t.Fatalf("install result should mention reload command: %q", result)
	}

	if _, ok := coreplugin.DefaultRegistry.Get("codex@openai-codex"); !ok {
		t.Fatal("expected installed plugin to be registered")
	}
}

func createTestMarketplace(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "codex", ".gen-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", pluginDir, err)
	}

	manifest := `{"name":"codex","description":"Test marketplace plugin"}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile(plugin.json): %v", err)
	}

	return root
}
