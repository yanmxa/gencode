# Subagent System

## What Is a Subagent

A subagent is a child agent spawned by a parent agent to handle a focused task. The parent delegates work, the child executes independently, and the result flows back. This is the fundamental mechanism for an LLM to decompose complex work into manageable pieces.

```
Parent Agent
  │
  ├─ "I need to search the codebase for all API endpoints"
  │     → spawn Explore subagent
  │     → child searches, returns findings
  │     → parent continues with the information
  │
  ├─ "I need to implement changes in 3 independent files"
  │     → spawn 3 background subagents in parallel
  │     → children work concurrently
  │     → parent notified as each completes
  │
  └─ "This needs careful planning before implementation"
        → spawn Plan subagent
        → child analyzes, returns plan
        → parent reviews and executes
```

The subagent is not a function call — it is a full agent with its own conversation history, its own LLM inference loop, and its own tool execution. The parent constructs intent (the prompt), the child constructs execution.

## Why Subagents

Four problems that subagents solve:

**1. Context isolation.** An LLM's context window is finite and shared. Research that requires reading 50 files pollutes the parent's context with irrelevant detail. A subagent absorbs this cost in its own context, returning only the distilled result.

**2. Parallel execution.** A single agent is sequential — one inference, one tool call at a time. Subagents enable true concurrency: multiple independent tasks running simultaneously, each with its own LLM loop.

**3. Specialization.** Different tasks benefit from different system prompts, tool sets, and behavioral constraints. A code reviewer needs different instructions than a test writer. Subagent types encode these specializations declaratively.

**4. Blast radius control.** A subagent that goes wrong affects only its own context and (optionally isolated) working directory. The parent's conversation and state remain intact.

## Lifecycle

Every subagent follows the same five-phase lifecycle:

```
CONSTRUCT → EXECUTE → RETURN → NOTIFY → CLEANUP

  ┌──────────────────────────────────────────────────────────┐
  │ 1. CONSTRUCT                                             │
  │    - resolve agent type → system prompt + tool set       │
  │    - inherit parent context (instructions, provider)     │
  │    - create core.Agent with its own Inbox/Outbox         │
  │    - optionally create worktree for file isolation       │
  └──────────────────────┬───────────────────────────────────┘
                         │
  ┌──────────────────────▼───────────────────────────────────┐
  │ 2. EXECUTE                                               │
  │    - append user prompt to conversation                  │
  │    - ThinkAct loop: LLM infer → tool exec → repeat      │
  │    - foreground: blocks parent until complete            │
  │    - background: runs in goroutine, parent continues     │
  └──────────────────────┬───────────────────────────────────┘
                         │
  ┌──────────────────────▼───────────────────────────────────┐
  │ 3. RETURN                                                │
  │    - extract final assistant message as result text      │
  │    - foreground: return directly to parent tool call     │
  │    - background: store result, queue notification        │
  └──────────────────────┬───────────────────────────────────┘
                         │
  ┌──────────────────────▼───────────────────────────────────┐
  │ 4. NOTIFY (background only)                              │
  │    - push completion event to parent's notification queue│
  │    - parent's TUI injects notification at next idle      │
  │    - LLM decides whether to act on the result            │
  └──────────────────────┬───────────────────────────────────┘
                         │
  ┌──────────────────────▼───────────────────────────────────┐
  │ 5. CLEANUP                                               │
  │    - save subagent session transcript                    │
  │    - close MCP connections (agent-specific only)         │
  │    - remove worktree if no changes were made             │
  └──────────────────────────────────────────────────────────┘
```

## Foreground vs. Background

Two execution modes, chosen by the parent at spawn time:

| | Foreground (default) | Background (`run_in_background: true`) |
|---|---|---|
| **Parent blocks?** | Yes, until child completes | No, parent continues immediately |
| **Result delivery** | Synchronous return value | Async notification injected at idle |
| **Use case** | Research needed before next step | Independent parallel work |
| **Concurrency** | One at a time per parent | Multiple simultaneously |
| **Permission prompts** | Normal (shown to user) | Suppressed (auto-deny or bubble) |
| **Abort propagation** | Parent abort → child abort | Independent (explicit TaskStop) |

The mental model: foreground = function call, background = goroutine.

### Background Agent Lifecycle Detail

```
Parent LLM response contains Agent tool call
  │
  ├─ run_in_background: true
  │     │
  │     ▼
  │   Register with task manager → taskId
  │   Spawn goroutine → child runs independently
  │   Return taskId to parent immediately
  │   Parent LLM continues with next tool call or response
  │     │
  │     │  ... time passes ...
  │     │
  │     ▼
  │   Child completes → push to notification queue
  │   Parent TUI polls queue at tick interval
  │   At idle: inject notification as user message
  │   Parent LLM processes result, decides next action
  │
  └─ run_in_background: false (default)
        │
        ▼
      Child runs in parent's goroutine (blocking)
      Result returned directly as tool output
      Parent LLM sees result in same turn
```

