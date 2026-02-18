package skill

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// InstalledPluginsData represents the installed_plugins.json structure.
type InstalledPluginsData struct {
	Version int                          `json:"version"`
	Plugins map[string][]PluginInstall   `json:"plugins"`
}

// PluginInstall represents a single plugin installation.
type PluginInstall struct {
	Scope       string `json:"scope"`       // "user" or "project"
	InstallPath string `json:"installPath"` // Full path to plugin
	Version     string `json:"version"`
}

// Loader handles loading skills from multiple directories.
type Loader struct {
	cwd             string       // Current working directory for project-level skills
	additionalPaths []searchPath // Additional paths from plugins
}

// searchPath represents a skill search location with optional namespace.
type searchPath struct {
	path      string
	scope     SkillScope
	namespace string // Default namespace for skills in this path (from plugin dir)
}

// NewLoader creates a new skill loader.
func NewLoader(cwd string) *Loader {
	return &Loader{
		cwd: cwd,
	}
}

// AddPluginPath adds a plugin skill path to the loader.
// Plugin paths are searched after user-level but before project-level skills.
func (l *Loader) AddPluginPath(path, namespace string, isProjectScope bool) {
	scope := ScopeUserPlugin
	if isProjectScope {
		scope = ScopeProjectPlugin
	}
	l.additionalPaths = append(l.additionalPaths, searchPath{
		path:      path,
		scope:     scope,
		namespace: namespace,
	})
}

// AddPluginPaths adds multiple plugin skill paths to the loader.
func (l *Loader) AddPluginPaths(paths []struct {
	Path      string
	Namespace string
	IsProject bool
}) {
	for _, p := range paths {
		l.AddPluginPath(p.Path, p.Namespace, p.IsProject)
	}
}

// getSearchPaths returns skill directories in priority order (lowest to highest).
// Note: .claude/plugins/ loading is removed - plugins are handled by the plugin system.
func (l *Loader) getSearchPaths() []searchPath {
	homeDir, _ := os.UserHomeDir()
	var paths []searchPath

	// 1. ~/.claude/skills/ (Claude user compat - lowest priority)
	paths = append(paths, searchPath{
		path:  filepath.Join(homeDir, ".claude", "skills"),
		scope: ScopeClaudeUser,
	})

	// 2. User plugins from ~/.gen/plugins/installed_plugins.json
	paths = append(paths, l.getPluginPaths(
		filepath.Join(homeDir, ".gen", "plugins"),
		ScopeUserPlugin,
	)...)

	// 3. ~/.gen/skills/ (User level)
	paths = append(paths, searchPath{
		path:  filepath.Join(homeDir, ".gen", "skills"),
		scope: ScopeUser,
	})

	// 4. .claude/skills/ (Claude project compat)
	paths = append(paths, searchPath{
		path:  filepath.Join(l.cwd, ".claude", "skills"),
		scope: ScopeClaudeProject,
	})

	// 5. Project plugins from .gen/plugins/installed_plugins.json
	paths = append(paths, l.getPluginPaths(
		filepath.Join(l.cwd, ".gen", "plugins"),
		ScopeProjectPlugin,
	)...)

	// 6. .gen/skills/ (Project level - highest priority)
	paths = append(paths, searchPath{
		path:  filepath.Join(l.cwd, ".gen", "skills"),
		scope: ScopeProject,
	})

	// Insert additional plugin paths at appropriate positions
	if len(l.additionalPaths) > 0 {
		// Separate user and project plugin paths
		var userPluginPaths, projectPluginPaths []searchPath
		for _, ap := range l.additionalPaths {
			if ap.scope == ScopeProjectPlugin {
				projectPluginPaths = append(projectPluginPaths, ap)
			} else {
				userPluginPaths = append(userPluginPaths, ap)
			}
		}

		// Insert at correct positions
		// User plugin paths go after ScopeUser (position after ~/.gen/skills)
		// Project plugin paths go after ScopeClaudeProject (before ScopeProject)
		var result []searchPath
		for _, p := range paths {
			result = append(result, p)
			if p.scope == ScopeUser {
				result = append(result, userPluginPaths...)
			} else if p.scope == ScopeClaudeProject {
				result = append(result, projectPluginPaths...)
			}
		}
		return result
	}

	return paths
}

