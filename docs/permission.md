# Permission

Permission is a tool-execution concern. All permission types, decisions, and enforcement
live under `internal/tool/`.

## Package Layout

```
internal/tool/
│
├── perm/                        ← consolidated permission package
│   ├── decision.go                 Decision, Checker, built-in checkers, AsPermissionFunc
│   ├── safetool.go                 IsSafeTool, IsReadOnlyTool
│   ├── types.go                    PermissionRequest, DiffMetadata, BashMetadata, ...
│   └── diff.go                     GenerateDiff, GeneratePreview
│
├── permission.go                ← PermissionFunc, WithPermission decorator (package tool)
│
├── adapter.go                      AdaptToolRegistry, toolAdapter
├── set.go                          Set (schema resolution: Allow/Disallow)
├── types.go                        Tool, PermissionAwareTool, InteractiveTool
├── call.go                         PrepareToolCall
└── ...
```

- `tool/perm/` — all permission primitives: decision types, checkers, safe-tool lists,
  approval-dialog metadata, diff computation.
- `tool/permission.go` — `WithPermission` decorator that wires a `PermissionFunc`
  into `core.Tools`.

## Decision Pipeline

Permission decisions combine **static rules** (settings files) and **dynamic state**
(user actions during a session). Each tool call is evaluated through a 6-step pipeline;
**any step that produces a terminal decision returns immediately** — later steps are skipped.

### Rule sources

**Static** — loaded at startup, merged by priority:

```
~/.gen/settings.json              user level
.gen/settings.json                project level
.gen/settings.local.json          local level (gitignored)
```

```json
{
  "permissions": {
    "allow": ["Bash(npm:*)", "Read(**/*.go)"],
    "deny":  ["Write(**/.env)"],
    "ask":   ["Bash(git:push *)"]
  }
}
```

**Dynamic** — accumulated at runtime via `SessionPermissions`:

| Action | Stored as |
|--------|-----------|
| User clicks "Allow" | one-shot approval (not stored) |
| User clicks "Always allow" | `AllowedTools["Edit"]` |
| User clicks "Always allow" with pattern | `AllowedPatterns["Bash(npm:*)"]` |
| `/auto-accept` | `AllowAllEdits`, `AllowedPatterns`, `WorkingDirectories` |
| `/bypass` | `Mode = ModeBypassPermissions` |

### Pipeline

```
tool call: Bash("npm install lodash")
│
│  Step 1: Hard blocks (bypass-immune, terminal)
│  ├─ deny rules match?                → Deny ■
│  ├─ sensitive path / destructive cmd? → Ask  ■
│  ├─ outside working directory?        → Ask  ■
│  └─ ask rules match?                 → Ask  ■
│
│  Step 2: Bypass mode
│  └─ Mode == BypassPermissions?        → Allow ■
│
│  Step 3: Session permissions
│  ├─ AllowedTools["Bash"]?             → Allow ■
│  └─ AllowedPatterns match?            → Allow ■
│
│  Step 4: Static allow rules
│  └─ settings.allow match?            → Allow ■
│
│  Step 5: Default
│  └─ safe tool → Allow ■              → otherwise Ask (continue)
│
│  Step 6: Mode transform
│  └─ Mode == DontAsk + Ask?            → Deny ■
│
▼
final Decision
```

`■` = terminal — returns immediately, skips remaining steps.

Step 1 cannot be overridden by any later step. Session permissions (step 3) are
checked before static allow rules (step 4) — runtime grants take precedence.

### Where rules flow into PermissionFunc

```
TUI path (full pipeline):

  Settings + SessionPermissions
       │
       ▼
  settings.HasPermissionToUseTool(name, args, session)   ← 6-step pipeline
       │
       ▼
  PermissionBridge.PermissionFunc() → tool.PermissionFunc
       │
       ▼
  tool.WithPermission(tools, permFn)


Subagent path (fixed policy, no settings):

  PermissionMode (plan / acceptEdits / default)
       │
       ▼
  perm.ReadOnly() / perm.AcceptEdits() / perm.PermitAll()
       │
       ▼
  perm.AsPermissionFunc(checker) → tool.PermissionFunc
       │
       ▼
  tool.WithPermission(tools, permFn)
```

