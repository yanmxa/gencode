package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func TestEncodePath(t *testing.T) {
	got := encodePath("/tmp/project/nested/")
	if got != "-tmp-project-nested" {
		t.Fatalf("encodePath() = %q, want %q", got, "-tmp-project-nested")
	}
}

func TestGenerateSessionIDFormat(t *testing.T) {
	got := generateSessionID()
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !pattern.MatchString(got) {
		t.Fatalf("generateSessionID() = %q, does not match UUID-like format", got)
	}
}

func TestGetGitBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.name", "Test User")
	run("config", "user.email", "test@example.com")
	file := filepath.Join(dir, "README.md")
	if err := os.WriteFile(file, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(): %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")

	if got := getGitBranch(dir); got != "main" {
		t.Fatalf("getGitBranch() = %q, want %q", got, "main")
	}
}
