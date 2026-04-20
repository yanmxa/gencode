# Subagent System

## Core Concept

The Main Agent runs in the TUI, driving an LLM loop:

```
User input → LLM inference → Tool execution → LLM inference → ... → end_turn → Wait for next input
```

The Main Agent can spawn Subagents via the `Agent` tool to delegate work. Each Subagent is a full LLM loop with its own conversation, tool set, and system prompt.

There are exactly two interaction patterns:

```
                        ┌─────────────┐
                        │    User     │
                        └──────┬──────┘
                               │ input
                               ▼
                        ┌─────────────┐
                        │ Main Agent  │
                        │  LLM loop   │
                        └──┬──────┬───┘
                           │      │
              foreground   │      │  background
              (synchronous)│      │  (async)
              ▼                      ▼
     ┌────────────────┐    ┌────────────────┐
     │  Subagent A    │    │  Subagent B    │ goroutine
     │  Blocks parent │    │  Runs alone    │
     │  Result = tool │    │  Done → EventHub│
     └────────────────┘    └────────────────┘
```

## Pattern 1: Foreground — Synchronous Call

The Subagent runs inside the Main Agent's tool execution. The Main Agent blocks until the Subagent finishes, and the result is returned directly as the `Agent` tool's output. No different from calling `Read` or `Grep`.

```
Main Agent LLM loop
│
├─ LLM inference → "Need to search the codebase"
│
├─ Tool call: Agent(prompt: "find all API endpoints")
│     │
│     ▼
│   Executor.Run()
│     ├─ Build core.Agent (system prompt + tool set)
│     ├─ agent.ThinkAct(ctx)
│     │     ├─ LLM → Grep → LLM → Read → LLM → end_turn
│     │     └─ Returns Result{Content: "Found 12 endpoints..."}
│     └─ Return Content as tool result
│
├─ Main Agent receives tool result: "Found 12 endpoints..."
├─ LLM continues next reasoning step
```

## Pattern 2: Background — EventHub

All inter-agent communication flows through a central **EventHub**. Producers call `hub.Publish(event)`. The EventHub calls the subscriber's delivery function. That's it.

Task completion is just one event type. SendMessage, cron, hooks — all the same `Publish()` call.

### Event

