# Hook Engine Architecture

## What It Does

Hook Engine lets external code react to application events — intercept
operations before they happen (deny a tool call, rewrite its input,
control permissions) or observe them after the fact (log, notify, audit).

External code can be a shell command, HTTP endpoint, LLM prompt, or
in-memory Go function.

## Design Principles

**Engine understands permissions.** The engine directly parses and returns
permission-related fields (`PermissionDecision`, `UpdatedPermissions`).
This is the only domain coupling — everything else (block, modify, inject)
is generic.

## Structure

```
internal/hook/
┌──────────────────────────────────────────┐
│ Engine                                   │
│                                          │
│  Execute(event, input) → HookOutcome     │
│  ExecuteAsync(event, input)              │
│                                          │
│ HookOutcome:                             │
│   ShouldBlock bool                       │
│   BlockReason string                     │
│   AdditionalContext string               │
│   UpdatedInput map[string]any            │
│   PermissionDecision string              │
│   UpdatedPermissions []PermissionUpdate  │
│   WatchPaths []string                    │
│   InitialUserMessage string              │
│   Retry bool                             │
├──────────────────────────────────────────┤
│ Store     config + session hooks         │
│ Matcher   2-layer filter                 │
│ Status    active hook tracking           │
├──────────────────────────────────────────┤
│ Executors                                │
│  Command  sh -c, stdin/stdout            │
│  HTTP     POST JSON to URL               │
│  Prompt   single LLM completion          │
│  Function in-memory callback             │
└──────────────────────────────────────────┘
```

- `hook/` is a self-contained app-layer package. Handles matching,
  execution, output parsing, permission field extraction.
  Depends on `setting/`, `llm/`, `plugin/`, `session/`.
- `app/` calls `hook.Engine` directly at app-layer event points
  (tool execution, session lifecycle, permissions, etc.).
- `core.Agent` has no dependency on hooks. Agent lifecycle events
  (streaming, inference, tool progress) flow through the Outbox
  for TUI rendering — a separate mechanism.

## Event Types

**24 hook event types:**

| Event | When fired | Matcher |
|-------|-----------|---------|
| `SessionStart` | Session initializes | source (`startup`, `resume`, `clear`, `compact`) |
| `SessionEnd` | Session terminates | reason (`clear`, `resume`, `logout`, `prompt_input_exit`) |
| `UserPromptSubmit` | User submits a prompt | — |
| `PreToolUse` | Before tool runs (app-level) | tool name |
| `PostToolUse` | After successful tool execution | tool name |
| `PostToolUseFailure` | After tool execution error | tool name |
| `PermissionRequest` | Permission check needed | tool name |
| `PermissionDenied` | Tool request denied | tool name |
| `Stop` | Assistant concludes response | — |
| `StopFailure` | Stop due to error | error type (`rate_limit`, `auth_failed`, `billing`, `server`, `max_tokens`, `unknown`) |
| `Notification` | System notification | notification_type |
| `SubagentStart` | Subagent starts | agent type |
| `SubagentStop` | Subagent finishes | agent type |
| `Setup` | System initialization | trigger (`init`, `maintenance`) |
| `TaskCreated` | Background task registered | — |
| `TaskCompleted` | Background task finishes | — |
| `ConfigChange` | Config file changes | source (`user_settings`, `project_settings`, `local_settings`) |
| `InstructionsLoaded` | Instruction file loaded | load_reason (`session_start`, `nested_traversal`, `path_glob_match`, `include`, `compact`) |
| `CwdChanged` | Working directory changes | — |
| `FileChanged` | Watched file modified | filename |
| `PreCompact` | Before compaction | trigger (`manual`, `auto`) |
| `PostCompact` | After compaction | trigger (`manual`, `auto`) |
| `WorktreeCreate` | Git worktree created | name |
| `WorktreeRemove` | Git worktree removed | worktree_path |

### Sync vs Async by Event

Events that need a decision use sync execution (caller blocks).
Events for observation use async execution (fire-and-forget).

| Sync (blocks, returns HookOutcome) | Async (fire-and-forget) |
|-------------------------------|------------------------|
| `PreToolUse` | `PostToolUse` |
| `PermissionRequest` | `PostToolUseFailure` |
| `UserPromptSubmit` | `StopFailure` |
| `PreCompact` | `Stop` (via `tea.Cmd`) |
| `SessionStart` | `SubagentStart` / `SubagentStop` |
| | `PostCompact` |
| | `SessionEnd` |
| | `FileChanged` |
| | `CwdChanged` |
| | `InstructionsLoaded` |
| | `TaskCreated` / `TaskCompleted` |
| | `WorktreeCreate` / `WorktreeRemove` |
| | `Notification` |
| | `ConfigChange` |

### TUI Thread Safety

Bubble Tea uses a single-threaded MVU loop: `Update` → `View` → render.
If `Update` blocks, the entire UI freezes — no rendering, no keyboard
input, nothing.

