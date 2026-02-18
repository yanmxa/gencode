package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillLazyLoading(t *testing.T) {
	// Create a temporary skill directory
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("Failed to create skill dir: %v", err)
	}

	// Create SKILL.md file
	skillFile := filepath.Join(skillDir, "SKILL.md")
	skillContent := `---
name: test-lazy-skill
description: A test skill for lazy loading verification
---

This is the full skill instructions content that should NOT be loaded at startup.
It contains detailed instructions for the skill.

## Instructions
1. Do something
2. Do something else

## Example
` + "```bash\necho 'hello world'\n```"

	if err := os.WriteFile(skillFile, []byte(skillContent), 0644); err != nil {
		t.Fatalf("Failed to create skill file: %v", err)
	}

	// Load the skill (simulating startup)
	loader := NewLoader(tmpDir)
	skill, err := loader.loadSkillFile(skillFile, ScopeUser, "")
	if err != nil {
		t.Fatalf("Failed to load skill file: %v", err)
	}

	// Verify metadata is loaded
	if skill.Name != "test-lazy-skill" {
		t.Errorf("Name not loaded: got %q", skill.Name)
	}
	if skill.Description != "A test skill for lazy loading verification" {
		t.Errorf("Description not loaded: got %q", skill.Description)
	}

	// Verify Instructions is NOT loaded at parse time (lazy loading)
	if skill.Instructions != "" {
		t.Errorf("Instructions should be empty at parse time, got: %q", skill.Instructions)
	}
	if skill.loaded {
		t.Error("loaded should be false at parse time")
	}

	// Now call GetInstructions() to trigger lazy loading
	instructions := skill.GetInstructions()

	// Verify the full content is now loaded
	if instructions == "" {
		t.Error("GetInstructions() should return the full content")
	}
	if !skill.loaded {
		t.Error("loaded should be true after GetInstructions()")
	}
	if skill.Instructions == "" {
		t.Error("Instructions should be populated after GetInstructions()")
	}

	// Verify it contains the expected content
	if len(instructions) < 50 {
		t.Errorf("Instructions seems too short: %d chars", len(instructions))
	}

	t.Logf("✓ Metadata loaded at startup (name=%s, desc=%s)",
		skill.Name, skill.Description[:20]+"...")
	t.Logf("✓ Instructions loaded lazily (%d chars)", len(instructions))
}
