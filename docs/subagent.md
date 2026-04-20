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
              (synchronous)│      │  (async notify)
              ▼                      ▼
     ┌────────────────┐    ┌────────────────┐
     │  Subagent A    │    │  Subagent B    │ goroutine
     │  Blocks parent │    │  Runs alone    │
     │  Result = tool │    │  Done → notify │
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
│     ├─ agent.ThinkAct(ctx)          ← Subagent's LLM loop
│     │     ├─ LLM → Grep → LLM → Read → LLM → end_turn
│     │     └─ Returns Result{Content: "Found 12 endpoints..."}
│     └─ Return Content as tool result
│
├─ Main Agent receives tool result: "Found 12 endpoints..."
├─ LLM continues next reasoning step
```

## Pattern 2: Background — Async Notification

The Subagent runs in a separate goroutine. The Main Agent immediately receives a task ID and continues working. When the Subagent completes, it **pushes a notification** that becomes a message in the Main Agent's conversation.

### Phase 1: Launch

The Main Agent's LLM calls `Agent` with `run_in_background: true`:

```
Main Agent LLM loop
│
├─ LLM inference → "These three tasks can run in parallel"
│
├─ Parallel tool calls:
│   Agent(prompt: "fix auth module",    run_in_background: true)
│   Agent(prompt: "fix payment module", run_in_background: true)
│   Agent(prompt: "fix logging module", run_in_background: true)
│
│   Each call internally:
│     Executor.RunBackground()
│       ├─ Build core.Agent
│       ├─ Register AgentTask → get taskID
│       ├─ Spawn goroutine → Subagent runs independently
│       └─ Return tool result immediately (don't wait for completion)
```

The Main Agent receives one tool result per `Agent` call:

```
Agent started in background.
Task ID: task-abc123
Agent: Explore
Description: fix auth module
```

After receiving all three tool results, the LLM replies: "I launched 3 agents..." then `end_turn`, entering idle state.

### Phase 2: Subagent Execution

The Subagent runs its own LLM loop inside the goroutine, fully independent from the Main Agent:

```
goroutine:
  Subagent.ThinkAct(ctx)
    ├─ LLM inference → Read(src/auth/validate.go)
    ├─ Execute Read
    ├─ LLM inference → Edit(src/auth/validate.go, ...)
    ├─ Execute Edit
    ├─ LLM inference → end_turn
    └─ Returns Result{Content: "Fixed null pointer in validate.go:42"}
```

### Phase 3: Completion → Push Notification

When the Subagent finishes, it **pushes a notification to the queue** — the Main Agent does not poll for results:

```
Subagent goroutine ends
     │
     ▼
AgentTask.Complete(result)
     │
     │  Triggers completion callback registered at launch
     ▼
Build Notification {
    Notice: "fix auth module completed"      ← For TUI display
    Prompt: "<task-notification>...</>"       ← For LLM reasoning
}
     │
     ▼
NotificationQueue.Push(notification)         ← Thread-safe queue
```

The Notification has only two fields. `Prompt` is minimal XML carrying just enough for the LLM to decide next steps:

```xml
<task-notification task-id="task-abc123" status="completed" agent-id="session-789" description="fix auth module">
Fixed null pointer in validate.go:42. Added nil check before user.ID access.
</task-notification>
```

All metadata as attributes (task-id, status, agent-id, description), result content as body text. Compact and complete.

On failure:

```xml
<task-notification task-id="task-ghi789" status="failed" description="fix logging module">
Could not find logging config at expected path /etc/app/logging.yaml
</task-notification>
```

### Phase 4: Injection into Main Agent's Conversation

The TUI checks the queue every 500ms. When the Main Agent is idle (not inferring, not executing tools), it pops notifications and converts them into **two messages** — one for the user, one for the LLM:

```
NotificationQueue
     │
     │  Main Agent idle
     ▼
┌────────────────────────────────────────────────┐
│                                                │
│  Message 1: RoleNotice                         │
│  "fix auth module completed"                   │
│  → Rendered in TUI for the user                │
│  → Not sent to LLM conversation               │
│                                                │
│  Message 2: RoleUser                           │
│  <task-notification task-id="task-abc123"      │
│    status="completed" agent-id="session-789"   │
│    description="fix auth module">              │
│  Fixed null pointer in validate.go:42...       │
│  </task-notification>                          │
│  → sendToAgent() → Agent Inbox                 │
│  → Triggers new LLM reasoning cycle            │
│                                                │
└────────────────────────────────────────────────┘
```

The Main Agent's LLM sees this user message like any regular input — it synthesizes results and decides next steps on its own.

### Multiple Notifications Arriving Together

If multiple Subagents complete while the Main Agent is busy, notifications queue up. When the Main Agent goes idle, it pops up to 8 notifications at once and merges them into a single message:

```xml
<task-notifications count="3">
<task-notification task-id="task-abc123" status="completed" agent-id="session-1" description="fix auth module">
Fixed auth module...
</task-notification>
<task-notification task-id="task-def456" status="completed" agent-id="session-2" description="fix payment module">
Fixed payment module...
</task-notification>
<task-notification task-id="task-ghi789" status="failed" description="fix logging module">
Could not find logging config...
</task-notification>
</task-notifications>
```

## Full Sequence

```
User: "Fix bugs in auth, payment, and logging modules"
  │
  ▼
Main Agent LLM inference
  │
  ├─ Agent(prompt:"fix auth",    bg:true) → "Task ID: task-abc123"
  ├─ Agent(prompt:"fix payment", bg:true) → "Task ID: task-def456"
  ├─ Agent(prompt:"fix logging", bg:true) → "Task ID: task-ghi789"
  │
  ├─ LLM: "I launched 3 background agents..."
  ├─ end_turn → idle
  │
  │              task-abc123        task-def456        task-ghi789
  │              goroutine          goroutine          goroutine
  │                 │                  │                  │
  │                 ▼                  │                  │
  │            Done → Push notify      │                  │
  │                                    ▼                  │
  │                               Done → Push notify      │
  │                                                       ▼
  │                                                  Failed → Push notify
  │
  │  Queue: [notif-1, notif-2, notif-3]
  │
  │  Main Agent idle → Pop all → Convert to two messages:
  │
  │    RoleNotice: "3 background tasks completed: ..."     → User sees
  │    RoleUser:   <task-notifications count="3">...</>    → LLM sees
  │
  ▼
Main Agent LLM inference:
  "task-abc123 and task-def456 succeeded, task-ghi789 failed. Let me check..."
  → May spawn new agent to retry
  → May reply directly to user
  → Entirely up to the LLM
```

## Lifecycle

```
CONSTRUCT → EXECUTE → RETURN → (NOTIFY) → CLEANUP
```

- **CONSTRUCT**: Resolve agent type → Build system prompt + tool set → Create core.Agent → Optional git worktree isolation
- **EXECUTE**: Inject prompt → ThinkAct loop (LLM inference → tool execution → repeat until end_turn)
- **RETURN**: Foreground → return tool result directly; Background → store in AgentTask
- **NOTIFY** (background only): Complete → Build Notification → Push queue → Inject as message when idle
- **CLEANUP**: Save session transcript → Close MCP connections → Delete worktree if no changes

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
| `SendMessage` | Send a message to a completed agent (resumes execution) |
| `TaskOutput` | Read output from a background task |
| `TaskStop` | Stop a running background task |

## Design Principles

1. **Prompt is the interface.** Subagents cannot see the parent's conversation — the prompt must be self-contained.
2. **Completed work becomes a message.** Background results are injected as a user message — no special protocol, no shared state, just a message in the conversation.
3. **Subagents push, Main Agent doesn't poll.** Subagents push to the queue on completion; the Main Agent receives when idle. Push, not pull.
4. **Isolated by default.** Independent context, file cache, and task state. Only share what's necessary (provider, instructions, hooks).
5. **Fail independently.** A Subagent failure produces an error result — it doesn't crash the parent. The parent decides how to handle it.
6. **Agent types are data, not code.** Markdown + frontmatter definitions — extensible without recompilation.
