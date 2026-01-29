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
	cwd          string // Current working directory for project-level skills
	claudeCompat bool   // Whether to load from .claude directories
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
		cwd:          cwd,
		claudeCompat: true,
	}
}

// getSearchPaths returns skill directories in priority order (lowest to highest).
func (l *Loader) getSearchPaths() []searchPath {
	homeDir, _ := os.UserHomeDir()
	var paths []searchPath

	// 1. ~/.claude/skills/ (Claude user compat - lowest priority)
	if l.claudeCompat {
		paths = append(paths, searchPath{
			path:  filepath.Join(homeDir, ".claude", "skills"),
			scope: ScopeClaudeUser,
		})
	}

	// 2. User plugins from ~/.claude/plugins/installed_plugins.json
	if l.claudeCompat {
		paths = append(paths, l.getPluginPaths(
			filepath.Join(homeDir, ".claude", "plugins"),
			ScopeUserPlugin,
		)...)
	}

	// 3. User plugins from ~/.gen/plugins/installed_plugins.json
	paths = append(paths, l.getPluginPaths(
		filepath.Join(homeDir, ".gen", "plugins"),
		ScopeUserPlugin,
	)...)

	// 4. ~/.gen/skills/ (User level)
	paths = append(paths, searchPath{
		path:  filepath.Join(homeDir, ".gen", "skills"),
		scope: ScopeUser,
	})

	// 5. .claude/skills/ (Claude project compat)
	if l.claudeCompat {
		paths = append(paths, searchPath{
			path:  filepath.Join(l.cwd, ".claude", "skills"),
			scope: ScopeClaudeProject,
		})
	}

	// 6. Project plugins from .claude/plugins/installed_plugins.json
	if l.claudeCompat {
		paths = append(paths, l.getPluginPaths(
			filepath.Join(l.cwd, ".claude", "plugins"),
			ScopeProjectPlugin,
		)...)
	}

	// 7. Project plugins from .gen/plugins/installed_plugins.json
	paths = append(paths, l.getPluginPaths(
		filepath.Join(l.cwd, ".gen", "plugins"),
		ScopeProjectPlugin,
	)...)

	// 8. .gen/skills/ (Project level - highest priority)
	paths = append(paths, searchPath{
		path:  filepath.Join(l.cwd, ".gen", "skills"),
		scope: ScopeProject,
	})

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
// defaultNamespace is applied if the skill doesn't have an explicit namespace.
func (l *Loader) loadSkillFile(path string, scope SkillScope, defaultNamespace string) (*Skill, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var frontmatter strings.Builder
	var content strings.Builder
	inFrontmatter := false
	frontmatterDone := false

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// First line must be ---
		if lineNum == 1 {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			// No frontmatter, treat entire file as content
			content.WriteString(line)
			content.WriteString("\n")
			continue
		}

		if inFrontmatter && !frontmatterDone {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
			frontmatter.WriteString(line)
			frontmatter.WriteString("\n")
		} else {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	skill := &Skill{
		FilePath: path,
		Scope:    scope,
		State:    StateEnable, // Default state
	}

	// Parse frontmatter
	if frontmatter.Len() > 0 {
		if err := yaml.Unmarshal([]byte(frontmatter.String()), skill); err != nil {
			return nil, err
		}
	}

	// If no name in frontmatter, derive from directory name
	if skill.Name == "" {
		skill.Name = filepath.Base(filepath.Dir(path))
	}

	// Apply default namespace if not set in frontmatter (e.g., from plugin dir)
	if skill.Namespace == "" && defaultNamespace != "" {
		skill.Namespace = defaultNamespace
	}

	// Don't load instructions yet (lazy loading)
	// Just store that we have content available
	skill.Instructions = strings.TrimSpace(content.String())
	skill.loaded = true

	return skill, nil
}

// loadInstructions loads the full instructions from a skill file.
func loadInstructions(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var content strings.Builder
	inFrontmatter := false
	frontmatterDone := false

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		// First line must be ---
		if lineNum == 1 {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = true
				continue
			}
			// No frontmatter, treat entire file as content
			content.WriteString(line)
			content.WriteString("\n")
			continue
		}

		if inFrontmatter && !frontmatterDone {
			if strings.TrimSpace(line) == "---" {
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
			// Skip frontmatter content
		} else {
			content.WriteString(line)
			content.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.TrimSpace(content.String()), nil
}