**Rule: Never call `Engine.Execute()` synchronously inside `Update` or
any method reachable from it, unless the hook is guaranteed to be fast.**

Slow hooks (external processes, network calls, LLM completions) will
freeze the UI for their entire duration. This caused a 9-12 second
freeze when `Stop` hooks ran synchronously in `ProcessTurnEnd`.

Choose the right pattern based on what the caller needs:

| Need outcome? | Pattern | Example |
|---|---|---|
| No | `ExecuteAsync` | `FileChanged`, `CwdChanged`, `PostToolUse` |
| Yes | Wrap in `tea.Cmd`, deliver result via custom `tea.Msg` | `Stop` (via `fireIdleHooksCmd` → `stopHookResultMsg`) |
| Yes + must block caller | `Execute` inside `tea.Cmd` (NOT inside `Update`) | `PreCompact` (inside `CompactCmd`), `PermissionRequest` (inside `DispatchPermissionHookAsync`) |

Events currently using `Execute` in the Update thread:
- `UserPromptSubmit` — intentional; must block to reject invalid input; typically fast
- `SessionStart` — runs at startup before TUI is interactive; acceptable

## Execution Pipeline

When `Execute(event, input)` is called, the engine runs three phases:

```
Match ─────────────────────────────────────
  L1: event type
      Hooks are bucketed by EventType string. Only hooks registered
      under the current event type are considered. The engine does
      not distinguish built-in from custom — all are string keys.

  L2: match pattern
      A single field that unifies filtering. The engine auto-detects
      the format:
        "Bash"          → exact match
        "Edit|Write"    → regex
        "Bash(git:*)"   → tool pattern with argument filter
      Empty or "*" matches everything.

  + once guard: hooks marked Once fire at most once per session.
───────────────────────────────────────────
  │
  ▼
Execute ───────────────────────────────────
  For each matched hook:
    sync  → run and collect HookOutcome
    async → go executeDetachedHook() (background, result discarded)

  Executors:
    Function → callback(ctx, input)
    Command  → sh -c subprocess, one-shot
    HTTP     → POST JSON to URL, response body=JSON
    Prompt   → single LLM completion, response=JSON

  Output JSON is parsed into HookOutcome fields.

  Command executor (one-shot):
    engine starts process, pipes HookInput JSON to stdin,
    waits for exit. stdout = final HookOutput JSON.

    Exit codes:
      0  → success, parse stdout as HookOutput
      2  → block (stderr = reason), ShouldBlock=true
      other → failure, logged and ignored

    First-line async detection: if stdout begins with
    {"async": true}, engine detaches the hook to background.
───────────────────────────────────────────
  │
  ▼
Merge ─────────────────────────────────────
  mergeOutcome() across all sync hooks.
  If any hook sets ShouldBlock=true, remaining hooks are skipped.
───────────────────────────────────────────
```

## HookOutcome

The pipeline returns a `HookOutcome`. Zero value = continue normally.

| Field | Scoped to | What it does |
|-------|-----------|-------------|
| `ShouldBlock` | any event | Reject the operation |
| `BlockReason` | any event | Reason fed back to the LLM |
| `AdditionalContext` | any event | Context injected into conversation |
| `UpdatedInput` | PreToolUse, PermissionRequest | Replace tool input parameters |
| `PermissionDecision` | PreToolUse | `"allow"` / `"deny"` / `"ask"` |
| `UpdatedPermissions` | PermissionRequest | `setMode` / `addRules` / `addDirectories` |
| `WatchPaths` | SessionStart, CwdChanged, FileChanged | Register file watcher paths |
| `InitialUserMessage` | SessionStart | Seed the first user turn |
| `Retry` | PermissionDenied | Resume assistant turn after denial |

Merge semantics (when multiple hooks fire):

- `ShouldBlock` — any true wins, short-circuits remaining hooks
- `AdditionalContext` — concatenate with newline
- `UpdatedInput` — last writer wins
- `PermissionDecision` — deny > ask > allow (most restrictive wins)
- `UpdatedPermissions` — accumulate (all updates applied)

## Hook Registration

```
Source           Lifetime            Where defined
──────────────────────────────────────────────────────────────
Config hooks     Permanent           settings.json, plugins
Session hooks    Session-scoped      AddSessionHook() / AddSessionFunctionHook()
```

Config hooks fire first, then session hooks.
Session hooks are cleared on session change (`ClearSessionHooks()`).

Session function hooks are in-memory Go callbacks:

```go
engine.AddSessionFunctionHook(event, matcher, hook) → hookID
engine.RemoveSessionFunctionHook(event, hookID) → bool
engine.ClearSessionHooks()
```

There is no runtime-scoped hook layer. All non-config hooks are
session-scoped and cleared when the session ends.

## Execution Modes

