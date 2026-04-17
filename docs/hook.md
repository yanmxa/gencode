# Hook Engine Architecture

## What It Does

Hook Engine lets external code react to agent lifecycle events — intercept
operations before they happen (deny a tool call, rewrite its input, set
LLM context) or observe them after the fact (log, notify, audit).

External code can be a shell command, HTTP endpoint, LLM prompt, or
in-memory Go function.

## Design Principles

**Dual event pathway.** Agent lifecycle has two independent channels:
`emit()` sends events to the Outbox for TUI rendering; `hooks.On()`
fires extensibility hooks and returns `HookOutcome` to control behavior.
They serve different purposes and do not interfere with each other.

**Engine understands permissions.** The engine directly parses and returns
permission-related fields (`PermissionDecision`, `UpdatedPermissions`).
This is the only domain coupling — everything else (block, modify, inject)
is generic.

## Structure

```
internal/core/                        internal/hook/
┌──────────────────────────┐         ┌──────────────────────────────────┐
│ Hooks interface          │◄─impl──│ Engine                           │
│   Register / On / Wait   │         │                                  │
│                          │         │  On(event):                      │
│ Event { Type, Source }   │         │    match → execute → merge       │
│                          │         │    → return HookOutcome          │
│ HookOutcome:             │         │                                  │
│   ShouldBlock bool       │         │                                  │
│   BlockReason string     │         │                                  │
│   AdditionalContext str  │         │                                  │
│   UpdatedInput map       │         │                                  │
│   PermissionDecision str │         │  "allow" | "deny" | "ask"       │
│   UpdatedPermissions []  │         │  setMode/addRules/addDirs       │
│   WatchPaths []string    │         │                                  │
│   InitialUserMessage str │         │                                  │
│   Retry bool             │         │                                  │
└──────────────────────────┘         │  ExecuteAsync(event):            │
                                     │    fire-and-forget, no result    │
                                     ├──────────────────────────────────┤
                                     │ Store     config + session hooks │
                                     │ Matcher   2-layer filter         │
                                     │ Status    active hook tracking   │
                                     ├──────────────────────────────────┤
                                     │ Executors                        │
                                     │  Command  sh -c, stdin/stdout    │
                                     │  HTTP     POST JSON to URL       │
                                     │  Prompt   single LLM completion  │
                                     │  Function in-memory callback     │
                                     └──────────────────────────────────┘
```

- `core/` defines the interface (`Hooks`, `Event`, `Action`). Zero external
  imports. The Agent depends only on this.
- `hook/` provides the implementation (`Engine`). Handles matching, execution,
  output parsing. Depends on `setting/`, `llm/`, `plugin/`, `session/`.
- `app/` wires them together: injects the Engine as `core.Hooks` into
  the Agent. TUI rendering is driven by the Outbox (emit), not hooks.

## Agent Lifecycle Events

The Agent has two event channels at each lifecycle point:

- `emit(event)` — sends to Outbox for TUI rendering (always fires)
- `hooks.On(event)` — fires extensibility hooks, returns HookOutcome (only at decision points)

```
core.Agent Run Loop
  emit(PreInfer)                        → TUI: show "thinking..."
  hooks.On(PreInfer)                    → HookOutcome: inject context
  streamInfer()
    emit(OnChunk)                       → TUI: render streaming tokens
  emit(PostInfer)                       → TUI: update state
  execTools()
    emit(PreTool)                       → TUI: show tool name
    hooks.On(PreTool)                   → HookOutcome: block | modify input
    tool.Execute()
    emit(PostTool)                      → TUI: show tool result
  emit(OnTurn)                          → TUI: turn complete
```

`emit` is fire-and-forget to the Outbox channel — the TUI observes.
`hooks.On` is synchronous — the returned HookOutcome controls agent behavior.

## App-Layer Events

Beyond agent lifecycle events, the app layer fires its own events via the
hook engine. These cover session lifecycle, permissions, compaction, and
system-level concerns.

### Event Types

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

| Sync (blocks, returns Action) | Async (fire-and-forget) |
|-------------------------------|------------------------|
| `PreToolUse` | `PostToolUse` |
| `PermissionRequest` | `PostToolUseFailure` |
| `UserPromptSubmit` | `StopFailure` |
| `Stop` | `SubagentStart` / `SubagentStop` |
| `PreCompact` | `PostCompact` |
| `SessionStart` | `SessionEnd` |
| `FileChanged` | `InstructionsLoaded` |
| `CwdChanged` | `TaskCreated` / `TaskCompleted` |
| | `WorktreeCreate` / `WorktreeRemove` |
| | `Notification` |
| | `ConfigChange` |

## Execution Pipeline

When `On(event)` is called, the engine runs three phases:

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

## Async Hooks

Three modes:

**Sync** (default) — caller blocks, results merge. Used when the caller
needs a decision. Any hook can short-circuit the chain via `ShouldBlock=true`.

**Async** (`async: true`) — background goroutine, result discarded. Used
for observation (logging, notifications, auditing).

**AsyncRewake** (`asyncRewake: true`) — async variant where a blocking
result (exit code 2) feeds back into the agent. Driven by Bubble Tea's
constraint that external goroutines cannot push into the MVU loop directly:

```
background hook blocks (exit 2) → AsyncHookCallback → TUI queue
  → ticker polls (500ms) → pop when idle → inject into agent conversation
```

First-line async detection: if a command hook's first stdout line is
`{"async": true}`, the engine detaches it to run in background.

`asyncRewake` differs from first-line `{"async":true}` detach: it is
configured declaratively in settings and only triggers a re-wake when
the background hook finishes with a blocking result.

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

## Permission Integration

Hooks integrate with the permission system at three points:

```
Tool call
  ↓
PreToolUse hook (sync)
  ├─ permissionDecision: "allow" → skip permission check
  ├─ permissionDecision: "deny"  → block tool
  ├─ permissionDecision: "ask"   → force permission prompt
  └─ updatedInput: {...}         → rewrite tool params
  ↓
Permission rules check (settings.json allow/deny rules)
  ↓
PermissionRequest hook (sync)
  ├─ behavior: "allow" / "deny"
  ├─ updatedInput: {...}
  └─ updatedPermissions: [
       {"type": "setMode", "mode": "bypassPermissions", "destination": "session"},
       {"type": "addRules", "rules": [...], "behavior": "allow", "destination": "persistent"},
       {"type": "addDirectories", "directories": [...], "destination": "session"}
     ]
  ↓
User dialog (if needed)
  ↓
PermissionDenied hook (if denied)
  └─ retry: true → resume assistant turn
```

PreToolUse cannot inject `updatedPermissions` — that capability is
exclusive to PermissionRequest hooks.

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
app/ → hook/ → core/
                 ↑
                 zero deps — Agent depends only on this

hook/ also depends on: setting/, llm/, plugin/, session/
```
