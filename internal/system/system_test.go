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

	if err := os.WriteFile(filepath.Join(tmpDir, "main.md"), []byte(mainContent), 0o644); err != nil {
		t.Fatalf("Failed to write main.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "imported.md"), []byte(importedContent), 0o644); err != nil {
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

	if err := os.WriteFile(filepath.Join(tmpDir, "file1.md"), []byte(file1Content), 0o644); err != nil {
		t.Fatalf("Failed to write file1.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.md"), []byte(file2Content), 0o644); err != nil {
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
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatalf("Failed to create rules dir: %v", err)
	}

	// Create rule files
	if err := os.WriteFile(filepath.Join(rulesDir, "coding.md"), []byte("# Coding Rules"), 0o644); err != nil {
		t.Fatalf("Failed to write coding.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "security.md"), []byte("# Security Rules"), 0o644); err != nil {
		t.Fatalf("Failed to write security.md: %v", err)
	}
	// Non-md file should be ignored
	if err := os.WriteFile(filepath.Join(rulesDir, "readme.txt"), []byte("Ignore me"), 0o644); err != nil {
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

	if err := os.WriteFile(filepath.Join(tmpDir, "a.md"), []byte(aContent), 0o644); err != nil {
		t.Fatalf("Failed to write a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.md"), []byte(bContent), 0o644); err != nil {
		t.Fatalf("Failed to write b.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "c.md"), []byte(cContent), 0o644); err != nil {
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
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create files with relative path import
	mainContent := `# Main
@./subdir/nested.md`

	nestedContent := `## Nested
Nested content here`

	if err := os.WriteFile(filepath.Join(tmpDir, "main.md"), []byte(mainContent), 0o644); err != nil {
		t.Fatalf("Failed to write main.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.md"), []byte(nestedContent), 0o644); err != nil {
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
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatalf("Failed to create .gen dir: %v", err)
	}

	// Create GEN.md with import
	genMdContent := `# Project Memory
@extra.md
End of memory`

	extraContent := `## Extra Content
This was imported`

	if err := os.WriteFile(filepath.Join(genDir, "GEN.md"), []byte(genMdContent), 0o644); err != nil {
		t.Fatalf("Failed to write GEN.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "extra.md"), []byte(extraContent), 0o644); err != nil {
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
	if err := os.WriteFile(existingFile, []byte("content"), 0o644); err != nil {
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

func TestPromptCaching(t *testing.T) {
	s := &System{
		Cwd:   "/tmp/test",
		IsGit: true,
	}

	// First call builds the prompt
	first := s.Prompt()
	if first == "" {
		t.Error("First Prompt() call should return non-empty string")
	}

	// Second call returns cached
	second := s.Prompt()
	if first != second {
		t.Error("Second Prompt() call should return same cached result")
	}

	// After Invalidate, should rebuild
	s.Invalidate()
	third := s.Prompt()
	if third == "" {
		t.Error("Prompt() after Invalidate() should return non-empty string")
	}
}

func TestPromptContainsInstructions(t *testing.T) {
	s := &System{
		Cwd:                 "/tmp/test",
		UserInstructions:    "Always use tabs for indentation.",
		ProjectInstructions: "This is a Go project using Bubble Tea.",
	}

	prompt := s.Prompt()

	if !strings.Contains(prompt, "<user-instructions>") {
		t.Error("prompt should contain <user-instructions> tag")
	}
	if !strings.Contains(prompt, "Always use tabs for indentation.") {
		t.Error("prompt should contain user instructions content")
	}
	if !strings.Contains(prompt, "<project-instructions>") {
		t.Error("prompt should contain <project-instructions> tag")
	}
	if !strings.Contains(prompt, "This is a Go project using Bubble Tea.") {
		t.Error("prompt should contain project instructions content")
	}
}

func TestPromptDirectFields(t *testing.T) {
	s := &System{
		Cwd:            "/tmp/test",
		SessionSummary: "<session-summary>\nRefactored core.\n</session-summary>",
		Skills:         "<available-skills>\n- commit\n</available-skills>",
		Agents:         "<available-agents>\n- Explore\n</available-agents>",
	}

	prompt := s.Prompt()

	if !strings.Contains(prompt, "<session-summary>") {
		t.Error("prompt should contain session-summary")
	}
	if !strings.Contains(prompt, "<available-skills>") {
		t.Error("prompt should contain skills")
	}
	if !strings.Contains(prompt, "<available-agents>") {
		t.Error("prompt should contain agents")
	}
}

func TestPromptExtra(t *testing.T) {
	s := &System{
		Cwd:   "/tmp/test",
		Extra: []string{"agent identity content here"},
	}

	prompt := s.Prompt()

	if !strings.Contains(prompt, "agent identity content here") {
		t.Error("prompt should contain Extra content")
	}
}

func TestPromptNarrativeOrder(t *testing.T) {
	s := &System{
		Cwd:                 "/tmp/test",
		UserInstructions:    "USER_INSTRUCTIONS_MARKER",
		ProjectInstructions: "PROJECT_INSTRUCTIONS_MARKER",
		SessionSummary:      "<session-summary>\nSESSION_SUMMARY_MARKER\n</session-summary>",
		Skills:              "<available-skills>\nSKILLS_MARKER\n</available-skills>",
		Agents:              "<available-agents>\nAGENTS_MARKER\n</available-agents>",
		Extra:               []string{"EXTRA_MARKER"},
	}

	prompt := s.Prompt()

	// Verify the 7-layer narrative order:
	// 1. Identity (base.txt) < 2. Environment < 3. Instructions
	// < 4. Summary < 5. Capabilities < 6. Guidelines < 7. Extra
	envIdx := strings.Index(prompt, "<env>")
	userIdx := strings.Index(prompt, "USER_INSTRUCTIONS_MARKER")
	projectIdx := strings.Index(prompt, "PROJECT_INSTRUCTIONS_MARKER")
	summaryIdx := strings.Index(prompt, "SESSION_SUMMARY_MARKER")
	skillsIdx := strings.Index(prompt, "SKILLS_MARKER")
	agentsIdx := strings.Index(prompt, "AGENTS_MARKER")
	extraIdx := strings.Index(prompt, "EXTRA_MARKER")

	if envIdx < 0 || userIdx < 0 || projectIdx < 0 || summaryIdx < 0 ||
		skillsIdx < 0 || agentsIdx < 0 || extraIdx < 0 {
		t.Fatal("prompt is missing one or more expected sections")
	}

	// Environment before Instructions
	if envIdx >= userIdx {
		t.Error("environment should appear before user instructions")
	}
	// User instructions before project instructions
	if userIdx >= projectIdx {
		t.Error("user instructions should appear before project instructions")
	}
	// Instructions before summary
	if projectIdx >= summaryIdx {
		t.Error("project instructions should appear before session summary")
	}
	// Summary before capabilities
	if summaryIdx >= skillsIdx {
		t.Error("session summary should appear before skills")
	}
	if skillsIdx >= agentsIdx {
		t.Error("skills should appear before agents")
	}
	// Extra at the end
	if agentsIdx >= extraIdx {
		t.Error("agents should appear before extra content")
	}
}

func TestPromptPlanMode(t *testing.T) {
	// PlanMode=true should include planmode content
	s := &System{
		Cwd:      "/tmp/test",
		PlanMode: true,
	}
	prompt := s.Prompt()

	if !strings.Contains(prompt, "plan") && !strings.Contains(prompt, "Plan") {
		t.Error("PlanMode=true should include plan mode content in prompt")
	}

	// PlanMode=false should not include planmode content
	s2 := &System{
		Cwd:      "/tmp/test",
		PlanMode: false,
	}
	prompt2 := s2.Prompt()

	// The planmode.txt content is only appended when PlanMode=true.
	// Verify PlanMode prompt has more content than non-PlanMode.
	if len(prompt) <= len(prompt2) {
		t.Error("PlanMode=true prompt should be longer than PlanMode=false prompt")
	}
}

func TestPromptEmptyFieldsExcluded(t *testing.T) {
	s := &System{
		Cwd: "/tmp/test",
		// All optional fields empty
	}
	prompt := s.Prompt()

	if strings.Contains(prompt, "<user-instructions>") {
		t.Error("empty UserInstructions should not produce <user-instructions> tag")
	}
	if strings.Contains(prompt, "<project-instructions>") {
		t.Error("empty ProjectInstructions should not produce <project-instructions> tag")
	}
	if strings.Contains(prompt, "<session-summary>") {
		t.Error("empty SessionSummary should not produce <session-summary> tag")
	}
	if strings.Contains(prompt, "<available-skills>") {
		t.Error("empty Skills should not produce <available-skills> tag")
	}
	if strings.Contains(prompt, "<available-agents>") {
		t.Error("empty Agents should not produce <available-agents> tag")
	}
}

func TestPromptInitCachedFiles(t *testing.T) {
	// Verify init() pre-cached the embedded prompt files
	if cachedBase == "" {
		t.Error("cachedBase should be non-empty after init()")
	}
	if cachedTools == "" {
		t.Error("cachedTools should be non-empty after init()")
	}
	if cachedPlanMode == "" {
		t.Error("cachedPlanMode should be non-empty after init()")
	}

	// Verify all prompts appear in the assembled result
	s := &System{Cwd: "/tmp/test"}
	prompt := s.Prompt()

	if !strings.Contains(prompt, cachedBase[:50]) {
		t.Error("prompt should contain base.txt content")
	}
	if !strings.Contains(prompt, cachedTools[:50]) {
		t.Error("prompt should contain tools content")
	}
}

func TestLoadInstructions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gencode-test-instructions")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create project-level GEN.md
	genDir := filepath.Join(tmpDir, ".gen")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatalf("Failed to create .gen dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "GEN.md"), []byte("Project instructions here"), 0o644); err != nil {
		t.Fatalf("Failed to write GEN.md: %v", err)
	}

	// Create local file
	if err := os.WriteFile(filepath.Join(genDir, "GEN.local.md"), []byte("Local instructions here"), 0o644); err != nil {
		t.Fatalf("Failed to write GEN.local.md: %v", err)
	}

	user, project := LoadInstructions(tmpDir)

	// Project and local should be in 'project' output
	if !strings.Contains(project, "Project instructions here") {
		t.Errorf("project instructions should contain GEN.md content, got: %s", project)
	}
	if !strings.Contains(project, "Local instructions here") {
		t.Errorf("project instructions should contain GEN.local.md content, got: %s", project)
	}

	// User instructions come from ~/.gen/GEN.md which we didn't create in tmpDir,
	// so they may or may not be empty depending on the test environment.
	// Just verify the function returns without error.
	_ = user
}

func TestLoadMemoryFiles_PrefersGenPathsAndPreservesSectionOrder(t *testing.T) {
	tmpHome := t.TempDir()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpHome)

	userGenDir := filepath.Join(tmpHome, ".gen")
	userClaudeDir := filepath.Join(tmpHome, ".claude")
	projectGenDir := filepath.Join(tmpDir, ".gen")
	projectClaudeDir := filepath.Join(tmpDir, ".claude")

	for _, dir := range []string{userGenDir, userClaudeDir, projectGenDir, projectClaudeDir, filepath.Join(userGenDir, "rules"), filepath.Join(projectGenDir, "rules")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}

	if err := os.WriteFile(filepath.Join(userGenDir, "GEN.md"), []byte("user gen"), 0o644); err != nil {
		t.Fatalf("WriteFile(user GEN): %v", err)
	}
	if err := os.WriteFile(filepath.Join(userClaudeDir, "CLAUDE.md"), []byte("user claude fallback"), 0o644); err != nil {
		t.Fatalf("WriteFile(user CLAUDE): %v", err)
	}
	if err := os.WriteFile(filepath.Join(userGenDir, "rules", "01-global.md"), []byte("global rule"), 0o644); err != nil {
		t.Fatalf("WriteFile(global rule): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectGenDir, "GEN.md"), []byte("project gen"), 0o644); err != nil {
		t.Fatalf("WriteFile(project GEN): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectClaudeDir, "CLAUDE.md"), []byte("project claude fallback"), 0o644); err != nil {
		t.Fatalf("WriteFile(project CLAUDE): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectGenDir, "rules", "01-project.md"), []byte("project rule"), 0o644); err != nil {
		t.Fatalf("WriteFile(project rule): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectGenDir, "GEN.local.md"), []byte("project local"), 0o644); err != nil {
		t.Fatalf("WriteFile(local GEN): %v", err)
	}

	files := LoadMemoryFiles(tmpDir)
	if len(files) != 5 {
		t.Fatalf("expected 5 memory files, got %d", len(files))
	}

	if files[0].Level != "global" || !strings.Contains(files[0].Path, filepath.Join(".gen", "GEN.md")) {
		t.Fatalf("expected global GEN.md first, got level=%q path=%q", files[0].Level, files[0].Path)
	}
	if strings.Contains(files[0].Content, "user claude fallback") {
		t.Fatal("expected user .gen/GEN.md to take precedence over ~/.claude/CLAUDE.md")
	}
	if files[1].Level != "global" || files[1].Source != "rules" {
		t.Fatalf("expected global rules second, got level=%q source=%q", files[1].Level, files[1].Source)
	}
	if files[2].Level != "project" || !strings.Contains(files[2].Path, filepath.Join(".gen", "GEN.md")) {
		t.Fatalf("expected project GEN.md third, got level=%q path=%q", files[2].Level, files[2].Path)
	}
	if strings.Contains(files[2].Content, "project claude fallback") {
		t.Fatal("expected project .gen/GEN.md to take precedence over project CLAUDE.md")
	}
	if files[3].Level != "project" || files[3].Source != "rules" {
		t.Fatalf("expected project rules fourth, got level=%q source=%q", files[3].Level, files[3].Source)
	}
	if files[4].Level != "local" || !strings.Contains(files[4].Path, "GEN.local.md") {
		t.Fatalf("expected local GEN.local.md last, got level=%q path=%q", files[4].Level, files[4].Path)
	}
}

// TestMemory_ImportChain verifies that @import A which itself imports B results
// in both files being loaded and their content present in the final output.
func TestMemory_ImportChain(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gencode-test-import-chain")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	genDir := filepath.Join(tmpDir, ".gen")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatalf("Failed to create .gen dir: %v", err)
	}

	// GEN.md -> a.md -> b.md  (3-level chain)
	genMdContent := "# Root\n@a.md"
	aMdContent := "## Level A\n@b.md"
	bMdContent := "### Level B\nFinal content from B"

	if err := os.WriteFile(filepath.Join(genDir, "GEN.md"), []byte(genMdContent), 0o644); err != nil {
		t.Fatalf("Failed to write GEN.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "a.md"), []byte(aMdContent), 0o644); err != nil {
		t.Fatalf("Failed to write a.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(genDir, "b.md"), []byte(bMdContent), 0o644); err != nil {
		t.Fatalf("Failed to write b.md: %v", err)
	}

	files := LoadMemoryFiles(tmpDir)

	// Find the project memory file
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

	// Both A and B content should be present after chain resolution
	if !strings.Contains(projectFile.Content, "Level A") {
		t.Errorf("Expected 'Level A' content (from a.md) in resolved output; got: %s", projectFile.Content)
	}
	if !strings.Contains(projectFile.Content, "Final content from B") {
		t.Errorf("Expected 'Final content from B' (from b.md) in resolved output; got: %s", projectFile.Content)
	}
	// Import comments should also be present
	if !strings.Contains(projectFile.Content, "<!-- Imported: a.md -->") {
		t.Errorf("Expected import comment for a.md; got: %s", projectFile.Content)
	}
	if !strings.Contains(projectFile.Content, "<!-- Imported: b.md -->") {
		t.Errorf("Expected import comment for b.md; got: %s", projectFile.Content)
	}
}

// TestMemory_MissingFile_NoError verifies that the absence of GEN.md does not
// cause a panic or returned error — LoadMemoryFiles should return gracefully.
func TestMemory_MissingFile_NoError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gencode-test-missing-genmd")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Intentionally do NOT create .gen/GEN.md

	// This must not panic or crash
	files := LoadMemoryFiles(tmpDir)

	// No project-level file should be returned
	for _, f := range files {
		if f.Level == "project" && strings.Contains(f.Path, tmpDir) {
			t.Errorf("Did not expect a project memory file when GEN.md is absent, got: %s", f.Path)
		}
	}

	// LoadInstructions must also handle it without error
	_, project := LoadInstructions(tmpDir)
	// project may contain content from global user GEN.md; what matters is no panic
	// and no project-level content from our tmpDir
	_ = project
}
