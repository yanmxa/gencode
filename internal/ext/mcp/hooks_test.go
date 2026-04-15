package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

type testConfigObserver struct {
	events []struct {
		source   string
		filePath string
	}
}

func (o *testConfigObserver) ConfigChanged(source, filePath string) {
	o.events = append(o.events, struct {
		source   string
		filePath string
	}{source: source, filePath: filePath})
}

func TestScopeConfigSource(t *testing.T) {
	tests := []struct {
		scope Scope
		want  string
	}{
		{ScopeUser, "user_settings"},
		{ScopeProject, "project_settings"},
		{ScopeLocal, "local_settings"},
	}
	for _, tt := range tests {
		if got := scopeConfigSource(tt.scope); got != tt.want {
			t.Fatalf("scopeConfigSource(%q) = %q, want %q", tt.scope, got, tt.want)
		}
	}
}

func TestNotifyConfigChanged(t *testing.T) {
	observer := &testConfigObserver{}
	SetConfigObserver(observer)
	defer SetConfigObserver(nil)

	notifyConfigChanged("project_settings", "/tmp/mcp.json")
	if len(observer.events) != 1 {
		t.Fatalf("expected 1 config event, got %d", len(observer.events))
	}
	if observer.events[0].source != "project_settings" {
		t.Fatalf("unexpected source %q", observer.events[0].source)
	}
}

func TestConfigSourceFromFilePath(t *testing.T) {
	if got := configSourceFromFilePath("/repo/.gen/mcp.local.json"); got != "local_settings" {
		t.Fatalf("unexpected local source: %q", got)
	}
	if got := configSourceFromFilePath("/repo/.gen/mcp.json"); got != "project_settings" {
		t.Fatalf("unexpected project source: %q", got)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	userPath := filepath.Join(homeDir, ".gen", "mcp.json")
	if got := configSourceFromFilePath(userPath); got != "user_settings" {
		t.Fatalf("unexpected user source: %q", got)
	}
}
