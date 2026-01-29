package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Loader handles loading and merging settings from multiple sources.
type Loader struct {
	// userDir is the user-level config directory (e.g., ~/.gen)
	userDir string

	// projectDir is the project-level config directory (e.g., .gen)
	projectDir string

	// claudeCompat enables loading from .claude directories
	claudeCompat bool
}

// NewLoader creates a new settings loader.
// It defaults to:
//   - userDir: ~/.gen
//   - projectDir: .gen
//   - claudeCompat: true (also loads from .claude directories)
func NewLoader() *Loader {
	homeDir, _ := os.UserHomeDir()
	return &Loader{
		userDir:      filepath.Join(homeDir, ".gen"),
		projectDir:   ".gen",
		claudeCompat: true,
	}
}

// NewLoaderWithOptions creates a loader with custom options.
func NewLoaderWithOptions(userDir, projectDir string, claudeCompat bool) *Loader {
	return &Loader{
		userDir:      userDir,
		projectDir:   projectDir,
		claudeCompat: claudeCompat,
	}
}

// Load loads and merges settings from all sources.
// Priority (lowest to highest):
//   1. ~/.claude/settings.json (Claude user level)
//   2. ~/.gen/settings.json (Gen user level)
//   3. .claude/settings.json (Claude project level)
//   4. .gen/settings.json (Gen project level)
//   5. .claude/settings.local.json (Claude local level)
//   6. .gen/settings.local.json (Gen local level)
//
// Later sources override earlier ones.
func (l *Loader) Load() (*Settings, error) {
	settings := NewSettings()
	homeDir, _ := os.UserHomeDir()

	// Build list of config sources in priority order (lowest to highest)
	var sources []string

	if l.claudeCompat {
		// Claude user level (lowest priority)
		sources = append(sources, filepath.Join(homeDir, ".claude", "settings.json"))
	}

	// Gen user level
	sources = append(sources, filepath.Join(l.userDir, "settings.json"))

	if l.claudeCompat {
		// Claude project level
		sources = append(sources, filepath.Join(".claude", "settings.json"))
	}

	// Gen project level
	sources = append(sources, filepath.Join(l.projectDir, "settings.json"))

	if l.claudeCompat {
		// Claude local level
		sources = append(sources, filepath.Join(".claude", "settings.local.json"))
	}

	// Gen local level (highest user-controllable priority)
	sources = append(sources, filepath.Join(l.projectDir, "settings.local.json"))

	// Load and merge each source
	for _, src := range sources {
		if data, err := os.ReadFile(src); err == nil {
			var s Settings
			if err := json.Unmarshal(data, &s); err == nil {
				settings = MergeSettings(settings, &s)
			}
		}
	}

	return settings, nil
}

// LoadFile loads settings from a specific file.
func (l *Loader) LoadFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

// GetUserDir returns the user config directory path.
func (l *Loader) GetUserDir() string {
	return l.userDir
}

// GetProjectDir returns the project config directory path.
func (l *Loader) GetProjectDir() string {
	return l.projectDir
}

// EnsureUserDir creates the user config directory if it doesn't exist.
func (l *Loader) EnsureUserDir() error {
	return os.MkdirAll(l.userDir, 0755)
}

// EnsureProjectDir creates the project config directory if it doesn't exist.
func (l *Loader) EnsureProjectDir() error {
	return os.MkdirAll(l.projectDir, 0755)
}

// SaveToProject saves settings to the project-level settings file.
// It merges with existing settings if the file exists.
func (l *Loader) SaveToProject(settings *Settings) error {
	return l.saveToFile(filepath.Join(l.projectDir, "settings.json"), settings)
}

// SaveToUser saves settings to the user-level settings file.
// It merges with existing settings if the file exists.
func (l *Loader) SaveToUser(settings *Settings) error {
	return l.saveToFile(filepath.Join(l.userDir, "settings.json"), settings)
}

// saveToFile saves settings to a specific file, merging with existing content.
func (l *Loader) saveToFile(path string, settings *Settings) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Load existing settings if file exists
	var existing *Settings
	if data, err := os.ReadFile(path); err == nil {
		existing = NewSettings()
		if err := json.Unmarshal(data, existing); err != nil {
			existing = nil
		}
	}

	// Merge with existing settings
	var toSave *Settings
	if existing != nil {
		toSave = MergeSettings(existing, settings)
	} else {
		toSave = settings
	}

	// Write to file with pretty formatting
	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// defaultSettings is a cached instance of the default settings
var defaultSettings *Settings

// loadedSettings is a cached instance of the loaded settings
var loadedSettings *Settings

// Load is a convenience function that loads settings using the default loader.
func Load() (*Settings, error) {
	if loadedSettings != nil {
		return loadedSettings, nil
	}
	loader := NewLoader()
	settings, err := loader.Load()
	if err != nil {
		return nil, err
	}
	loadedSettings = settings
	return loadedSettings, nil
}

// Reload forces reloading of settings, clearing the cache.
func Reload() (*Settings, error) {
	loadedSettings = nil
	return Load()
}

// Default returns the default settings without loading from files.
func Default() *Settings {
	if defaultSettings == nil {
		defaultSettings = NewSettings()
	}
	return defaultSettings
}

// UpdateDisabledTools updates the disabled tools in project-level settings.
// It only saves the DisabledTools field, preserving other settings.
func UpdateDisabledTools(disabledTools map[string]bool) error {
	return UpdateDisabledToolsAt(disabledTools, false)
}

// UpdateDisabledToolsAt updates the disabled tools at the specified level.
// If userLevel is true, saves to ~/.gen/settings.json, otherwise to .gen/settings.json.
func UpdateDisabledToolsAt(disabledTools map[string]bool, userLevel bool) error {
	loader := NewLoader()

	// Create settings with only DisabledTools
	settings := &Settings{
		DisabledTools: disabledTools,
	}

	// Save to appropriate level
	var err error
	if userLevel {
		err = loader.SaveToUser(settings)
	} else {
		err = loader.SaveToProject(settings)
	}
	if err != nil {
		return err
	}

	// Clear cache so next Load() picks up changes
	loadedSettings = nil
	return nil
}

// GetDisabledTools returns the disabled tools from loaded settings.
func GetDisabledTools() map[string]bool {
	settings, err := Load()
	if err != nil {
		return make(map[string]bool)
	}
	if settings.DisabledTools == nil {
		return make(map[string]bool)
	}
	return settings.DisabledTools
}

// GetDisabledToolsAt returns the disabled tools from a specific level (not merged).
// If userLevel is true, loads from ~/.gen/settings.json, otherwise from .gen/settings.json.
func GetDisabledToolsAt(userLevel bool) map[string]bool {
	loader := NewLoader()

	var path string
	if userLevel {
		path = filepath.Join(loader.userDir, "settings.json")
	} else {
		path = filepath.Join(loader.projectDir, "settings.json")
	}

	settings, err := loader.LoadFile(path)
	if err != nil {
		return make(map[string]bool)
	}
	if settings.DisabledTools == nil {
		return make(map[string]bool)
	}

	// Return a copy to avoid mutation
	result := make(map[string]bool)
	for k, v := range settings.DisabledTools {
		result[k] = v
	}
	return result
}
