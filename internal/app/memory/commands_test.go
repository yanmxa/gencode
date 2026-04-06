package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleInitCommand(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gencode-init-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	t.Run("init creates project memory file", func(t *testing.T) {
		result, err := HandleInitCommand(tmpDir, "")
		if err != nil {
			t.Fatalf("HandleInitCommand failed: %v", err)
		}

		expectedPath := filepath.Join(tmpDir, ".gen", "GEN.md")
		if !strings.Contains(result, "Created") {
			t.Errorf("Expected 'Created' in result, got: %s", result)
		}

		// Verify file exists
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Errorf("Expected file to exist: %s", expectedPath)
		}

		// Verify content
		content, err := os.ReadFile(expectedPath)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		if !strings.Contains(string(content), "# GEN.md") {
			t.Errorf("Expected template header in content")
		}
	})

	t.Run("init local creates local memory file", func(t *testing.T) {
		// Create .gitignore first
		gitignorePath := filepath.Join(tmpDir, ".gitignore")
		os.WriteFile(gitignorePath, []byte("# Ignore files\n"), 0o644)

		result, err := HandleInitCommand(tmpDir, "local")
		if err != nil {
			t.Fatalf("HandleInitCommand local failed: %v", err)
		}

		expectedPath := filepath.Join(tmpDir, ".gen", "GEN.local.md")
		if !strings.Contains(result, "Created") {
			t.Errorf("Expected 'Created' in result, got: %s", result)
		}
		if !strings.Contains(result, ".gitignore") {
			t.Errorf("Expected '.gitignore' mention in result, got: %s", result)
		}

		// Verify file exists
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Errorf("Expected file to exist: %s", expectedPath)
		}

		// Verify .gitignore was updated
		gitignoreContent, _ := os.ReadFile(gitignorePath)
		if !strings.Contains(string(gitignoreContent), "GEN.local.md") {
			t.Errorf("Expected GEN.local.md in .gitignore")
		}
	})

	t.Run("init rules creates rules directory", func(t *testing.T) {
		// Use a new temp dir for this test
		tmpDir2, _ := os.MkdirTemp("", "gencode-init-rules-test")
		defer os.RemoveAll(tmpDir2)

		result, err := HandleInitCommand(tmpDir2, "rules")
		if err != nil {
			t.Fatalf("HandleInitCommand rules failed: %v", err)
		}

		expectedDir := filepath.Join(tmpDir2, ".gen", "rules")
		if !strings.Contains(result, "Created") {
			t.Errorf("Expected 'Created' in result, got: %s", result)
		}

		// Verify directory exists
		info, err := os.Stat(expectedDir)
		if os.IsNotExist(err) {
			t.Errorf("Expected directory to exist: %s", expectedDir)
		}
		if err == nil && !info.IsDir() {
			t.Errorf("Expected %s to be a directory", expectedDir)
		}

		// Verify example file
		examplePath := filepath.Join(expectedDir, "example.md")
		if _, err := os.Stat(examplePath); os.IsNotExist(err) {
			t.Errorf("Expected example.md to exist: %s", examplePath)
		}
	})

	t.Run("init --claude creates claude memory file", func(t *testing.T) {
		tmpDir2, _ := os.MkdirTemp("", "gencode-init-claude-test")
		defer os.RemoveAll(tmpDir2)

		result, err := HandleInitCommand(tmpDir2, "--claude")
		if err != nil {
			t.Fatalf("HandleInitCommand --claude failed: %v", err)
		}

		expectedPath := filepath.Join(tmpDir2, ".claude", "CLAUDE.md")
		if !strings.Contains(result, "Created") {
			t.Errorf("Expected 'Created' in result, got: %s", result)
		}
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Errorf("Expected file to exist: %s", expectedPath)
		}
	})

	t.Run("init rules --claude creates claude rules directory", func(t *testing.T) {
		tmpDir2, _ := os.MkdirTemp("", "gencode-init-claude-rules-test")
		defer os.RemoveAll(tmpDir2)

		result, err := HandleInitCommand(tmpDir2, "rules --claude")
		if err != nil {
			t.Fatalf("HandleInitCommand rules --claude failed: %v", err)
		}

		expectedDir := filepath.Join(tmpDir2, ".claude", "rules")
		if !strings.Contains(result, "Created") {
			t.Errorf("Expected 'Created' in result, got: %s", result)
		}
		if info, err := os.Stat(expectedDir); os.IsNotExist(err) || !info.IsDir() {
			t.Errorf("Expected claude rules dir to exist: %s", expectedDir)
		}
	})
}

func TestHandleMemoryList(t *testing.T) {
	// Create temp directory with some memory files
	tmpDir, err := os.MkdirTemp("", "gencode-memory-list-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .gen/GEN.md
	genDir := filepath.Join(tmpDir, ".gen")
	os.MkdirAll(genDir, 0o755)
	os.WriteFile(filepath.Join(genDir, "GEN.md"), []byte("# Test Memory"), 0o644)

	result, err := HandleMemoryList(tmpDir)
	if err != nil {
		t.Fatalf("HandleMemoryList failed: %v", err)
	}

	// Verify output contains project section marker
	if !strings.Contains(result, "● Project") {
		t.Errorf("Expected '● Project' in result, got: %s", result)
	}

	// Verify output contains Total line (may include global files too)
	if !strings.Contains(result, "Total:") {
		t.Errorf("Expected 'Total:' in result, got: %s", result)
	}

	// Verify box alignment
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "│") && !strings.HasSuffix(line, "│") {
			t.Errorf("Line %d missing closing │: %q", i+1, line)
		}
	}
}
