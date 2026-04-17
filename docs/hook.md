# Hook Engine Architecture

## What It Does

Hook Engine lets external code react to agent lifecycle events — intercept
operations before they happen (block a tool call, modify its input, inject
LLM context) or observe them after the fact (log, notify, audit).

External code can be a shell command, HTTP endpoint, LLM prompt, or
in-memory Go function.

## Design Principles

**Generic engine, no domain knowledge.** The hook engine provides generic
capabilities (Block, Modify, Inject, Meta). It does not understand the
semantics of any specific application built on top of it. For example,
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
│ Action { Block, Inject,  │         │    → return Action               │
│          Modify, Meta }  │         │                                  │
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
  emitAndFire(PreInfer)   → Action: Block | Inject
  streamInfer()
  execTools()
    emitAndFire(PreTool)  → Action: Block | Modify
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

  Output JSON is parsed and converted to core.Action.
───────────────────────────────────────────
  │
  ▼
Merge ─────────────────────────────────────
  MergeActions() across all sync hooks.
  If any hook sets Block=true, remaining hooks are skipped.
───────────────────────────────────────────
```

## Action

The pipeline returns an `Action`. Zero value = continue normally.

```
Field           When used        What it does
──────────────────────────────────────────────────────────────────
Block + Reason  any event        Stop the operation. Reason is fed
                                 back to the LLM as an error.

Inject          PreInfer         Add temporary context to the system
                                 prompt. Ephemeral — replaced each turn.

Modify          PreTool          Replace tool input parameters.
                                 e.g. rewrite a dangerous command.

Meta            any event        Opaque map for app-layer extensions.
                                 core.Agent ignores it. App reads it
                                 for domain-specific behavior.
```

Merge semantics (when multiple hooks fire):

- `Block` — any true wins, short-circuits remaining hooks
- `Inject` — concatenate with newline
- `Modify` — last writer wins
- `Meta` — merge maps, last writer wins per key

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
decision. Any hook can short-circuit the chain via `Block=true`.

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
