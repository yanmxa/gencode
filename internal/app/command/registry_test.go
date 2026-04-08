package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yanmxa/gencode/internal/plugin"
	"github.com/yanmxa/gencode/internal/skill"
)

func newTestSkillRegistry(t *testing.T, skills map[string]*skill.Skill) *skill.Registry {
	t.Helper()
	tmpDir := t.TempDir()
	userStore, err := skill.NewStore(filepath.Join(tmpDir, "user-skills.json"))
	if err != nil {
		t.Fatalf("NewStore(user): %v", err)
	}
	projectStore, err := skill.NewStore(filepath.Join(tmpDir, "project-skills.json"))
	if err != nil {
		t.Fatalf("NewStore(project): %v", err)
	}
	return skill.NewRegistryForTest(skills, userStore, projectStore)
}

func TestGetSkillCommands_OnlyEnabledAndIncludesArgumentHint(t *testing.T) {
	prev := skill.DefaultRegistry
	t.Cleanup(func() { skill.DefaultRegistry = prev })

	skill.DefaultRegistry = newTestSkillRegistry(t, map[string]*skill.Skill{
		"search": {
			Name:         "search",
			Description:  "Search files",
			ArgumentHint: "<pattern>",
			State:        skill.StateEnable,
		},
		"review": {
			Name:        "review",
			Description: "Review code",
			State:       skill.StateActive,
		},
		"hidden": {
			Name:        "hidden",
			Description: "Hidden skill",
			State:       skill.StateDisable,
		},
	})

	cmds := GetSkillCommands()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 enabled skill commands, got %d", len(cmds))
	}

	foundSearch := false
	for _, cmd := range cmds {
		if cmd.Name == "search" {
			foundSearch = true
			if cmd.Description != "Search files <pattern>" {
				t.Errorf("search description = %q, want %q", cmd.Description, "Search files <pattern>")
			}
		}
		if cmd.Name == "hidden" {
			t.Fatal("disabled skill should not appear in slash command list")
		}
	}
	if !foundSearch {
		t.Fatal("expected enabled search skill in slash command list")
	}
}

func TestIsSkillCommand_RejectsDisabledSkill(t *testing.T) {
	prev := skill.DefaultRegistry
	t.Cleanup(func() { skill.DefaultRegistry = prev })

	skill.DefaultRegistry = newTestSkillRegistry(t, map[string]*skill.Skill{
		"enabled": {
			Name:  "enabled",
			State: skill.StateEnable,
		},
		"disabled": {
			Name:  "disabled",
			State: skill.StateDisable,
		},
	})

	if _, ok := IsSkillCommand("enabled"); !ok {
		t.Fatal("expected enabled skill to be invocable as slash command")
	}
	if _, ok := IsSkillCommand("disabled"); ok {
		t.Fatal("disabled skill should not be invocable as slash command")
	}
}

func TestLoadCustomCommandFile_WithFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	cmdFile := filepath.Join(tmpDir, "deploy.md")
	content := "---\nname: deploy\ndescription: Deploy to production\n---\nRun the deploy pipeline.\n"
	if err := os.WriteFile(cmdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pc := loadCustomCommandFile(cmdFile, "myplug")
	if pc == nil {
		t.Fatal("expected plugin command to be loaded")
	}
	if pc.Name != "deploy" {
		t.Errorf("name = %q, want %q", pc.Name, "deploy")
	}
	if pc.Description != "Deploy to production" {
		t.Errorf("description = %q, want %q", pc.Description, "Deploy to production")
	}
	if pc.Namespace != "myplug" {
		t.Errorf("namespace = %q, want %q", pc.Namespace, "myplug")
	}
	if pc.FullName() != "myplug:deploy" {
		t.Errorf("fullName = %q, want %q", pc.FullName(), "myplug:deploy")
	}
	inst := pc.GetInstructions()
	if inst != "Run the deploy pipeline." {
		t.Errorf("instructions = %q, want %q", inst, "Run the deploy pipeline.")
	}
}

