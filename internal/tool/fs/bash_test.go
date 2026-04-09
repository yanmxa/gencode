package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBashToolTracksChangedDirectory(t *testing.T) {
	cwd := t.TempDir()
	subdir := filepath.Join(cwd, "subdir")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	result := (&BashTool{}).ExecuteApproved(context.Background(), map[string]any{
		"command": "cd subdir",
	}, cwd)
	if !result.Success {
		t.Fatalf("ExecuteApproved() failed: %s", result.Error)
	}

	resp, ok := result.HookResponse.(map[string]any)
	if !ok {
		t.Fatalf("expected hook response map, got %#v", result.HookResponse)
	}
	got, _ := resp["cwd"].(string)
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("EvalSymlinks(got) error = %v", err)
	}
	wantResolved, err := filepath.EvalSymlinks(subdir)
	if err != nil {
		t.Fatalf("EvalSymlinks(subdir) error = %v", err)
	}
	if gotResolved != wantResolved {
		t.Fatalf("tracked cwd = %q (%q), want %q (%q)", got, gotResolved, subdir, wantResolved)
	}
}