// getPluginPaths discovers plugin skill directories from installed_plugins.json.
// Plugin skills inherit namespace from their plugin name (before @).
func (l *Loader) getPluginPaths(pluginsDir string, scope SkillScope) []searchPath {
	var paths []searchPath

	// Read installed_plugins.json
	configPath := filepath.Join(pluginsDir, "installed_plugins.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return paths // File doesn't exist
	}

	var config InstalledPluginsData
	if err := json.Unmarshal(data, &config); err != nil {
		return paths // Invalid JSON
	}

	// Process each installed plugin
	for pluginKey, installs := range config.Plugins {
		if len(installs) == 0 {
			continue
		}

		// Use the first (most recent) installation
		install := installs[0]

		// Extract plugin name from key (format: "plugin-name@marketplace")
		pluginName := pluginKey
		if idx := strings.Index(pluginKey, "@"); idx > 0 {
			pluginName = pluginKey[:idx]
		}

		// Check if skills directory exists in the install path
		skillsDir := filepath.Join(install.InstallPath, "skills")
		if _, err := os.Stat(skillsDir); err == nil {
			paths = append(paths, searchPath{
				path:      skillsDir,
				scope:     scope,
				namespace: pluginName, // Plugin name becomes default namespace
			})
		}
	}

	return paths
}

// LoadAll loads all skills from all directories.
// Higher priority scopes override lower priority ones with the same name.
func (l *Loader) LoadAll() (map[string]*Skill, error) {
	skills := make(map[string]*Skill)

	for _, sp := range l.getSearchPaths() {
		if _, err := os.Stat(sp.path); os.IsNotExist(err) {
			continue
		}

		// Walk the skills directory
		err := filepath.Walk(sp.path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip errors, continue walking
			}

			// Look for SKILL.md files (case-insensitive)
			if info.IsDir() {
				return nil
			}
			baseName := strings.ToLower(info.Name())
			if baseName != "skill.md" {
				return nil
			}

			skill, err := l.loadSkillFile(path, sp.scope, sp.namespace)
			if err != nil {
				return nil // Skip invalid skills
			}

			// Use FullName (namespace:name) as the key
			fullName := skill.FullName()

			// Higher priority scopes override lower ones
			if existing, ok := skills[fullName]; ok {
				if skill.Scope > existing.Scope {
					skills[fullName] = skill
				}
			} else {
				skills[fullName] = skill
			}

			return nil
		})
		if err != nil {
			continue // Skip directory errors
		}
	}

	return skills, nil
}

// loadSkillFile loads a skill from a file path.
// Only parses frontmatter for metadata; instructions are lazy-loaded.
func (l *Loader) loadSkillFile(path string, scope SkillScope, defaultNamespace string) (*Skill, error) {
	fm, _, err := parseFrontmatterFile(path)
	if err != nil {
		return nil, err
	}

	skillDir := filepath.Dir(path)

	skill := &Skill{
		FilePath: path,
		SkillDir: skillDir,
		Scope:    scope,
		State:    StateEnable,
	}

	if fm != "" {
		if err := yaml.Unmarshal([]byte(fm), skill); err != nil {
			return nil, err
		}
	}

	if skill.Name == "" {
		skill.Name = filepath.Base(skillDir)
	}

	if skill.Namespace == "" && defaultNamespace != "" {
		skill.Namespace = defaultNamespace
	}

	skill.Scripts = scanResourceDir(filepath.Join(skillDir, "scripts"))
	skill.References = scanResourceDir(filepath.Join(skillDir, "references"))
	skill.Assets = scanResourceDir(filepath.Join(skillDir, "assets"))
	skill.loaded = false

	return skill, nil
}

// parseFrontmatterFile reads a markdown file and returns (frontmatter, body).
func parseFrontmatterFile(path string) (frontmatter, body string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer file.Close()

	var fmBuilder strings.Builder
	var bodyBuilder strings.Builder
	inFrontmatter := false
	frontmatterDone := false

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if lineNum == 1 {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			bodyBuilder.WriteString(line)
			bodyBuilder.WriteString("\n")
			continue
		}

		if inFrontmatter && !frontmatterDone {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
			fmBuilder.WriteString(line)
			fmBuilder.WriteString("\n")
		} else {
			bodyBuilder.WriteString(line)
			bodyBuilder.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", "", err
	}

	return fmBuilder.String(), strings.TrimSpace(bodyBuilder.String()), nil
}

// scanResourceDir scans a directory and returns file names.
func scanResourceDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files
}

// loadInstructions loads the full instructions from a skill file.
func loadInstructions(path string) (string, error) {
	_, body, err := parseFrontmatterFile(path)
	if err != nil {
		return "", err
	}
	return body, nil
}
