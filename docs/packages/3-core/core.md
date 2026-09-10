---
package: github.com/genai-io/san/internal/core
layer: core
---

# core

The agent primitive: the `Agent` interface, its surrounding `System` /
`Tools` / `LLM` contracts, and the message/event types they exchange. Agent
runtime and TUI view state live outside this package.

## Purpose

Everything in `feature` and above depends on this package; nothing here
depends on anything outside `internal/log`, `context`, and stdlib. Keeping
the surface small and stable is the whole point.

This is also the only package that gets multiple interfaces on one page —
`Agent`, `System`, `Tools`, `Tool`, `LLM` are the system's primitives and
move together when they move at all.

## Contract

### Agent

```go
package core

// Agent — an LLM in a loop. Three capabilities: System (WHO), Tools (WHAT),
// Inbox/Outbox (HOW it communicates).
type Agent interface {
    ID() string
    System() System
    Tools() Tools
    Inbox() chan<- Message    // caller owns and closes
    Outbox() <-chan Event     // agent owns and closes on Run() return
    Messages() []Message
    SetMessages(msgs []Message)
    Append(ctx context.Context, msg Message)
    ThinkAct(ctx context.Context) (*Result, error)
    Run(ctx context.Context) error
}

```

### System

```go
// System — the composable, mutable system prompt.
type System interface {
    Prompt() string
    Use(sec Section, caller string)
    Drop(name, caller string)
    Refresh(name, caller string)
    Sections() []Section
    SetObserver(fn func(SystemChange))
}
```

### Tools

```go
// Tool — one capability the agent can execute. Pure (no hooks, no permissions).
type Tool interface {
    Name() string
    Description() string
    Schema() ToolSchema
    Execute(ctx context.Context, input map[string]any) (string, error)
}

// Tools — mutable collection of Tool.
type Tools interface {
    Get(name string) Tool
    All() []Tool
    Add(tool Tool, caller string)
    Remove(name, caller string)
    Schemas() []ToolSchema
    SetObserver(fn func(ToolsChange))
}
```

### LLM

```go
// LLM — inference. Streams Chunk; final chunk carries the aggregated InferResponse.
type LLM interface {
    Infer(ctx context.Context, req InferRequest) (<-chan Chunk, error)
    InputLimit() int
}
```

### Known Violations

The contracts here are mostly clean (this package is the *design intent*),
but a few items deserve flagging:

- **Rule 1 (small) — `Agent` has 8 methods.** Borderline. The methods
  cluster into identity (`ID`/`System`/`Tools`), I/O (`Inbox`/`Outbox`),
  state (`Messages`/`SetMessages`/`Append`), and execution
  (`ThinkAct`/`Run`). A clean split would yield `AgentIdentity`,
  `AgentIO`, `AgentMessages`, `AgentRunner` — but `Agent` is the central
  primitive and downstream code treats it as one cohesive value. Document
  the trade-off; don't split.
- **Rule 1 — `System` has 6 methods, `Tools` has 6.** Same trade-off:
  observer + mutation + query on one type. Acceptable for now.

`Tool` (4 methods) and `LLM` (2 methods) are model-citizen interfaces and
need no changes.

## Internals

The `core.Agent` implementation lives in `internal/agent/runtime`. TUI-only
`ChatMessage` state lives in `internal/app/conv`. The root package retains the
small default `System` and `Tools` collections because they directly implement
the shared mutation/observer contracts; prompt catalog and assembly live in
`internal/core/system/`, while tool execution adapters live in `internal/tool/`.

## Lifecycle

`runtime.New` panics if `LLM`, `System`, or `Tools` is nil. After
construction, callers own the `Inbox` channel (must close when done
sending) and read the `Outbox` until it closes (agent owns it).

`Run` returns when the context is cancelled or a `SigStop` message is
received. After `Run` returns, sending to the inbox blocks indefinitely.

## Tests

```
internal/core/message_test.go            — message and compaction values.
internal/core/retry_test.go              — shared retry/backoff policy.
internal/agent/runtime/agent_test.go     — agent loop behavior and signals.
internal/agent/runtime/retry_test.go     — runtime stream retry behavior.
```

## See Also

- Code: `internal/core/`
- Consumer: [`packages/agent.md`](../2-feature/agent.md) (`internal/agent` wraps `core.Agent`)
- Subsystem implementations: [`packages/tool.md`](../2-feature/tool.md), [`packages/llm.md`](../2-feature/llm.md), [`packages/subagent.md`](../2-feature/subagent.md)
- Layer: `core` (see [`reference/dependency-rules.md`](../../reference/dependency-rules.md))