### Parallel Background Agents (Batching)

When the LLM spawns multiple background agents in a single response (parallel tool calls), they form a **batch**. The system tracks batch progress and delivers coordinated notifications:

```
Parent LLM response:
  ├─ Agent(prompt: "fix auth module",     run_in_background: true) → task-1
  ├─ Agent(prompt: "fix payment module",  run_in_background: true) → task-2
  └─ Agent(prompt: "fix logging module",  run_in_background: true) → task-3

Batch created: {total: 3, completed: 0}

  task-2 completes → {total: 3, completed: 1}
    → notification: "1/3 done, 2 still running — wait"
  task-1 completes → {total: 3, completed: 2}
    → notification: "2/3 done, 1 still running — wait"
  task-3 completes → {total: 3, completed: 3}
    → notification: "all 3 done — finalize results"
```

## Inheritance and Isolation

A subagent must inherit enough to be useful but isolate enough to be safe. The principle: **inherit identity and capability, isolate state and context.**

### What the Child Inherits

| Inherited | Why |
|---|---|
| LLM provider + model | Child needs the same (or specified) LLM to reason |
| User instructions (GEN.md) | User's preferences apply to all agents |
| Project instructions | Project conventions apply to all agents |
| Hook engine | Hooks fire on child's tool calls too |
| MCP connections | Child can use parent's MCP servers |
| Working directory | Child operates in the same repo (unless worktree) |

### What the Child Does NOT Inherit

| Isolated | Why |
|---|---|
| Conversation history | Child starts fresh — parent's context is irrelevant noise |
| File cache | Child reads files independently to avoid stale data |
| Task/todo state | Child has its own task tracking |
| Abort controller | Background children outlive parent turns |

### Worktree Isolation

For agents that modify files, git worktree provides filesystem isolation:

```
Normal:      Parent and child share the same working directory
             Risk: child edits conflict with parent edits

Worktree:    Child gets a temporary git worktree
             .gen/worktrees/<session>/<slug>/
             Child's file operations happen in the copy
             On completion:
               - no changes → worktree auto-removed
               - has changes → worktree path + branch returned to parent
```

## Agent Type System

Agent types are declarative specializations — a combination of system prompt, tool restrictions, and behavioral constraints.

### Definition Sources (priority order)

```
1. Built-in agents     — compiled into the binary, dynamic prompts
2. Project agents      — .gen/agents/*.md in the repo
3. User agents         — ~/.gen/agents/*.md personal definitions
4. Plugin agents       — provided by installed plugins
```

### Agent Definition Schema

Agents are defined as markdown files with YAML frontmatter:

```yaml
---
name: go-reviewer
description: Reviews Go code for correctness and idioms
tools:
  - Read
  - Glob
  - Grep
  - Bash
disallowedTools: []
model: inherit           # use parent's model, or override
permissionMode: plan     # plan | bubble | bypassPermissions
maxTurns: 50             # prevent runaway agents
isolation: worktree      # optional filesystem isolation
---

You are a Go code reviewer. Focus on correctness bugs, race conditions,
and non-idiomatic patterns. Do not suggest style-only changes.

Report findings as a prioritized list with file:line references.
```

### Built-in Agent Types

| Type | Purpose | Tools | Key Behavior |
|---|---|---|---|
| `general-purpose` | Default. Full capability. | All | No restrictions |
| `Explore` | Fast codebase exploration | Read, Glob, Grep, Bash | Read-only, no edits |
| `Plan` | Architecture planning | Read, Glob, Grep, Bash | Read-only, returns plan |
| `code-reviewer` | Code review | Read, Glob, Grep, Bash | Read-only, structured output |

### Agent Selection

The LLM selects agent types via the `subagent_type` parameter. When omitted, defaults to `general-purpose`. The system prompt lists available agents with their `description` and `whenToUse` to guide the LLM's selection.

## Permission Model

Subagents execute tools that affect the real filesystem. The permission model ensures the user retains control.

### Permission Inheritance

```
Parent permission mode
  │
  ├─ bypassPermissions → child inherits (most permissive wins)
  ├─ acceptEdits       → child inherits (unless agent def is more restrictive)
  ├─ default           → agent definition's permissionMode takes effect
  │
  Agent definition permissionMode
  │
  ├─ plan     → child requires plan approval before edits
  ├─ bubble   → permission prompts bubble up to parent's terminal
  └─ (none)   → normal permission flow
```