func TestLoadCustomCommandFile_WithoutFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	cmdFile := filepath.Join(tmpDir, "check.md")
	if err := os.WriteFile(cmdFile, []byte("Run health checks.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pc := loadCustomCommandFile(cmdFile, "ops")
	if pc == nil {
		t.Fatal("expected plugin command to be loaded")
	}
	if pc.Name != "check" {
		t.Errorf("name = %q, want %q", pc.Name, "check")
	}
	if pc.FullName() != "ops:check" {
		t.Errorf("fullName = %q, want %q", pc.FullName(), "ops:check")
	}
}

func TestLoadCustomCommandFile_NamespaceInFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	cmdFile := filepath.Join(tmpDir, "test.md")
	content := "---\nname: test\nnamespace: ci\ndescription: Run tests\n---\nExecute test suite.\n"
	if err := os.WriteFile(cmdFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pc := loadCustomCommandFile(cmdFile, "fallback")
	if pc.Namespace != "ci" {
		t.Errorf("namespace = %q, want %q (frontmatter should override default)", pc.Namespace, "ci")
	}
	if pc.FullName() != "ci:test" {
		t.Errorf("fullName = %q, want %q", pc.FullName(), "ci:test")
	}
}

func setupPluginRegistryWithCommands(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	pluginDir := filepath.Join(tmpDir, "myplugin")
	metaDir := filepath.Join(pluginDir, ".gen-plugin")
	cmdsDir := filepath.Join(pluginDir, "commands")
	if err := os.MkdirAll(metaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cmdsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := `{"name": "myplugin", "version": "1.0.0"}`
	if err := os.WriteFile(filepath.Join(metaDir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd1 := "---\nname: greet\ndescription: Say hello\n---\nGreet the user warmly.\n"
	if err := os.WriteFile(filepath.Join(cmdsDir, "greet.md"), []byte(cmd1), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd2 := "---\nname: build\ndescription: Build project\n---\nBuild all targets.\n"
	if err := os.WriteFile(filepath.Join(cmdsDir, "build.md"), []byte(cmd2), 0o644); err != nil {
		t.Fatal(err)
	}

	return tmpDir
}

func TestIsCustomCommand_MatchesCustomCommands(t *testing.T) {
	prevReg := plugin.DefaultRegistry
	prevCache := cachedCustomCommands
	t.Cleanup(func() {
		plugin.DefaultRegistry = prevReg
		cachedCustomCommands = prevCache
	})
	cachedCustomCommands = nil

	tmpDir := setupPluginRegistryWithCommands(t)

	plugin.DefaultRegistry = plugin.NewRegistry()
	if err := plugin.DefaultRegistry.LoadFromPath(nil, filepath.Join(tmpDir, "myplugin")); err != nil {
		t.Fatal(err)
	}

	pc, ok := IsCustomCommand("myplugin:greet")
	if !ok {
		cmds := GetCustomCommands()
		t.Logf("available plugin commands: %+v", cmds)
		t.Fatal("expected myplugin:greet to be found as plugin command")
	}
	if pc.Description != "Say hello" {
		t.Errorf("description = %q, want %q", pc.Description, "Say hello")
	}

	_, ok = IsCustomCommand("greet")
	if !ok {
		t.Fatal("expected short name 'greet' to match plugin command")
	}

	_, ok = IsCustomCommand("nonexistent")
	if ok {
		t.Fatal("nonexistent command should not match")
	}
}

func TestGetMatchingCommands_IncludesCustomCommands(t *testing.T) {
	prevReg := plugin.DefaultRegistry
	prevSkill := skill.DefaultRegistry
	prevCache := cachedCustomCommands
	t.Cleanup(func() {
		plugin.DefaultRegistry = prevReg
		skill.DefaultRegistry = prevSkill
		cachedCustomCommands = prevCache
	})
	skill.DefaultRegistry = nil
	cachedCustomCommands = nil

	tmpDir := setupPluginRegistryWithCommands(t)

	plugin.DefaultRegistry = plugin.NewRegistry()
	if err := plugin.DefaultRegistry.LoadFromPath(nil, filepath.Join(tmpDir, "myplugin")); err != nil {
		t.Fatal(err)
	}

	matches := GetMatchingCommands("gre")
	found := false
	for _, m := range matches {
		if m.Name == "myplugin:greet" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected myplugin:greet in matching commands for 'gre', got %+v", matches)
	}
}

func TestLoadCommandsFromDir(t *testing.T) {
	tmpDir := t.TempDir()
	cmdsDir := filepath.Join(tmpDir, "commands")
	if err := os.MkdirAll(cmdsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd1 := "---\nname: lint\ndescription: Run linter\n---\nLint all files.\n"
	if err := os.WriteFile(filepath.Join(cmdsDir, "lint.md"), []byte(cmd1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cmdsDir, "readme.txt"), []byte("not a command"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := loadCommandsFromDir(cmdsDir, "", ScopeUser)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Name != "lint" {
		t.Errorf("name = %q, want %q", cmds[0].Name, "lint")
	}
	if cmds[0].Scope != ScopeUser {
		t.Errorf("scope = %d, want %d (ScopeUser)", cmds[0].Scope, ScopeUser)
	}
	if cmds[0].Namespace != "" {
		t.Errorf("namespace = %q, want empty (user-level has no namespace)", cmds[0].Namespace)
	}
}

func TestLoadCommandsFromDir_NonexistentDir(t *testing.T) {
	cmds := loadCommandsFromDir("/nonexistent/path", "", ScopeProject)
	if len(cmds) != 0 {
		t.Fatalf("expected 0 commands from nonexistent dir, got %d", len(cmds))
	}
}

func TestProjectCommandOverridesUser(t *testing.T) {
	prevReg := plugin.DefaultRegistry
	prevSkill := skill.DefaultRegistry
	prevCwd := commandCwd
	prevCache := cachedCustomCommands
	t.Cleanup(func() {
		plugin.DefaultRegistry = prevReg
		skill.DefaultRegistry = prevSkill
		commandCwd = prevCwd
		cachedCustomCommands = prevCache
	})
	cachedCustomCommands = nil
	plugin.DefaultRegistry = nil
	skill.DefaultRegistry = nil

	root := t.TempDir()

	homeDir := filepath.Join(root, "home")
	userCmds := filepath.Join(homeDir, ".gen", "commands")
	if err := os.MkdirAll(userCmds, 0o755); err != nil {
		t.Fatal(err)
	}
	userCmd := "---\nname: deploy\ndescription: User deploy\n---\nUser-level deploy.\n"
	if err := os.WriteFile(filepath.Join(userCmds, "deploy.md"), []byte(userCmd), 0o644); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(root, "project")
	projectCmds := filepath.Join(projectDir, ".gen", "commands")
	if err := os.MkdirAll(projectCmds, 0o755); err != nil {
		t.Fatal(err)
	}
	projectCmd := "---\nname: deploy\ndescription: Project deploy\n---\nProject-level deploy.\n"
	if err := os.WriteFile(filepath.Join(projectCmds, "deploy.md"), []byte(projectCmd), 0o644); err != nil {
		t.Fatal(err)
	}

	commandCwd = projectDir

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", homeDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	cmds := loadAllCustomCommands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command (project overrides user), got %d: %+v", len(cmds), cmds)
	}
	if cmds[0].Description != "Project deploy" {
		t.Errorf("description = %q, want %q (project should override user)", cmds[0].Description, "Project deploy")
	}
	if cmds[0].Scope != ScopeProject {
		t.Errorf("scope = %d, want %d (ScopeProject)", cmds[0].Scope, ScopeProject)
	}
}

func TestUserCommandWithoutNamespace(t *testing.T) {
	root := t.TempDir()
	cmdsDir := filepath.Join(root, "commands")
	if err := os.MkdirAll(cmdsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := "---\nname: review\ndescription: Code review\n---\nReview code.\n"
	if err := os.WriteFile(filepath.Join(cmdsDir, "review.md"), []byte(cmd), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := loadCommandsFromDir(cmdsDir, "", ScopeUser)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].FullName() != "review" {
		t.Errorf("fullName = %q, want %q (user commands have no namespace)", cmds[0].FullName(), "review")
	}
}

func TestIsCustomCommand_MatchesUserAndProjectCommands(t *testing.T) {
	prevReg := plugin.DefaultRegistry
	prevSkill := skill.DefaultRegistry
	prevCwd := commandCwd
	prevCache := cachedCustomCommands
	t.Cleanup(func() {
		plugin.DefaultRegistry = prevReg
		skill.DefaultRegistry = prevSkill
		commandCwd = prevCwd
		cachedCustomCommands = prevCache
	})
	cachedCustomCommands = nil
	plugin.DefaultRegistry = nil
	skill.DefaultRegistry = nil

	root := t.TempDir()
	projectDir := filepath.Join(root, "project")
	projectCmds := filepath.Join(projectDir, ".gen", "commands")
	if err := os.MkdirAll(projectCmds, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := "---\nname: format\ndescription: Format code\n---\nFormat all source files.\n"
	if err := os.WriteFile(filepath.Join(projectCmds, "format.md"), []byte(cmd), 0o644); err != nil {
		t.Fatal(err)
	}

	commandCwd = projectDir

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", filepath.Join(root, "empty-home"))
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	pc, ok := IsCustomCommand("format")
	if !ok {
		t.Fatal("expected project-level 'format' command to be found")
	}
	if pc.Description != "Format code" {
		t.Errorf("description = %q, want %q", pc.Description, "Format code")
	}
	if pc.Scope != ScopeProject {
		t.Errorf("scope = %d, want %d (ScopeProject)", pc.Scope, ScopeProject)
	}
}

func TestPluginScopeMapping(t *testing.T) {
	tests := []struct {
		pluginScope plugin.Scope
		want        CommandScope
	}{
		{plugin.ScopeUser, ScopeUserPlugin},
		{plugin.ScopeManaged, ScopeUserPlugin},
		{plugin.ScopeProject, ScopeProjectPlugin},
		{plugin.ScopeLocal, ScopeProjectPlugin},
	}
	for _, tt := range tests {
		got := pluginScopeToCommandScope(tt.pluginScope)
		if got != tt.want {
			t.Errorf("pluginScopeToCommandScope(%q) = %d, want %d", tt.pluginScope, got, tt.want)
		}
	}
}

func TestCustomCommandScopeFromRegistry(t *testing.T) {
	prevReg := plugin.DefaultRegistry
	prevSkill := skill.DefaultRegistry
	prevCwd := commandCwd
	prevCache := cachedCustomCommands
	t.Cleanup(func() {
		plugin.DefaultRegistry = prevReg
		skill.DefaultRegistry = prevSkill
		commandCwd = prevCwd
		cachedCustomCommands = prevCache
	})
	cachedCustomCommands = nil
	skill.DefaultRegistry = nil
	commandCwd = ""

	tmpDir := setupPluginRegistryWithCommands(t)

	plugin.DefaultRegistry = plugin.NewRegistry()
	// LoadFromPath uses ScopeLocal → should map to ScopeProjectPlugin
	if err := plugin.DefaultRegistry.LoadFromPath(nil, filepath.Join(tmpDir, "myplugin")); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", filepath.Join(tmpDir, "empty-home"))
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	pc, ok := IsCustomCommand("myplugin:greet")
	if !ok {
		t.Fatal("expected plugin command to be found")
	}
	if pc.Scope != ScopeProjectPlugin {
		t.Errorf("scope = %d, want %d (ScopeProjectPlugin for ScopeLocal plugin)", pc.Scope, ScopeProjectPlugin)
	}
}
