---
package: github.com/genai-io/san/internal/setting
layer: feature
---

# setting

Settings data loader, merger, persistence, and live synchronized handle.
Reads `~/.san/settings.json` and `<project>/.san/settings.json`, merges
project-over-user with documented precedence, and supplies permission rule
configuration to `internal/permission`.

## Purpose

Load and merge two-tier settings (user + project), including hooks, disabled
tools, search provider, permission rules, env vars, work directory, and Claude
Code-compatible `.claude/` shims. Permission methods on `Data` and `Settings`
are compatibility adapters to `permission.Policy`; policy itself is not owned
here.

## Contract

`Settings` wraps `Data` under a mutex; methods are synchronized views. The
package exposes `*Settings` directly—there is no producer-side Service
interface.

```go
package setting

// Settings is the opaque handle. Type exported; fields unexported.
type Settings struct { /* internal fields */ }

func (s *Settings) Snapshot() *Data
func (s *Settings) AllowBypass() bool
func (s *Settings) IsGitRepo(cwd string) bool
func (s *Settings) Reload(cwd string) error
func (s *Settings) DisabledTools() map[string]bool
func (s *Settings) SearchProvider() string
func (s *Settings) SetSearchProvider(provider string)
func (s *Settings) Hooks() map[string][]Hook
func (s *Settings) CheckPermission(toolName string, args map[string]any, session *SessionPermissions) PermissionBehavior
func (s *Settings) HasPermissionToUseTool(toolName string, args map[string]any, session *SessionPermissions) PermissionDecision
func (s *Settings) ResolveHookAllow(toolName string, args map[string]any, session *SessionPermissions) bool
func (s *Settings) GetDisabledToolsAt(userLevel bool) map[string]bool
func (s *Settings) UpdateDisabledToolsAt(disabledTools map[string]bool, userLevel bool) error

// Package-level access
func Initialize(opts Options)
func Default() *Settings
func DefaultIfInit() *Settings           // nil pre-Initialize
func SetDefaultSettings(s *Settings)      // test-only
func ResetDefaultSettings()              // test-only
```


## Internals

- `Data` (`settings.go`) — value type holding all merged config.
- `loader.go` + `merger.go` — read the two tiers and combine them with
  documented precedence (project overrides user, except in a few flagged
  fields).
- `permission.go` — thin compatibility adapters to `internal/permission`.
- `workdir.go` — cwd resolution and git-root detection.
- Permission rules, Bash AST parsing, working-directory containment, and safety
  checks live in `internal/permission`.

## Lifecycle

- Construction: `Initialize(Options{CWD})` runs once at startup and after
  cwd changes.
- Reload: `Reload(cwd)` rebuilds settings under lock; the singleton swaps
  atomically.
- Per-call: permission adapters take a mutex-protected snapshot and delegate to
  `permission.Policy`.

## Tests

```
internal/setting/config_extra_test.go    — config merge semantics.
internal/permission/path_test.go         — cwd containment and symlink safety.
internal/permission/policy_test.go       — permission decision scenarios.
internal/permission/bash_ast_test.go     — Bash rule/security parsing.
```

## See Also

- Code: `internal/setting/`
- Reference: [`reference/configuration.md`](../../reference/configuration.md)
- Concepts: [`concepts/permission-model.md`](../../concepts/permission-model.md)
- Permission policy: [`packages/permission.md`](permission.md)
- Permission consumers: [`packages/tool.md`](tool.md), [`packages/hook.md`](hook.md)
- Layer: `feature`