## Types

### `tool/perm/decision.go`

```go
package perm

type Decision int

const (
    Permit Decision = iota   // auto-execute
    Reject                    // block
    Prompt                    // ask user
)

type Checker interface {
    Check(name string, params map[string]any) Decision
}

func PermitAll() Checker    { ... }
func ReadOnly() Checker     { ... }
func DenyAll() Checker      { ... }
func AcceptEdits() Checker  { ... }

// AsPermissionFunc converts a Checker to a tool.PermissionFunc.
// Reject → (false, reason), Permit/Prompt → (true, "").
func AsPermissionFunc(c Checker) tool.PermissionFunc {
    if c == nil {
        return nil
    }
    return func(ctx context.Context, name string, input map[string]any) (bool, string) {
        if c.Check(name, input) == Reject {
            return false, fmt.Sprintf("tool %s is not permitted in this mode", name)
        }
        return true, ""
    }
}
```

### `tool/perm/safetool.go`

```go
package perm

func IsSafeTool(name string) bool     { ... }   // Read, Glob, Grep, TaskCreate, ...
func IsReadOnlyTool(name string) bool { ... }   // Read, Glob, Grep, WebFetch, ...
```

### `tool/permission.go`

```go
package tool

// PermissionFunc gates tool execution.
// Called with tool name and parsed input. May block (e.g., TUI approval).
type PermissionFunc func(ctx context.Context, name string, input map[string]any) (allow bool, reason string)

// WithPermission wraps core.Tools with permission checking.
// Safe tools (perm.IsSafeTool) bypass the check automatically.
// nil check returns inner unchanged (permit-all).
func WithPermission(inner core.Tools, check PermissionFunc) core.Tools {
    if check == nil {
        return inner
    }
    return &permissionTools{inner: inner, check: check}
}
```

`permissionTool.Execute` handles `IsSafeTool` bypass internally — callers
never need to check it themselves:

```go
func (pt *permissionTool) Execute(ctx context.Context, input map[string]any) (string, error) {
    if !perm.IsSafeTool(pt.inner.Name()) {
        if allow, reason := pt.check(ctx, pt.inner.Name(), input); !allow {
            return "", fmt.Errorf("blocked: %s", reason)
        }
    }
    return pt.inner.Execute(ctx, input)
}
```

## Tool Decorator Structure

`WithPermission` is a transparent decorator. The agent only sees `core.Tools` /
`core.Tool` — it has no knowledge of permission.

### Interfaces

```
┌───────────────────────────┐      ┌─────────────────────────┐
│       core.Tools          │      │       core.Tool         │
│                           │      │                         │
│  Get(name string) Tool    │      │  Name() string          │
│  All() []Tool             │      │  Description() string   │
│  Add(tool Tool)           │      │  Schema() ToolSchema    │
│  Remove(name string)      │      │  Execute(ctx, input)    │
│  Schemas() []ToolSchema   │      │    → (string, error)    │
└───────────────────────────┘      └─────────────────────────┘
```

### Base implementation (no permission)

```
┌──────────────────────────────────────────────────────────┐
│  core.toolSet  (implements core.Tools)                   │
│                                                          │
│  tools: map[string]core.Tool                             │
│                                                          │
│  Get("Bash") → toolAdapter{Bash}                         │
│  Get("Read") → toolAdapter{Read}                         │
└──────────────────────────────────────────────────────────┘
```

### With permission decorator

```
┌──────────────────────────────────────────────────────────┐
│  permissionTools  (implements core.Tools)                 │
│                                                          │
│  inner ──► core.toolSet                                  │
│  check ──► PermissionFunc                                │
│                                                          │
│  Get("Bash")                                             │
│    └─► inner.Get("Bash") → toolAdapter{Bash}             │
│    └─► wrap → permissionTool{inner: Bash, check: fn}     │
│                                                          │
│  All(), Add(), Remove(), Schemas()                       │
│    └─► delegate to inner (no wrapping)                   │
└──────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────┐
│  permissionTool  (implements core.Tool)                   │
│                                                          │
│  inner ──► toolAdapter{Bash}                             │
│  check ──► PermissionFunc                                │
│                                                          │
│  Execute(ctx, input):                                    │
│    1. perm.IsSafeTool(name)?  → skip to step 3           │
│    2. check(ctx, name, input) → blocked? return error    │
│    3. inner.Execute(ctx, input)                          │
└──────────────────────────────────────────────────────────┘
```

