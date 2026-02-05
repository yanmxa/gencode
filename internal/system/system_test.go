package system

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveImports(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gencode-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	mainContent := `# Main File
@imported.md
Some content after import`

	importedContent := `## Imported Content
This was imported from another file.`

	if err := os.WriteFile(filepath.Join(tmpDir, "main.md"), []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "imported.md"), []byte(importedContent), 0644); err != nil {
		t.Fatalf("Failed to write imported.md: %v", err)
	}

	// Test import resolution
	seen := make(map[string]bool)
	result := resolveImports(mainContent, tmpDir, 0, seen)

	// Verify import was resolved
	if !strings.Contains(result, "<!-- Imported: imported.md -->") {
		t.Errorf("Expected import comment, got: %s", result)
	}
	if !strings.Contains(result, "This was imported from another file.") {
		t.Errorf("Expected imported content, got: %s", result)
	}
	if !strings.Contains(result, "Some content after import") {
		t.Errorf("Expected content after import, got: %s", result)
	}
}

func TestResolveImportsCycle(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gencode-test-cycle")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files that reference each other (cycle)
	file1Content := `# File 1
@file2.md`

	file2Content := `# File 2
@file1.md`

	if err := os.WriteFile(filepath.Join(tmpDir, "file1.md"), []byte(file1Content), 0644); err != nil {
		t.Fatalf("Failed to write file1.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.md"), []byte(file2Content), 0644); err != nil {
		t.Fatalf("Failed to write file2.md: %v", err)
	}

	// Test cycle detection
	seen := make(map[string]bool)
	seen[filepath.Join(tmpDir, "file1.md")] = true // Simulate file1.md already seen
	result := resolveImports(file1Content, tmpDir, 0, seen)

	// file2 should be imported, but file1 should be skipped (cycle)
	if !strings.Contains(result, "# File 2") {
		t.Errorf("Expected file2 content, got: %s", result)
	}
	// The cycle comment includes the @ prefix from the original match
	if !strings.Contains(result, "Skipped (cycle)") {
		t.Errorf("Expected cycle skip comment, got: %s", result)
	}
}

func TestResolveImportsNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gencode-test-notfound")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `# Test
@nonexistent.md`

	seen := make(map[string]bool)
	result := resolveImports(content, tmpDir, 0, seen)

	// The not found comment includes the path from the match
	if !strings.Contains(result, "Import not found") {
		t.Errorf("Expected not found comment, got: %s", result)
	}
}

func TestResolveImportsMaxDepth(t *testing.T) {
	content := `@deep.md`

	seen := make(map[string]bool)
	// Start at max depth - should not process imports
	result := resolveImports(content, "/tmp", maxImportDepth, seen)

	// Should return content unchanged
	if result != content {
		t.Errorf("Expected unchanged content at max depth, got: %s", result)
	}
}

func TestLoadRulesDirectory(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gencode-test-rules")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	rulesDir := filepath.Join(tmpDir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatalf("Failed to create rules dir: %v", err)
	}

	// Create rule files
	if err := os.WriteFile(filepath.Join(rulesDir, "coding.md"), []byte("# Coding Rules"), 0644); err != nil {
		t.Fatalf("Failed to write coding.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "security.md"), []byte("# Security Rules"), 0644); err != nil {
		t.Fatalf("Failed to write security.md: %v", err)
	}
	// Non-md file should be ignored
	if err := os.WriteFile(filepath.Join(rulesDir, "readme.txt"), []byte("Ignore me"), 0644); err != nil {
		t.Fatalf("Failed to write readme.txt: %v", err)
	}

	seen := make(map[string]bool)
	files := loadRulesDirectory(rulesDir, "project", seen)

	if len(files) != 2 {
		t.Errorf("Expected 2 rule files, got %d", len(files))
	}

	// Check files are loaded in alphabetical order
	if len(files) > 0 && !strings.Contains(files[0].Path, "coding.md") {
		t.Errorf("Expected coding.md first (alphabetical), got: %s", files[0].Path)
	}
	if len(files) > 1 && !strings.Contains(files[1].Path, "security.md") {
		t.Errorf("Expected security.md second, got: %s", files[1].Path)
	}
}

func TestGetAllMemoryPaths(t *testing.T) {
	cwd := "/test/project"
	paths := GetAllMemoryPaths(cwd)

	// Check project paths
	if len(paths.Project) != 4 {
		t.Errorf("Expected 4 project paths, got %d", len(paths.Project))
	}

	// Check local paths
	if len(paths.Local) != 1 {
		t.Errorf("Expected 1 local path, got %d", len(paths.Local))
	}
	if !strings.Contains(paths.Local[0], "GEN.local.md") {
		t.Errorf("Expected GEN.local.md in local paths, got: %s", paths.Local[0])
	}

	// Check rules directory paths
	if !strings.Contains(paths.ProjectRules, "rules") {
		t.Errorf("Expected rules in project rules path, got: %s", paths.ProjectRules)
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{500, "500B"},
		{1024, "1.0KB"},
		{2048, "2.0KB"},
		{1024 * 1024, "1.0MB"},
		{1536 * 1024, "1.5MB"},
	}

	for _, tc := range tests {
		result := FormatFileSize(tc.size)
		if result != tc.expected {
			t.Errorf("FormatFileSize(%d) = %s, expected %s", tc.size, result, tc.expected)
		}
	}
}

func TestResolveImportsNested(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gencode-test-nested")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested import chain: a.md -> b.md -> c.md
	aContent := `# Level A