### Tool Restrictions

Agent definitions can restrict available tools in two ways:

- **Allowlist** (`tools: [Read, Grep, ...]`): child can ONLY use these tools
- **Denylist** (`disallowedTools: [Bash, Write]`): child can use everything EXCEPT these

This is the primary safety mechanism for read-only agents — if an agent's definition only lists read tools, it physically cannot modify files.

### Background Agent Permissions

Background agents cannot show interactive permission prompts (no user to respond). Three strategies:

1. **Auto-deny**: reject any tool call that would require permission (safest)
2. **Bubble**: queue the prompt and surface it in the parent's terminal (interactive)
3. **Pre-authorize**: agent runs with `bypassPermissions` (most autonomous, requires trust)

## Communication

### Parent → Child: The Prompt

The primary communication channel. The parent writes a self-contained prompt that tells the child:
- What to accomplish
- Relevant context the child needs
- What form the result should take

The prompt is the child's only window into the parent's intent. It must be self-contained — the child has no access to the parent's conversation history.

### Child → Parent: The Result

Foreground agents return their final assistant message as the tool result. Background agents store their result and deliver it via the notification system.

### Mid-Flight Communication: SendMessage

For long-running background agents, the parent (or other agents) can send messages to a running child:

```
SendMessage(to: "agent-name", message: "also check the auth module")
  │
  ▼
Message queued in child's pending messages
  │
  ▼
At child's next turn boundary, message injected as user input
  │
  ▼
Child processes the message in its next inference cycle
```

This is a one-way channel. The child cannot send messages back to the parent — it can only complete and return its result.

### Task Observation: TaskOutput

The parent can read a background agent's accumulated output without waiting for completion:

```
TaskOutput(taskId: "xxx")           → read current output
TaskOutput(taskId: "xxx", block: true) → block until new output available
TaskStop(taskId: "xxx")             → gracefully terminate the agent
```

## System Prompt Construction

The subagent's system prompt is layered:

```
┌─────────────────────────────────────────────┐
│ Layer 1: Agent Type Definition              │
│   "You are a Go code reviewer..."           │
│   (from agent definition markdown body)     │
├─────────────────────────────────────────────┤
│ Layer 2: User & Project Instructions        │
│   GEN.md / CLAUDE.md content                │
│   (inherited from parent)                   │
├─────────────────────────────────────────────┤
│ Layer 3: Environment Context                │
│   Git status, working directory, platform   │
│   Available tools (filtered by agent type)  │
├─────────────────────────────────────────────┤
│ Layer 4: Spawner Context                    │
│   Extra context from parent's tool call     │
│   (e.g., "focus on files in src/auth/")     │
└─────────────────────────────────────────────┘
```

Read-only agents (Explore, Plan) can omit heavy context (git status, full instructions) to save tokens. The agent definition controls this via `omitClaudeMd: true`.

## Relationship to TUI Architecture

Subagents integrate with the three-source MVU architecture:

```
Source 2 (Agents) in the TUI
  │
  ├─ Background agent completes
  │     → task manager fires completion callback
  │     → notify.Model queues notification
  │     → TUI tick polls queue
  │     → at idle: inject as user message → sendToAgent()
  │     → parent LLM processes result
  │
  ├─ SendMessage to running agent
  │     → message queued in orchestration store
  │     → child drains at next turn boundary
  │
  └─ Progress updates
        → child reports progress via callback
        → TUI renders progress inline
```

Foreground subagents do not interact with the TUI at all — they run synchronously inside a tool execution, which is already within the agent's ThinkAct loop.

## Design Principles

**1. The prompt is the interface.** The quality of a subagent's work depends entirely on the quality of the parent's prompt. The system should make it easy to write good prompts (agent type descriptions, structured schemas) but cannot compensate for bad ones.

**2. Isolation by default, sharing by opt-in.** Start with maximum isolation (separate context, separate file cache, separate task state). Only share what's explicitly needed (provider, instructions, hooks).

**3. Subagents are not threads.** They don't share memory. They don't have locks. Communication is message-passing only. This makes the system simple to reason about and impossible to deadlock.

**4. Fail independently.** A subagent failure should not crash the parent. The parent receives an error result and decides how to proceed. Background agents that panic are caught and reported as failures.

**5. Agent types are data, not code.** Agent specializations are defined declaratively (markdown + frontmatter), not as Go code. This makes them user-extensible without recompilation.

**6. Background is async, not magic.** Background agents don't have special powers. They're the same agent running in a goroutine instead of inline. The only differences are: non-blocking return, notification-based result delivery, and restricted permission prompts.
