package config

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/yanmxa/gencode/internal/log"
)

// Loader handles loading and merging settings from multiple sources.
type Loader struct {
	userDir      string
	projectDir   string
	claudeCompat bool
}

// NewLoader creates a loader with default paths (~/.gen, .gen) and Claude compatibility enabled.
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

// Load loads and merges settings from all sources in priority order (lowest to highest):
//  1. ~/.claude/settings.json
//  2. ~/.gen/settings.json
//  3. .claude/settings.json
//  4. .gen/settings.json
//  5. .claude/settings.local.json
//  6. .gen/settings.local.json
func (l *Loader) Load() (*Settings, error) {
	homeDir, _ := os.UserHomeDir()

	sources := make([]string, 0, 6)
	if l.claudeCompat {
		sources = append(sources,
			filepath.Join(homeDir, ".claude", "settings.json"),
		)
	}
	sources = append(sources, filepath.Join(l.userDir, "settings.json"))
	if l.claudeCompat {
		sources = append(sources,
			filepath.Join(".claude", "settings.json"),
		)
	}
	sources = append(sources, filepath.Join(l.projectDir, "settings.json"))
	if l.claudeCompat {
		sources = append(sources,
			filepath.Join(".claude", "settings.local.json"),
		)
	}
	sources = append(sources, filepath.Join(l.projectDir, "settings.local.json"))

	settings := NewSettings()
	for _, src := range sources {
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		var s Settings
		if err := json.Unmarshal(data, &s); err != nil {
			log.Logger().Warn("failed to parse config file", zap.String("path", src), zap.Error(err))
			continue
		}
		settings = MergeSettings(settings, &s)
	}
	return settings, nil
}

// LoadFile loads settings from a specific file.
func (l *Loader) LoadFile(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// SaveToProject saves settings to the project-level settings file, merging with existing.
func (l *Loader) SaveToProject(settings *Settings) error {
	return l.saveToFile(filepath.Join(l.projectDir, "settings.json"), settings)
}

// SaveToUser saves settings to the user-level settings file, merging with existing.
func (l *Loader) SaveToUser(settings *Settings) error {
	return l.saveToFile(filepath.Join(l.userDir, "settings.json"), settings)
}

func (l *Loader) saveToFile(path string, settings *Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	toSave := settings
	if data, err := os.ReadFile(path); err == nil {
		existing := NewSettings()
		if err := json.Unmarshal(data, existing); err == nil {
			toSave = MergeSettings(existing, settings)
		}
	}

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

var loadedSettings *Settings

// Load loads settings using the default loader (cached after first call).
func Load() (*Settings, error) {
	if loadedSettings != nil {
		return loadedSettings, nil
	}
	s, err := NewLoader().Load()
	if err != nil {
		return nil, err
	}
	loadedSettings = s
	return s, nil
}

// Reload clears the settings cache and reloads from disk.
func Reload() (*Settings, error) {
	loadedSettings = nil
	return Load()
}

// Default returns default settings without loading from disk.
func Default() *Settings {
	return NewSettings()
}

// UpdateDisabledTools updates disabled tools in project-level settings.
func UpdateDisabledTools(disabledTools map[string]bool) error {
	return UpdateDisabledToolsAt(disabledTools, false)
}

// UpdateDisabledToolsAt updates disabled tools at user level (true) or project level (false).
func UpdateDisabledToolsAt(disabledTools map[string]bool, userLevel bool) error {
	loader := NewLoader()
	settings := &Settings{DisabledTools: disabledTools}

	var err error
	if userLevel {
		err = loader.SaveToUser(settings)
	} else {
		err = loader.SaveToProject(settings)
	}
	if err != nil {
		return err
	}

	loadedSettings = nil
	return nil
}

// GetDisabledTools returns the merged disabled tools map from loaded settings.
func GetDisabledTools() map[string]bool {
	s, err := Load()
	if err != nil || s.DisabledTools == nil {
		return make(map[string]bool)
	}
	return s.DisabledTools
}

// GetDisabledToolsAt returns disabled tools from a single settings file (not merged).
// userLevel=true reads from ~/.gen/settings.json; false reads from .gen/settings.json.
func GetDisabledToolsAt(userLevel bool) map[string]bool {
	loader := NewLoader()
	path := filepath.Join(loader.projectDir, "settings.json")
	if userLevel {
		path = filepath.Join(loader.userDir, "settings.json")
	}
	s, err := loader.LoadFile(path)
	if err != nil || s.DisabledTools == nil {
		return make(map[string]bool)
	}
	result := make(map[string]bool, len(s.DisabledTools))
	maps.Copy(result, s.DisabledTools)
	return result
}

// SaveTheme persists the chosen theme to ~/.gen/settings.json.
func SaveTheme(t string) error {
	if err := NewLoader().SaveToUser(&Settings{Theme: t}); err != nil {
		return err
	}
	loadedSettings = nil
	return nil
}
