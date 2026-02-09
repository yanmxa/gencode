package plan

import (
	"os"
	"testing"
)

func TestValidatePlanID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		// Valid IDs
		{"20260209-add-dark-mode", true},
		{"20260209-plan", true},
		{"20260209-fix-login-bug-auth", true},
		{"abc-def", true},
		{"abc-def-ghi", true},
		{"a1-b2-c3", true},

		// Invalid IDs
		{"", false},
		{"single", false},           // no hyphen segments
		{"ABC-DEF", false},          // uppercase
		{"-abc-def", false},         // leading hyphen
		{"abc-def-", false},         // trailing hyphen (won't match since last segment is empty)
		{"abc--def", false},         // double hyphen
		{"abc def", false},          // space
		{"abc/def-ghi", false},      // slash
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := ValidatePlanID(tt.id)
			if got != tt.valid {
				t.Errorf("ValidatePlanID(%q) = %v, want %v", tt.id, got, tt.valid)
			}
		})
	}
}

func TestPlanSaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plan-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir}

	plan := &Plan{
		ID:      "20260209-test-plan",
		Task:    "Test plan saving",
		Status:  StatusDraft,
		Content: "## Summary\nThis is a test plan.\n\n## Steps\n1. Do something",
	}

	path, err := store.Save(plan)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if path == "" {
		t.Error("Save returned empty path")
	}

	// Load it back
	loaded, err := store.Load("20260209-test-plan")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ID != plan.ID {
		t.Errorf("ID mismatch: got %q, want %q", loaded.ID, plan.ID)
	}
	if loaded.Task != plan.Task {
		t.Errorf("Task mismatch: got %q, want %q", loaded.Task, plan.Task)
	}
	if loaded.Status != plan.Status {
		t.Errorf("Status mismatch: got %q, want %q", loaded.Status, plan.Status)
	}
	if loaded.Content != plan.Content {
		t.Errorf("Content mismatch:\ngot:  %q\nwant: %q", loaded.Content, plan.Content)
	}
}

func TestPlanSaveWithSpecialChars(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plan-special-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir}

	plan := &Plan{
		ID:      "20260209-special-chars",
		Task:    `Fix the "auth" module: handle edge cases`,
		Status:  StatusApproved,
		Content: "## Summary\nFix auth module.\n\n## Details\nHandle: colons, \"quotes\", and newlines\nin task",
	}

	_, err = store.Save(plan)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load("20260209-special-chars")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Task != plan.Task {
		t.Errorf("Task with special chars mismatch:\ngot:  %q\nwant: %q", loaded.Task, plan.Task)
	}
}

func TestGeneratePlanName(t *testing.T) {
	tests := []struct {
		task     string
		wantEnd  string // suffix the name should end with (date prefix varies)
	}{
		{"Add dark mode support", "-add-dark-mode-support"},
		{"", "-plan"},
		{"Fix the login bug", "-fix-login-bug"},
	}

	for _, tt := range tests {
		name := GeneratePlanName(tt.task)
		// Check it starts with a date
		if len(name) < 8 {
			t.Errorf("GeneratePlanName(%q) = %q, too short", tt.task, name)
			continue
		}
		// Check date prefix is all digits
		datePrefix := name[:8]
		for _, c := range datePrefix {
			if c < '0' || c > '9' {
				t.Errorf("GeneratePlanName(%q) date prefix %q contains non-digit", tt.task, datePrefix)
				break
			}
		}
		// Should be valid ID
		if !ValidatePlanID(name) {
			t.Errorf("GeneratePlanName(%q) = %q, not valid per ValidatePlanID", tt.task, name)
		}
	}
}

func TestPlanList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "plan-list-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store := &Store{baseDir: tmpDir}

	// Save two plans
	p1 := &Plan{ID: "20260209-plan-one", Task: "Plan one", Status: StatusDraft, Content: "Content one"}
	p2 := &Plan{ID: "20260209-plan-two", Task: "Plan two", Status: StatusApproved, Content: "Content two"}

	store.Save(p1)
	store.Save(p2)

	plans, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(plans) != 2 {
		t.Errorf("expected 2 plans, got %d", len(plans))
	}
}
