package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetLatestByCwd(t *testing.T) {
	// Create a temp dir for the test store
	tmpDir, err := os.MkdirTemp("", "session-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir}

	// Create sessions in different directories
	sessA := &Session{
		Metadata: SessionMetadata{
			ID:        "sess-a",
			Cwd:       "/projects/alpha",
			CreatedAt: time.Now().Add(-3 * time.Minute),
			UpdatedAt: time.Now().Add(-3 * time.Minute),
		},
		Messages: []StoredMessage{{Role: "user", Content: "hello from alpha"}},
	}
	sessB := &Session{
		Metadata: SessionMetadata{
			ID:        "sess-b",
			Cwd:       "/projects/beta",
			CreatedAt: time.Now().Add(-2 * time.Minute),
			UpdatedAt: time.Now().Add(-2 * time.Minute),
		},
		Messages: []StoredMessage{{Role: "user", Content: "hello from beta"}},
	}
	sessA2 := &Session{
		Metadata: SessionMetadata{
			ID:        "sess-a2",
			Cwd:       "/projects/alpha",
			CreatedAt: time.Now().Add(-1 * time.Minute),
			UpdatedAt: time.Now().Add(-1 * time.Minute),
		},
		Messages: []StoredMessage{{Role: "user", Content: "second alpha session"}},
	}

	// Save all sessions
	for _, s := range []*Session{sessA, sessB, sessA2} {
		if err := store.Save(s); err != nil {
			t.Fatalf("failed to save session %s: %v", s.Metadata.ID, err)
		}
	}

	// Test: GetLatestByCwd for /projects/alpha should return sess-a2 (most recent)
	result, err := store.GetLatestByCwd("/projects/alpha")
	if err != nil {
		t.Fatalf("GetLatestByCwd failed: %v", err)
	}
	if result.Metadata.ID != "sess-a2" {
		t.Errorf("expected sess-a2, got %s", result.Metadata.ID)
	}

	// Test: GetLatestByCwd for /projects/beta should return sess-b
	result, err = store.GetLatestByCwd("/projects/beta")
	if err != nil {
		t.Fatalf("GetLatestByCwd failed: %v", err)
	}
	if result.Metadata.ID != "sess-b" {
		t.Errorf("expected sess-b, got %s", result.Metadata.ID)
	}

	// Test: GetLatestByCwd for non-existent directory should return error
	_, err = store.GetLatestByCwd("/projects/gamma")
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}

	// Test: GetLatest (global) should return sess-a2 (most recent overall)
	result, err = store.GetLatest()
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if result.Metadata.ID != "sess-a2" {
		t.Errorf("expected sess-a2 (global latest), got %s", result.Metadata.ID)
	}
}

func TestValidatePlanID(t *testing.T) {
	// This tests the fix in plan package but we import it here for convenience
	// Actually ValidatePlanID is in plan package, test it separately
}

func TestStoreCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-cleanup-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir}

	// Save sessions first (Save overrides UpdatedAt to now)
	oldSess := &Session{
		Metadata: SessionMetadata{
			ID:  "old-sess",
			Cwd: "/projects/old",
		},
		Messages: []StoredMessage{{Role: "user", Content: "old"}},
	}
	if err := store.Save(oldSess); err != nil {
		t.Fatal(err)
	}

	newSess := &Session{
		Metadata: SessionMetadata{
			ID:  "new-sess",
			Cwd: "/projects/new",
		},
		Messages: []StoredMessage{{Role: "user", Content: "new"}},
	}
	if err := store.Save(newSess); err != nil {
		t.Fatal(err)
	}

	// Now manually overwrite the old session file with an old UpdatedAt timestamp
	// so cleanup will detect it as expired
	oldSess.Metadata.UpdatedAt = time.Now().AddDate(0, 0, -(SessionRetentionDays + 1))
	oldSess.Metadata.CreatedAt = oldSess.Metadata.UpdatedAt
	data, _ := json.MarshalIndent(oldSess, "", "  ")
	os.WriteFile(filepath.Join(tmpDir, "old-sess.json"), data, 0644)

	// Run cleanup
	if err := store.Cleanup(); err != nil {
		t.Fatal(err)
	}

	// Old session should be deleted
	_, err = os.Stat(filepath.Join(tmpDir, "old-sess.json"))
	if !os.IsNotExist(err) {
		t.Error("expected old session to be cleaned up")
	}

	// New session should still exist
	_, err = os.Stat(filepath.Join(tmpDir, "new-sess.json"))
	if err != nil {
		t.Error("expected new session to still exist")
	}
}
