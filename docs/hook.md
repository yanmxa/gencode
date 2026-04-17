# Hook Engine Architecture

## What It Does

Hook Engine lets external code react to agent lifecycle events — intercept
operations before they happen (deny a tool call, rewrite its input, set
LLM context) or observe them after the fact (log, notify, audit).

External code can be a shell command, HTTP endpoint, LLM prompt, or
in-memory Go function.

## Design Principles

**Generic engine, no domain knowledge.** The hook engine provides generic
capabilities (Deny, Rewrite, SetContext, Extra). It does not understand
the semantics of any specific application built on top of it. For example,
permission automation is an app-layer application of hooks — the hook
engine has no knowledge of allow/deny/ask/setMode. Domain-specific
interpretation stays in the caller.

**Built-in events are lifecycle only.** The engine defines a small set of
agent lifecycle events (PreInfer, PreTool, etc). `EventType` is
`type EventType string` — extensible at the code level when new events
are needed, without changing the hook engine.

## Structure

```
internal/core/                        internal/hook/
┌──────────────────────────┐         ┌──────────────────────────────────┐
│ Hooks interface          │◄─impl──│ Engine                           │
│   Register / On / Wait   │         │                                  │
│                          │         │  On(event):                      │
│ Event { Type, Source }   │         │    match → execute → merge       │
│ Action { Deny, SetContext,│        │    → return Action               │
│      Rewrite, Extra }    │         │                                  │
└──────────────────────────┘         │  ExecuteAsync(event):            │
                                     │    fire-and-forget, no result    │
                                     ├──────────────────────────────────┤
                                     │ Store     config + code hooks    │
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
- `app/` wires them together: `hook.AsCoreHooks(engine)` wraps the Engine as
  `core.Hooks` and injects it into the Agent.

## Event Sources

Events reach the hook engine from two places. Both go through the same
`core.Hooks.On()` interface and receive the same `core.Action` back.

### Agent lifecycle (built-in events)

The Agent calls `emitAndFire()` at two decision points in its Run Loop.
All other lifecycle points use `emit()` (Outbox only, no hook).

```
ThinkAct() loop
  emitAndFire(PreInfer)   → Deny | SetContext
  streamInfer()
  execTools()
    emitAndFire(PreTool)  → Deny | Rewrite
    tool.Execute()
```

`emit` = send to Outbox for TUI observation.
`emitAndFire` = emit + `hooks.On()`, returns Action that controls behavior.

### App layer (custom events)

The app layer can fire its own event types via `hooks.On()`. The engine
treats them identically to built-in events. `EventType` is extensible
(`type EventType string`), so new events can be added at the code level
without changing the hook engine.

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
    sync  → run and collect Action
    async → go executeDetachedHook() (background, result discarded)

  Executors:
    Function → callback(ctx, input)
    Command  → sh -c subprocess, stdin=JSON, stdout=JSON
    HTTP     → POST JSON to URL, response body=JSON
    Prompt   → single LLM completion, response=JSON

  Bidirectional mode (Command executor only):
    During execution, a hook process can request input from the
    caller by writing a PromptRequest to stdout. The engine pauses,
    delegates to a PromptCallback (provided by app layer), writes
    the response back to the hook's stdin, and the hook continues.
    The hook decides the final Action based on the response.

    Example: a PreTool hook asks "Allow rm -rf?" → app shows a
    dialog → user says No → hook returns Deny. The hook controls
    the outcome, not the engine.

  Output JSON is parsed and converted to core.Action.
───────────────────────────────────────────
  │
  ▼
Merge ─────────────────────────────────────
  MergeActions() across all sync hooks.
  If any hook sets Deny=true, remaining hooks are skipped.
───────────────────────────────────────────
```

## Action

The pipeline returns an `Action`. Zero value = continue normally.

```
Field              When used      What it does
──────────────────────────────────────────────────────────────────
Deny + Reason      any event      Reject the operation. The Reason
                                  is fed back to the LLM as an error.

SetContext         PreInfer        Set temporary context in the system
                                  prompt for the next LLM call.
                                  Replaces previous value each turn.

Rewrite            PreTool         Replace tool input parameters
                                  before execution. e.g. rewrite
                                  "rm -rf /" to "echo blocked".

Extra              any event       Opaque map for app-layer extensions.
                                  core.Agent ignores it. App reads it
                                  for domain-specific behavior.
```

Merge semantics (when multiple hooks fire):

- `Deny` — any true wins, short-circuits remaining hooks
- `SetContext` — concatenate (final result overwrites previous turn's slot)
- `Rewrite` — last writer wins
- `Extra` — merge maps, last writer wins per key

## Hook Registration

```
Source           Lifetime            Where defined
──────────────────────────────────────────────────────────────
Config hooks     Permanent           settings.json, plugins
Code hooks       runtime-scoped      AddHook(scope: runtime)
                 session-scoped      AddHook(scope: session)
```

Config hooks fire first, then code hooks.
Session-scoped hooks are cleared on session change.

## Sync vs Async

**Sync** — caller blocks, results merge. Used when the caller needs a
decision. Any hook can short-circuit the chain via `Deny=true`.

**Async** — background goroutine, result discarded. Used for observation
(logging, notifications, auditing).

**AsyncRewake** — async variant where a blocking result (exit code 2) feeds
back into the agent. Driven by Bubble Tea's constraint that external
goroutines cannot push into the MVU loop directly:

```
background hook blocks (exit 2) → AsyncHookCallback → TUI queue
  → ticker polls (500ms) → pop when idle → inject into agent conversation
```

## Dependencies

```
app/ → hook/ → core/
                 ↑
                 zero deps — Agent depends only on this

hook/ also depends on: setting/, llm/, plugin/, session/
```
