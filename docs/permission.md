# Permission

Permission is a tool-execution concern. All permission types, decisions, and enforcement
live under `internal/tool/`.

## Package Layout

```
internal/tool/
│
├── perm/                        ← consolidated permission package
│   ├── decision.go                 Decision, Checker, built-in checkers
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

`tool/perm/` holds all permission primitives: decision types, checkers, safe-tool
lists, approval-dialog metadata, and diff computation.

`tool/permission.go` provides the `WithPermission` decorator that wires a
`PermissionFunc` into `core.Tools`.

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
// nil check returns inner unchanged (permit-all).
func WithPermission(inner core.Tools, check PermissionFunc) core.Tools
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
│    1. allow, reason := check(ctx, "Bash", input)         │
│    2. if !allow → return error("blocked: " + reason)     │
│    3. return inner.Execute(ctx, input)                    │
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
     │ ├─ check(ctx, name, input)           │
     │ │   IsSafeTool?  ─── yes ──► allow   │
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
  check          check           check
  │              │               │
  IsSafeTool     Prompt          Prompt
  → allow        │               │
  │              send ──────►    blocked (chan full)
  Execute()      block           │
  done           │               │
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

```go
// app/output/permission_bridge.go

func (pb *PermissionBridge) PermissionFunc() tool.PermissionFunc {
    return func(ctx context.Context, name string, input map[string]any) (bool, string) {
        if perm.IsSafeTool(name) {
            return true, ""
        }
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

func adaptPermission(checker perm.Checker) tool.PermissionFunc {
    if checker == nil {
        return nil
    }
    return func(ctx context.Context, name string, input map[string]any) (bool, string) {
        if perm.IsSafeTool(name) {
            return true, ""
        }
        if checker.Check(name, input) == perm.Reject {
            return false, fmt.Sprintf("tool %s is not permitted in this mode", name)
        }
        return true, ""
    }
}
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