### Construction pipeline

```
1. base         tools := tool.AdaptToolRegistry(schemas, cwd)
                tools.Add(mcpTools...)
                                    │
                                    ▼
                          ┌───────────────────┐
                          │   core.toolSet    │
                          │   {Bash, Read, …} │
                          └─────────┬─────────┘
                                    │
2. wrap         tools = tool.WithPermission(tools, permFn)
                                    │
                                    ▼
                          ┌───────────────────┐
                          │ permissionTools   │
                          │  inner ──► toolSet│
                          │  check ──► permFn │
                          └─────────┬─────────┘
                                    │
3. inject       core.NewAgent(Config{ Tools: tools })
                                    │
                                    ▼
                          ┌───────────────────┐
                          │    core.Agent     │
                          │                   │
                          │ sees core.Tools   │
                          │ (no permission    │
                          │  knowledge)       │
                          └───────────────────┘
```

## Execution Flow

### execTools

```
agent.execTools(calls)
│
├── Phase 1: emit + resolve
│   for each ToolCall:
│     ├── emit PreToolEvent
│     └── tools.Get(name)   → returns permissionTool (or nil)
│         └── nil? → appendResult(error), skip
│
├── Phase 2: execute (parallel when multiple)
│   permissionTool.Execute(ctx, input)
│     ├── IsSafeTool? → skip permission check
│     ├── check(ctx, name, input)
│     │     blocked → return error
│     │     allowed → continue
│     └── inner.Execute(ctx, input) → (content, error)
│
└── Phase 3: record results
    appendResult + emit PostToolEvent
```

### TUI prompt

```
agent goroutine                        TUI goroutine
     │                                      │
     │ permissionTool.Execute()             │
     │ │                                    │
     │ ├─ IsSafeTool? → skip to Execute     │
     │ │                                    │
     │ ├─ check(ctx, name, input)           │
     │ │   decideFn:                        │
     │ │     Permit ──────────────► allow    │
     │ │     Reject ──────────────► block    │
     │ │     Prompt ─┐                      │
     │ │             │                      │
     │ │   send req ─┼────────────────────────► PollPermBridge
     │ │   (block)   │                      │   ├─ show dialog
     │ │             │                      │   ├─ user decides
     │ │             │                      │   └─ send response
     │ │   recv ◄────┼──────────────────────────
     │ │             │                      │
     │ │   return (allow, reason)           │
     │ │                                    │
     │ ├─ allowed → inner.Execute()         │
     │ └─ blocked → return error            │
```

### Parallel tool calls

```
execTools([Read, Bash, Write])           bridge channel (size 1)
                                         serializes TUI prompts
Phase 2: three goroutines
                                         ┌─────────────────────┐
  Read           Bash           Write    │   PermBridgeReq     │
  │              │               │       │   chan (cap=1)       │
  │              │               │       └─────────────────────┘
  IsSafeTool     check           check
  → skip check   │               │
  │              Prompt          Prompt
  Execute()      │               │
  done           send ──────►    blocked (chan full)
                 block           │
                 │               │
                 ◄── respond     │
                 Execute()       send ──────►
                 done            block
                                 │
                                 ◄── respond
                                 Execute()
                                 done

Phase 3: record in [Read, Bash, Write] order
```

## PermissionFunc Implementations

### TUI (PermissionBridge)

`IsSafeTool` bypass is handled by the decorator — the bridge only deals with
its own Permit/Reject/Prompt logic.