@b.md
After B import`

	bContent := `## Level B
@c.md
After C import`

	cContent := `### Level C
Deepest content`

	if err := os.WriteFile(filepath.Join(tmpDir, "a.md"), []byte(aContent), 0644); err != nil {
		t.Fatalf("Failed to write a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.md"), []byte(bContent), 0644); err != nil {
		t.Fatalf("Failed to write b.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "c.md"), []byte(cContent), 0644); err != nil {
		t.Fatalf("Failed to write c.md: %v", err)
	}

	// Test nested import resolution
	seen := make(map[string]bool)
	result := resolveImports(aContent, tmpDir, 0, seen)

	// Verify all levels are imported
	if !strings.Contains(result, "<!-- Imported: b.md -->") {
		t.Errorf("Expected b.md import comment, got: %s", result)
	}
	if !strings.Contains(result, "<!-- Imported: c.md -->") {
		t.Errorf("Expected c.md import comment, got: %s", result)
	}
	if !strings.Contains(result, "Deepest content") {
		t.Errorf("Expected deepest content from c.md, got: %s", result)
	}
	if !strings.Contains(result, "After C import") {
		t.Errorf("Expected content after C import from b.md, got: %s", result)
	}
	if !strings.Contains(result, "After B import") {
		t.Errorf("Expected content after B import from a.md, got: %s", result)
	}
}

func TestResolveImportsRelativePath(t *testing.T) {
	// Create temp directory with subdirectory
	tmpDir, err := os.MkdirTemp("", "gencode-test-relative")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create files with relative path import
	mainContent := `# Main
@./subdir/nested.md`

	nestedContent := `## Nested
Nested content here`

	if err := os.WriteFile(filepath.Join(tmpDir, "main.md"), []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.md"), []byte(nestedContent), 0644); err != nil {
		t.Fatalf("Failed to write nested.md: %v", err)
	}

	// Test relative path import resolution
	seen := make(map[string]bool)
	result := resolveImports(mainContent, tmpDir, 0, seen)

	if !strings.Contains(result, "<!-- Imported: ./subdir/nested.md -->") {
		t.Errorf("Expected nested import comment, got: %s", result)
	}
	if !strings.Contains(result, "Nested content here") {
		t.Errorf("Expected nested content, got: %s", result)
	}
}

func TestLoadMemoryFilesWithImports(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gencode-test-memory-imports")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .gen directory
	genDir := filepath.Join(tmpDir, ".gen")
	if err := os.MkdirAll(genDir, 0755); err != nil {
		t.Fatalf("Failed to create .gen dir: %v", err)
	}

	// Create GEN.md with import
	genMdContent := `# Project Memory
@extra.md
End of memory`

	extraContent := `## Extra Content
This was imported`

	if err := os.WriteFile(filepath.Join(genDir, "GEN.md"), []byte(genMdContent), 0644); err != nil {
		t.Fatalf("Failed to write GEN.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "extra.md"), []byte(extraContent), 0644); err != nil {
		t.Fatalf("Failed to write extra.md: %v", err)
	}

	// Load memory files
	files := LoadMemoryFiles(tmpDir)

	// Should have at least one project file
	var projectFile *MemoryFile
	for i := range files {
		if files[i].Level == "project" && strings.Contains(files[i].Path, "GEN.md") {
			projectFile = &files[i]
			break
		}
	}

	if projectFile == nil {
		t.Fatal("Expected to find project GEN.md file")
	}

	// Verify import was resolved in content
	if !strings.Contains(projectFile.Content, "<!-- Imported: extra.md -->") {
		t.Errorf("Expected import comment in content, got: %s", projectFile.Content)
	}
	if !strings.Contains(projectFile.Content, "This was imported") {
		t.Errorf("Expected imported content, got: %s", projectFile.Content)
	}
}

func TestFindMemoryFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gencode-test-find")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create one file
	existingFile := filepath.Join(tmpDir, "exists.md")
	if err := os.WriteFile(existingFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	tests := []struct {
		name     string
		paths    []string
		expected string
	}{
		{
			name:     "first existing file wins",
			paths:    []string{filepath.Join(tmpDir, "notexist.md"), existingFile},
			expected: existingFile,
		},
		{
			name:     "no files exist",
			paths:    []string{filepath.Join(tmpDir, "a.md"), filepath.Join(tmpDir, "b.md")},
			expected: "",
		},
		{
			name:     "empty paths",
			paths:    []string{},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := FindMemoryFile(tc.paths)
			if result != tc.expected {
				t.Errorf("FindMemoryFile() = %q, expected %q", result, tc.expected)
			}
		})
	}
}
