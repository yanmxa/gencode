---
package: github.com/genai-io/san/internal/permission
layer: feature
---

# permission

Owns tool permission policy independently from configuration loading. The
package evaluates persisted allow/deny/ask rules together with session grants,
operation mode, working-directory constraints, and bypass-immune safety checks.

## Entrypoints

- `Policy.HasPermissionToUseTool` returns behavior plus its audit reason.
- `Policy.ResolveHookAllow` verifies that a Hook allow does not override a
  deny, explicit ask, or bypass-immune rule.
- `BuildRule`, `MatchesToolPattern`, and `MatchAllowList` implement rule syntax.
- `BypassImmuneReason` applies sensitive-path and Bash security analysis.
- `GenerateSuggestions` builds bounded reusable rule suggestions.

`PermissionSettings`, `SessionPermissions`, and `OperationMode` are owned here.
`internal/setting` aliases these values so existing JSON and app APIs stay
compatible, but it does not implement policy.

## Internals and tests

- `policy.go` — ordered decision pipeline and rule matching.
- `bash_ast.go` — shell AST extraction and structural security checks.
- `security.go` — destructive-command, sensitive-path, and denial safeguards.
- `path.go` — symlink-aware working-directory containment.
- `suggestion.go` — safe reusable rule suggestions.
- Tests live beside each responsibility under `internal/permission/*_test.go`.

See [`concepts/permission-model.md`](../../concepts/permission-model.md) and
[`reference/claude-permission-compat.md`](../../reference/claude-permission-compat.md).