```go
// app/output/permission_bridge.go

func (pb *PermissionBridge) PermissionFunc() tool.PermissionFunc {
    return func(ctx context.Context, name string, input map[string]any) (bool, string) {
        decision := pb.decideFn(name, input)
        switch decision.Decision {
        case perm.Permit:
            return true, decision.Reason
        case perm.Reject:
            return false, decision.Reason
        default: // Prompt
            req := &PermBridgeRequest{
                ToolName: decision.ToolName, Description: decision.Description,
                Input: input, Response: make(chan PermBridgeResponse, 1),
            }
            select {
            case pb.requests <- req:
            case <-ctx.Done():
                return false, "cancelled"
            }
            select {
            case resp := <-req.Response:
                return resp.Allow, resp.Reason
            case <-ctx.Done():
                return false, "cancelled"
            }
        }
    }
}
```

### Subagent

```go
// subagent/executor.go

permFn := perm.AsPermissionFunc(perm.ReadOnly())
tools = tool.WithPermission(tools, permFn)
```

### Test

```go
// tests/integration/testutil/core_helpers.go

func NewTestAgentWithPermission(t *testing.T, permFn tool.PermissionFunc, ...) (core.Agent, *FakeLLM) {
    tools := buildAllRegisteredTools(cwd)
    if permFn != nil {
        tools = tool.WithPermission(tools, permFn)
    }
    return core.NewAgent(core.Config{
        Tools: tools,
        ...
    }), fakeLLM
}
```

## Hook Integration

Hooks participate in the permission pipeline at the **app layer** — they sit
around the tool-layer `PermissionFunc`, not inside it. Three hook events form
the extension points (see [hook.md](hook.md) for hook mechanics):

```
tool call arrives
│
│  ① PreToolUse hook (sync, app layer)
│     Hook can return permissionDecision:
│       "allow" → skip permission check entirely          ──► execute tool
│       "deny"  → block tool immediately                  ──► return error
│       "ask"   → force interactive prompt
│     Hook can also return updatedInput to rewrite params.
│
│  ② permissionTool.Execute  (tool layer)
│     PermissionFunc runs the 6-step decision pipeline.
│     Result: Permit / Reject / Prompt
│
│     If Prompt:
│       ③ PermissionRequest hook (sync, app layer)
│          Hook can decide on behalf of the user:
│            behavior: "allow" / "deny"
│          Hook can update rules:
│            updatedPermissions: [
│              {type: "setMode",  mode: "bypassPermissions"},
│              {type: "addRules", rules: [...], behavior: "allow"},
│              {type: "addDirectories", directories: [...]}
│            ]
│          If no hook decides → show user dialog.
│
│       User dialog → approve / deny
│
│       If denied:
│         ④ PermissionDenied hook (async, app layer)
│            retry: true → resume assistant turn
│
│  tool executes (or blocked)
```

Key points:
- Hooks are **app-layer orchestration** — `core.Agent` and `permissionTool`
  know nothing about them.
- `PreToolUse` hook runs **before** the tool-layer permission check. Its
  `permissionDecision` can short-circuit the entire pipeline.
- `PermissionRequest` hook runs **after** the pipeline returns `Prompt`,
  giving external code a chance to auto-approve before the user dialog.
- `PreToolUse` cannot return `updatedPermissions` — that is exclusive to
  `PermissionRequest`.

## TODO

- **Tool-level checkPermissions**: CC allows each tool to participate in the
  decision pipeline via `tool.checkPermissions()` (e.g., Bash evaluates command
  content for risk). GenCode has `PermissionAwareTool` but it only prepares
  approval UI — it does not feed into the allow/deny decision. Consider adding
  a `CheckPermission(input) Decision` method to the tool interface that the
  decorator calls before the global `PermissionFunc`.

- **Auto-mode classifier**: CC uses an AI classifier to auto-approve/deny in
  auto mode, with denial-limit fallback to interactive prompting. Consider
  adding a similar mechanism for headless and auto-accept scenarios.

- **Headless agent auto-deny**: CC auto-denies `Ask` results for headless agents
  (subagents without TUI). GenCode subagents currently use fixed `perm.Checker`
  policies which sidestep this, but if subagents ever use the full settings
  pipeline they will need explicit headless handling.
