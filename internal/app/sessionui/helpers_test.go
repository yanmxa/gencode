package sessionui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yanmxa/gencode/internal/session"
)

func TestEncodePath(t *testing.T) {
	got := session.EncodePath("/tmp/project/nested/")
	if got != "-tmp-project-nested" {
		t.Fatalf("EncodePath() = %q, want %q", got, "-tmp-project-nested")
	}
}

func TestGenerateSessionIDFormat(t *testing.T) {
	got := session.NewSessionID()
	pattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	if !pattern.MatchString(got) {
		t.Fatalf("NewSessionID() = %q, does not match UUID-like format", got)
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "main" {
		t.Fatalf("GetGitBranch() = %q, want %q", got, "main")
	}
}
