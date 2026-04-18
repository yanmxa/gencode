package setting

import "sync"

// Service is the public contract for the setting module.
type Service interface {
	// Snapshot returns the current merged settings.
	Snapshot() *Settings

	// AllowBypass reports whether bypass mode is permitted.
	AllowBypass() bool

	// IsGitRepo checks if the given directory is a git repository.
	IsGitRepo(cwd string) bool

	// Reload re-reads all settings files for the given cwd and updates the singleton.
	Reload(cwd string) error

	// DisabledTools returns the merged disabled-tools map.
	DisabledTools() map[string]bool

	// SearchProvider returns the configured search provider string.
	SearchProvider() string

	// SetSearchProvider updates the search provider in the current settings.
	SetSearchProvider(provider string)

	// Hooks returns the merged hooks configuration.
	Hooks() map[string][]Hook

	// CheckPermission is a convenience wrapper returning just the permission behavior.
	CheckPermission(toolName string, args map[string]any, session *SessionPermissions) PermissionBehavior

	// HasPermissionToUseTool is the central permission gate.
	HasPermissionToUseTool(toolName string, args map[string]any, session *SessionPermissions) PermissionDecision

	// ResolveHookAllow checks if a hook's "allow" decision should be honored.
	ResolveHookAllow(toolName string, args map[string]any, session *SessionPermissions) bool

	// GetDisabledToolsAt returns disabled tools from a single settings file.
	// userLevel=true reads from ~/.gen/settings.json; false reads from .gen/settings.json.
	GetDisabledToolsAt(userLevel bool) map[string]bool

	// UpdateDisabledToolsAt updates disabled tools at user level (true) or project level (false).
	UpdateDisabledToolsAt(disabledTools map[string]bool, userLevel bool) error
}

// Compile-time check: *settingsService implements Service.
var _ Service = (*settingsService)(nil)

// Options holds configuration for Initialize.
type Options struct {
	CWD string
}

// ── singleton ──────────────────────────────────────────────

var (
	svcMu    sync.RWMutex
	svcInst  Service
)

// Default returns the singleton Service instance.
// Panics if Initialize has not been called.
func Default() Service {
	svcMu.RLock()
	s := svcInst
	svcMu.RUnlock()
	if s == nil {
		panic("setting: not initialized")
	}
	return s
}

// DefaultIfInit returns the singleton Service instance, or nil if not initialized.
func DefaultIfInit() Service {
	svcMu.RLock()
	s := svcInst
	svcMu.RUnlock()
	return s
}

// SetDefault replaces the singleton instance. Intended for tests.
func SetDefault(s Service) {
	svcMu.Lock()
	svcInst = s
	svcMu.Unlock()
}

// ResetService clears the singleton instance. Intended for tests.
func ResetService() {
	svcMu.Lock()
	svcInst = nil
	svcMu.Unlock()
}

// ── implementation ─────────────────────────────────────────

// settingsService wraps a *Settings to implement the Service interface.
type settingsService struct {
	mu       sync.RWMutex
	settings *Settings
}

func (s *settingsService) Snapshot() *Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.Clone()
}

func (s *settingsService) AllowBypass() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings != nil && s.settings.AllowBypass != nil && *s.settings.AllowBypass
}

func (s *settingsService) IsGitRepo(cwd string) bool {
	return IsGitRepo(cwd)
}

func (s *settingsService) Reload(cwd string) error {
	var (
		newSettings *Settings
		err         error
	)
	if cwd != "" {
		newSettings, err = LoadForCwd(cwd)
	} else {
		newSettings, err = Load()
	}
	if err != nil {
		return err
	}
	if newSettings == nil {
		newSettings = NewSettings()
	}
	mergeProviderPreferences(newSettings)
	cloned := newSettings.Clone()

	s.mu.Lock()
	s.settings = cloned
	s.mu.Unlock()

	// Keep DefaultSetup in sync for backward compatibility.
	DefaultSetup = cloned
	return nil
}

func (s *settingsService) DisabledTools() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings == nil || s.settings.DisabledTools == nil {
		return make(map[string]bool)
	}
	result := make(map[string]bool, len(s.settings.DisabledTools))
	for k, v := range s.settings.DisabledTools {
		result[k] = v
	}
	return result
}

func (s *settingsService) SearchProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings == nil {
		return ""
	}
	return s.settings.SearchProvider
}

func (s *settingsService) SetSearchProvider(provider string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.settings != nil {
		s.settings.SearchProvider = provider
	}
}

func (s *settingsService) Hooks() map[string][]Hook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings == nil {
		return nil
	}
	return s.settings.Hooks
}

func (s *settingsService) CheckPermission(toolName string, args map[string]any, session *SessionPermissions) PermissionBehavior {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings == nil {
		return Ask
	}
	return s.settings.CheckPermission(toolName, args, session)
}

func (s *settingsService) HasPermissionToUseTool(toolName string, args map[string]any, session *SessionPermissions) PermissionDecision {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings == nil {
		return decide(Ask, "default: no settings loaded")
	}
	return s.settings.HasPermissionToUseTool(toolName, args, session)
}

func (s *settingsService) ResolveHookAllow(toolName string, args map[string]any, session *SessionPermissions) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings == nil {
		return true
	}
	return s.settings.ResolveHookAllow(toolName, args, session)
}

func (s *settingsService) GetDisabledToolsAt(userLevel bool) map[string]bool {
	return GetDisabledToolsAt(userLevel)
}

func (s *settingsService) UpdateDisabledToolsAt(disabledTools map[string]bool, userLevel bool) error {
	return UpdateDisabledToolsAt(disabledTools, userLevel)
}
