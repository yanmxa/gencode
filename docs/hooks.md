# Hook Engine Architecture

Hook Engine is an event-driven intercept/observe system. External code (shell
commands, HTTP, LLM prompts, in-memory functions) reacts to agent lifecycle
events. Two-layer design: abstract interface in `core/`, full implementation
in `hook/`.

## Structure

```
internal/core/                        internal/hook/
┌──────────────────────────┐         ┌──────────────────────────────────┐
│ Hooks interface          │◄─impl──│ Engine                           │
│   Register / On / Wait   │         │                                  │
│                          │         │  On(event):                      │
│ Event { Type, Source }   │         │    Go handlers → coreToEngine   │
│ Action { Block, Inject,  │         │    → Execute() → MergeActions   │
│          Modify, Meta }  │         │                                  │
│                          │         │  Execute():  match → run → merge│
│ agent_impl.go:           │         │  ExecuteAsync(): fire-and-forget│
│   emitAndFire(event) {   │         ├──────────────────────────────────┤
│     emit → Outbox (TUI)  │         │ Store     4 hook maps           │
│     hooks.On → Action    │         │ Registry  3-layer match filter  │
│   }                      │         │ Matcher   regex + matchValue    │
└──────────────────────────┘         │ Status    active hook tracking  │
                                     ├──────────────────────────────────┤
                                     │ Executors                       │
                                     │  Command  sh -c, stdin/stdout   │
                                     │  HTTP     POST JSON to URL      │
                                     │  Prompt   single LLM completion │
                                     │  Function in-memory callback    │
                                     └──────────────────────────────────┘
```

## Action

A hook returns an `Action` — the zero value means "continue normally."

```
Block + Reason    Stop the current operation. Agent skips the tool call or
                  halts inference. Reason is fed back to the LLM as an error.

Inject            Add temporary context to the system prompt for the next
                  LLM call. Only effective on PreInfer. Ephemeral — replaced
                  each turn, not accumulated.

Modify            Replace tool input parameters. Only effective on PreTool.
                  e.g. rewrite a dangerous Bash command to a safer variant.

Meta              Transparent map for app-layer data that core.Agent does not
                  consume. hook/ packs permissions, watch paths, retry flags,
                  etc. into Meta; app/ unpacks and applies them.
```

When multiple hooks fire for the same event, actions merge: Block — any true
wins (short-circuits); Inject — concatenate; Modify — last writer wins;
Meta — merge maps.

## Two Integration Paths

### Path 1: Agent lifecycle (core.Agent → core.Hooks)

Agent calls `emitAndFire()` at key points. Hooks run synchronously inside
the Agent Run Loop goroutine and return an Action.

```
Run()
  emit(StartEvent)
  waitForInput()
    emit(MessageEvent)
  ThinkAct() loop
    emitAndFire(PreInfer)  → Block | Inject
    streamInfer()
      emit(Chunk)
    emit(PostInfer)
    execTools()
      emitAndFire(PreTool)  → Block | Modify
      tool.Execute()
      emit(PostTool)
  emitFinal(StopEvent)
  hooks.Wait()
```

`emit` = send to Outbox for TUI observation only.
`emitAndFire` = emit + hooks.On(), returns Action that controls agent behavior.

### Path 2: App layer (TUI → hook.Engine directly)

App layer calls `hook.Engine` for events outside the agent lifecycle.

```
Sync:   SessionStart, UserPromptSubmit, PermissionRequest, CwdChanged, FileChanged, Stop
Async:  Setup, Notification, InstructionsLoaded, TaskCreated, WorktreeCreate, ...
```

Domain packages that can't import `hook/` use observer bridges in `app/hooks.go`:

```
task/      ──observer──► taskHookBridge      ──► ExecuteAsync(TaskCreated/Completed)
worktree/  ──observer──► worktreeHookBridge  ──► ExecuteAsync(WorktreeCreate/Remove)
plugin/mcp ──observer──► configHookBridge    ──► ExecuteAsync(ConfigChange/FileChanged)
```

## Execution Pipeline

```
Event arrives
  │
  ▼
getMatchingHooks()              ← 3-layer filter
  L1: event type bucket
  L2: matcher regex on matchValue (e.g. ToolName, FilePath)
  L3: if condition (tool pattern, e.g. Bash(git:*))
  + once guard
  │
  ▼
For each matched hook:
  ├─ async?  → go executeDetachedHook()
  └─ sync   → executeMatchedHook()
               ├─ Function → callback(ctx, input)
               └─ Command  → dispatch by type
               ↓
             parseOutput → applyHookOutput → HookOutcome
  │
  ▼
mergeOutcome()                  ← across all sync hooks
  AdditionalContext: concatenate
  UpdatedInput: last writer wins
  PermissionAllow: any true wins
  ShouldContinue=false: stop chain
  │
  ▼
outcomeToAction() → core.Action
  Block/Reason, Modify, Inject, Meta
```

## Hook Registration

```
Priority  Source           Lifetime          API
────────────────────────────────────────────────────────────
1         Settings/plugins Permanent         settings.json hooks map
2         Runtime hooks    Process-scoped    AddRuntimeHook/FunctionHook()
3         Session hooks    Session-scoped    AddSessionHook/FunctionHook()
4         Core Go handlers Via Register()    Register(core.Hook{...})
```

Core Go handlers fire BEFORE config-driven hooks in `Engine.On()`.

## Sync vs Async

**Sync** (`Execute`): caller blocks, results merge, any hook can stop the chain.
Used when the caller needs a decision (block/allow/modify).

**Async** (`ExecuteAsync`): each hook runs in a background goroutine, results
discarded. Used for observation-only events.

**AsyncRewake**: async hook that blocks (exit 2) triggers `AsyncHookCallback`
→ TUI queue → ticker polls when idle → injects continuation prompt into agent.

## Dependencies

```
app/ → hook/ → core/ (zero deps)
               ▲
hook/ also → setting/, llm/, plugin/, session/

Domain packages (task/, worktree/, plugin/, mcp/)
  can't import hook/ → bridged via app/hooks.go observer pattern
```