Inspired by [CloudEvents](https://github.com/cloudevents/spec):

```go
type Event struct {
    Type    string    // "task.completed", "agent.message", "cron.fired"
    Source  string    // producer: "agent:<id>", "system:cron"
    Target  string    // consumer: "agent:<id>", "main"
    Subject string    // human-readable: "fix auth module completed"
    Data    string    // payload: XML content, message text, etc.
    Time    time.Time
}
```

### EventHub

The EventHub is `map[string]func(Event)` — subscribers register a delivery function, not a channel. Each consumer decides HOW to receive.

```go
package hub

type EventHub struct {
    mu   sync.RWMutex
    subs map[string]func(Event)
}

func New() *EventHub
func (h *EventHub) Register(id string, deliver func(Event))
func (h *EventHub) Unregister(id string)
func (h *EventHub) Publish(e Event)    // calls subs[e.Target](e)
```

Implementation:

```go
func (h *EventHub) Publish(e Event) {
    if e.Time.IsZero() {
        e.Time = time.Now()
    }
    h.mu.RLock()
    deliver, ok := h.subs[e.Target]
    h.mu.RUnlock()
    if ok {
        deliver(e)
    }
}
```

~20 lines total. Point-to-point routing. No channels, no goroutines inside the EventHub.

### Register

Each consumer registers a delivery function. The function captures whatever the consumer needs — inbox, program, hooks — via closure. No intermediate types.

**Main agent — deliver via `program.Send()`:**

```go
hub.Register("main", func(e Event) {
    program.Send(e)    // Bubble Tea's native mechanism → arrives in Update loop
})
```

`program.Send()` is goroutine-safe and non-blocking. The event arrives as a `tea.Msg` in the Update loop:

```go
// In TUI Update:
case hub.Event:
    tracker.Done(e.Source)
    injectNotification(e)
```

No intermediary channel. No `tea.Cmd` blocking pattern. Bubble Tea already solves this.

**Background agent — deliver directly to existing inbox:**

```go
hub.Register("agent:"+taskID, func(e Event) {
    agent.Inbox() <- core.Message{Role: core.RoleUser, Content: e.Data}
})
```

No bridge goroutine. No second channel. The agent already has a buffered inbox (16) — just write to it. The agent reads at the next turn boundary via `drainInbox()`.

**Unregister — on completion or kill:**

```go
hub.Unregister("agent:" + taskID)    // just delete(map, id)
```

No channel to close, no goroutine to stop. Just remove the function from the map.

### Publish

Any code that calls `hub.Publish()` is a producer. No observer structs — just closures:

```go
// ── At app startup ──

task.OnCompleted(func(info task.TaskInfo) {
    hooks.ExecuteAsync(hook.TaskCompleted, hookInput(info))
    hub.Publish(Event{
        Type:    "task.completed",
        Source:  "agent:" + info.ID,
        Target:  "main",
        Subject: formatSubject(info),
        Data:    formatXML(info),
    })
})

// ── SendMessage tool ──

hub.Publish(Event{
    Type:   "agent.message",
    Source: "agent:" + senderID,
    Target: "agent:" + recipientID,
    Data:   content,
})

// ── Cron scheduler ──

hub.Publish(Event{
    Type:   "cron.fired",
    Source: "system:cron",
    Target: "main",
    Data:   prompt,
})
```

### Architecture

```
 Producers                          Hub                         Consumers
 ─────────                     ──────────────                   ─────────

 task.OnCompleted(func{        ┌────────────┐
   hub.Publish(Event{          │            │    Register("main", func(e) {
     Target: "main"        ───►│            ├──►   program.Send(e)    ← TUI Update loop
   })                          │            │    })
 })                            │            │      ├─ Tracker.Done(e.Source)
                                │    Hub     │      ├─ Show e.Subject
 SendMessage tool:              │            │      └─ Inbox ← e.Data
   hub.Publish(Event{          │ map[string] │
     Target: "agent:B"    ───►│  func(Event)│    Register("agent:B", func(e) {
   })                          │            ├──►   agent.Inbox() <- msg  ← direct write
                                │            │    })
 Cron scheduler:                │            │
   hub.Publish(Event{          │            │
     Target: "main"        ───►│            │
   })                          └────────────┘
```

No channels inside the EventHub. No bridge goroutines. Each delivery function writes to whatever the consumer already has.

### Background Agent Flow

```
 Agent Tool                 Background Goroutine                TUI Update Loop
 ──────────                 ────────────────────                ───────────────

 Agent(bg:true)
 ├─ Build subagent
 ├─ hub.Register("agent:A",
 │    func(e) { agent.Inbox() <- ... })
 ├─ Spawn goroutine ──────► ThinkAct()
 ├─ Return taskID             ├─ LLM → tools → ...
 │                            └─ end_turn
 │  TUI Update:                     │
 │  Tracker.Add(id, ◷)            │
 │                                  ├─ hub.Publish(Event{
 │  Main Agent                      │     Type:   task.completed
 │  continues working               │     Source: agent:A
 │                                  │     Target: main
 │                                  │     ...
 │                                  │   })
 │                                  └─ hub.Unregister("agent:A")
 │
 │                                  EventHub calls subs["main"](e)
 │                                    = program.Send(e)
 │
 │                                                          Update loop receives:
 │                                                          ├─ Tracker.Done("agent:A")
 │                                                          ├─ Notice: e.Subject
 │                                                          └─ Inbox ← e.Data
 │
 │  Main Agent next turn:
 │  drainInbox → sees <task-notification> → LLM reasons about result
```

### Agent-to-Agent Communication

Same `Publish()` call, different target:

```
 Agent A                              Hub                        Agent B
 ───────                         ──────────                      ───────

 SendMessage(to: "agent:B",
   content: "check tests")
   │
   └─ hub.Publish(Event{
        Type:   agent.message
        Source: agent:A               EventHub calls subs["agent:B"](e)
        Target: agent:B          ───►   = agent.Inbox() <- msg   ← direct write
        Data:   "check tests"
      })                                                          drainInbox:
                                                                  sees "check tests"
                                                                  LLM reasons about it
```

### Tracker

Pure TUI display state. Only mutated in the Update loop:

```
On launch:      Tracker.Add(id, subject, owner)     → show ◷ spinner
On hub event:   Tracker.Done(e.Source, status)       → show ✓ or ✗
```

No goroutine touches the Tracker. No races.

### Batching

When the TUI receives multiple events in rapid succession (e.g., concurrent agent completions), the Update loop batches them naturally:

```go
// In TUI Update:
case hub.Event:
    events := []hub.Event{e}
    // Check if more events are queued (Bubble Tea processes all pending msgs)
    return m, tea.Batch(
        m.injectNotification(mergeEvents(events)),
    )
```

Or the delivery function can buffer before calling `program.Send()`:

```go
hub.Register("main", func(e Event) {
    batcher.Add(e)               // accumulate
    batcher.FlushAfter(10*ms, func(batch []Event) {
        program.Send(EventBatch(batch))
    })
})
```

Concurrent completions naturally merge. No tick, no polling.

### Two-Message Injection

Each event delivery becomes two messages in the TUI conversation:

```
RoleNotice:  e.Subject → user sees ("fix auth completed")
RoleUser:    e.Data    → LLM sees (<task-notification ...>)
```

## Lifecycle

```
CONSTRUCT → EXECUTE → RETURN → (HUB) → CLEANUP
```

- **CONSTRUCT**: Resolve agent type → Build system prompt + tool set → Create core.Agent → Optional git worktree isolation
- **EXECUTE**: Inject prompt → ThinkAct loop (LLM inference → tool execution → repeat until end_turn)
- **RETURN**: Foreground → return tool result directly; Background → store in AgentTask
- **HUB** (background only): Complete → `hub.Publish()` → delivery function → target agent
- **CLEANUP**: `hub.Unregister()` → Save session → Close MCP → Delete worktree if no changes

## What Subagents Inherit

Principle: **Inherit identity and capabilities, isolate state and context.**

| Inherited | Reason |
|---|---|
| LLM provider + model | Needs the same (or specified) LLM for inference |
| User instructions (GEN.md) | User preferences apply to all agents |
| Project instructions | Project conventions apply to all agents |
| Hook engine | Subagent tool calls must also trigger hooks |
| MCP connections | Subagent can use parent's MCP services |
| Working directory | Same repository (unless worktree-isolated) |

| Isolated | Reason |
|---|---|
| Conversation history | Subagent starts fresh; parent context is noise |
| File cache | Independent reads avoid stale data |
| Task state | Independent task tracking |

## Agent Types

Agent types are declarative specialization definitions — markdown files with YAML frontmatter:

```yaml
---
name: go-reviewer
description: Reviews Go code for correctness and idioms
tools: [Read, Glob, Grep, Bash]
model: inherit
permissionMode: plan
maxTurns: 50
---

You are a Go code reviewer. Focus on correctness bugs...
```

Source priority: Built-in → Project `.gen/agents/*.md` → User `~/.gen/agents/*.md` → Plugin agents

## Related Tools

| Tool | Purpose |
|------|---------|
| `Agent` | Spawn a subagent (foreground or background) |
| `ContinueAgent` | Resume a completed agent from its saved session |
| `SendMessage` | Send a message to a running/completed agent |
| `TaskOutput` | Read output from a background task |
| `TaskStop` | Stop a running background task |

## Design Principles

1. **Prompt is the interface.** Subagents cannot see the parent's conversation — the prompt must be self-contained.
2. **Completed work becomes a message.** Background results are injected as a user message — no special protocol, no shared state, just a message in the conversation.
3. **EventHub = `map[string]func(Event)`.** ~20 lines. No channels, no goroutines inside. Each consumer registers a delivery function — `program.Send()` for the TUI, `agent.Inbox()` for background agents.
4. **No intermediate types.** No observer structs, no bridge goroutines, no intermediary channels. Closures capture what they need. Producers call `hub.Publish()` directly.
5. **All TUI mutations in Update.** `program.Send()` delivers events to the Update loop natively. Tracker updates, notices, inbox sends — all happen there.
6. **Isolated by default.** Independent context, file cache, and task state. Only share what's necessary (provider, instructions, hooks).
7. **Fail independently.** A Subagent failure produces an error result — it doesn't crash the parent. The parent decides how to handle it.
8. **Agent types are data, not code.** Markdown + frontmatter definitions — extensible without recompilation.