| Mode | Config | Behavior |
|------|--------|----------|
| **Sync** | (default) | Caller blocks. Results merge. Can short-circuit via `ShouldBlock=true`. |
| **Async** | `async: true` | Background goroutine, result discarded. For observation only. |
| **AsyncRewake** | `asyncRewake: true` | Background, but if it blocks (exit code 2), queues a notice for the model (see below). |

**AsyncRewake** solves a specific problem: hooks that are too slow to
block the conversation, but whose results matter if they find issues.

Example — a security scan hook on `Stop`:

```
1. Model completes response, about to idle
2. AsyncRewake hook runs security scan in background (30s)
3. User can keep typing — conversation not blocked
4. Scan finishes:
   - Clean → silent, same as async
   - Found vulnerability (exit 2) → notice queued → model wakes up and fixes it
```

Sync would freeze the conversation for 30s. Async would discard the
result. AsyncRewake is the middle ground.

Implementation: the TUI polls a queue every 500ms. When the model is
idle, it pops the notice and injects it as a user message to resume
the conversation. This indirection exists because Bubble Tea's MVU
loop does not allow external goroutines to push messages directly.

## Hook Input/Output

**Input** (JSON on stdin for command hooks, POST body for HTTP):

Common fields (always present):
```json
{
  "session_id": "...",
  "transcript_path": "/path/to/transcript",
  "cwd": "/current/working/dir",
  "hook_event_name": "PreToolUse",
  "permission_mode": "default"
}
```

Event-specific fields vary: `tool_name`, `tool_input`, `tool_response`,
`last_assistant_message`, `source`, `reason`, `file_path`, etc.

**Output** (JSON from stdout for command, response body for HTTP):

```json
{
  "continue": true,
  "stopReason": "...",
  "systemMessage": "...",
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow|deny|ask",
    "permissionDecisionReason": "...",
    "updatedInput": {},
    "additionalContext": "...",
    "watchPaths": [],
    "initialUserMessage": "..."
  }
}
```

**Exit code semantics** (command hooks only):

| Exit Code | Behavior |
|-----------|----------|
| 0 | Success, parse stdout as HookOutput |
| 1 | Non-blocking error, logged |
| 2 | Block operation, stderr = reason |
| other | Non-blocking error, logged |

## Permission-Related Hook Events

Three hook events participate in the permission pipeline (see
[permission.md](permission.md) for the full decision flow):

| Event | When | What hooks can do |
|-------|------|-------------------|
| `PreToolUse` | Before permission check | Return `permissionDecision`: `"allow"` / `"deny"` / `"ask"` to override the decision pipeline |
| `PermissionRequest` | When decision is Ask, before user dialog | Return `behavior` + `updatedPermissions` to decide on behalf of the user and update rules |
| `PermissionDenied` | After user denies | Return `retry: true` to resume the assistant turn |

Constraints:
- `PreToolUse` cannot return `updatedPermissions` — that is exclusive
  to `PermissionRequest`.
- `PermissionDecision` merge: deny > ask > allow (most restrictive wins
  when multiple hooks fire).

## Configuration

Hooks are configured in `settings.json` under `hooks`:

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{
        "type": "command",
        "command": "audit-tool.sh",
        "timeout": 30,
        "if": "Bash(git *)",
        "statusMessage": "Auditing..."
      }]
    }],
    "Stop": [{
      "hooks": [{ "type": "command", "command": "notify.sh", "async": true }]
    }]
  }
}
```

**Hook options:**

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `"command"` (default), `"prompt"`, `"agent"`, `"http"` |
| `command` | string | Shell command (command type) |
| `prompt` | string | LLM prompt template, `$ARGUMENTS` substituted (prompt/agent type) |
| `url` | string | HTTP endpoint URL (http type) |
| `shell` | string | `"sh"` (default) or `"powershell"` |
| `model` | string | Override default hook model (prompt/agent type) |
| `async` | bool | Fire in background, discard result |
| `asyncRewake` | bool | Background, inject queue if blocks |
| `timeout` | int | Timeout in seconds (default 600) |
| `statusMessage` | string | UI status message while running |
| `once` | bool | Fire at most once per session |
| `if` | string | Tool pattern condition (e.g. `"Bash(cd *)"`) |
| `headers` | map | HTTP headers with env var interpolation |
| `allowedEnvVars` | list | Environment variables allowed in header interpolation |

**Settings sources and priority:**

| Source | Path | Priority |
|--------|------|----------|
| User | `~/.gen/settings.json` | base |
| Project | `.gen/settings.json` | overrides user per event type |
| Plugin | `plugin.json` hooks | lowest |

Project settings override user settings **per event type** — if a project
defines `Stop` hooks, all user-level `Stop` hooks are replaced.

## Dependencies

```
app/ → hook/
         ↓
       setting/, llm/, plugin/, session/

core.Agent has no hook dependency.
```
