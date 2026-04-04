package config

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"

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

	// Two-phase loading: first load Claude-compat settings, then GenCode-native.
	// For hooks, GenCode-native settings override Claude-compat settings per event
	// to prevent incompatible hooks (e.g., Claude Code's interactive protocol)
	// from blocking GenCode's own hooks.

	type source struct {
		path        string
		claudeCompat bool
	}

	var sources []source
	if l.claudeCompat {
		sources = append(sources, source{filepath.Join(homeDir, ".claude", "settings.json"), true})
	}
	sources = append(sources, source{filepath.Join(l.userDir, "settings.json"), false})
	if l.claudeCompat {
		sources = append(sources, source{filepath.Join(".claude", "settings.json"), true})
	}
	sources = append(sources, source{filepath.Join(l.projectDir, "settings.json"), false})
	if l.claudeCompat {
		sources = append(sources, source{filepath.Join(".claude", "settings.local.json"), true})
	}
	sources = append(sources, source{filepath.Join(l.projectDir, "settings.local.json"), false})

	// Collect hooks separately: Claude-compat hooks and GenCode-native hooks.
	// For native hooks, higher-priority sources REPLACE lower-priority sources
	// per event (project overrides user, local overrides project).
	claudeHooks := make(map[string][]Hook)
	nativeHooks := make(map[string][]Hook) // last write wins per event

	settings := NewSettings()
	for _, src := range sources {
		data, err := os.ReadFile(src.path)
		if err != nil {
			continue
		}
		var s Settings
		if err := json.Unmarshal(data, &s); err != nil {
			log.Logger().Warn("failed to parse config file", zap.String("path", src.path), zap.Error(err))
			continue
		}

		// Extract hooks before merging — we'll merge hooks manually
		srcHooks := s.Hooks
		s.Hooks = nil
		settings = MergeSettings(settings, &s)

		// Accumulate hooks by source type.
		// Native hooks: higher-priority sources replace lower-priority per event.
		// This means project .gen/settings.json can set "PermissionRequest": []
		// to disable user-level PermissionRequest hooks.
		for event, hooks := range srcHooks {
			if src.claudeCompat {
				claudeHooks[event] = append(claudeHooks[event], hooks...)
			} else {
				nativeHooks[event] = hooks
			}
		}
	}

	// Merge hooks: for each event, use native hooks if available, otherwise use Claude-compat hooks.
	// PermissionRequest hooks are NEVER inherited from Claude-compat sources because
	// Claude Code's interactive permission protocol (e.g., vibe-island-bridge) is
	// incompatible with GenCode's TUI-based approval flow and can cause hangs.
	merged := make(map[string][]Hook)
	for event, hooks := range claudeHooks {
		if event == "PermissionRequest" {
			continue // skip — incompatible protocol
		}
		if _, hasNative := nativeHooks[event]; !hasNative {
			merged[event] = hooks
		}
	}
	for event, hooks := range nativeHooks {
		if len(hooks) > 0 {
			merged[event] = hooks
		}
		// Empty hooks array means explicitly disabled — don't add to merged
	}
	settings.Hooks = merged

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

// AddAllowRule appends a permission allow rule to project-level settings.
// The rule is built from the tool name and arguments (e.g., "Bash(git:*)").
func AddAllowRule(toolName string, args map[string]any) error {
	return AddAllowRuleDirectly(BuildRule(toolName, args))
}

// AddAllowRuleDirectly appends a pre-built permission allow rule string
// to project-level settings. Unlike AddAllowRule, it does not build the rule
// from tool name + args — the caller provides the final rule string.
func AddAllowRuleDirectly(rule string) error {
	if rule == "" {
		return nil
	}

	loader := NewLoader()
	path := filepath.Join(loader.projectDir, "settings.json")

	// Load existing to check for duplicates
	existing, _ := loader.LoadFile(path)
	if existing != nil && slices.Contains(existing.Permissions.Allow, rule) {
		return nil // already exists
	}

	settings := &Settings{
		Permissions: PermissionSettings{
			Allow: []string{rule},
		},
	}
	if err := loader.SaveToProject(settings); err != nil {
		return err
	}
	loadedSettings = nil
	return nil
}

// SaveTheme persists the chosen theme to ~/.gen/settings.json.
func SaveTheme(t string) error {
	if err := NewLoader().SaveToUser(&Settings{Theme: t}); err != nil {
		return err
	}
	loadedSettings = nil
	return nil
}
