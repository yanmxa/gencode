# Agents and Coordination

## The Agent Primitive

An agent is an LLM in a loop: observe → reason → act → observe result → repeat.

```
┌──────────────────────────────────────────────────────────────────────┐
│                          Agent                                       │
│                                                                      │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                           │
│  │   LLM    │  │  System   │  │  Tools   │                           │
│  │ reasoning│  │  Prompt   │  │ actions  │                           │
│  │ planning │  │ identity  │  │ effects  │                           │
│  │ deciding │  │ context   │  │ feedback │                           │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                           │
│       └──────────────┴─────────────┘                                 │
│                      │                                               │
│              ┌───────┴───────┐                                       │
│              │   ThinkAct    │  ◄── atomic operation                  │
│              │               │                                       │
│              │  LLM stream   │                                       │
│              │      │        │                                       │
│              │  tool calls?  │                                       │
│              │  ├─ yes ──► execute tools ──► loop back               │
│              │  └─ no  ──► return Result (end of turn)               │
│              └───────────────┘                                       │
│                                                                      │
│  Two execution modes:                                                │
│  ┌──────────────────────┐    ┌──────────────────────┐                │
│  │  Run() — goroutine   │    │  Direct — sync call  │                │
│  │  wait(inbox)         │    │  agent.ThinkAct(ctx)  │                │
│  │  ThinkAct            │    │  returns Result       │                │
│  │  emit to outbox      │    │                       │                │
│  │  wait...             │    │  Used by: subagents   │                │
│  │  Used by: TUI agent  │    │           workers     │                │
│  └──────────────────────┘    └──────────────────────┘                │
│                                                                      │
│  Channels (Run mode only):                                           │
│       Inbox ──────────► Agent Loop ──────────► Outbox                │
│    (chan Message)        WAIT→DRAIN→ThinkAct    (chan Event)          │
│    user, agents,        ▲            │         PreInfer, OnChunk,    │
│    system events        └────────────┘         PostInfer, PreTool,   │
│                                                PostTool, OnTurn,     │
│                                                OnStop                │
└──────────────────────────────────────────────────────────────────────┘

core.Agent has NO dependency on hooks, permissions, or UI.
Those are layered on by the application. See core.md.
```

**Context is the scarce resource.** Everything the agent knows — identity,
instructions, history, tool results — competes for the same finite window.
This constraint is the fundamental reason multi-agent systems exist.

## Why Multiple Agents

Single agent is the right default. Multi-agent adds 3-10x token cost.
Justified in exactly three scenarios:

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│  ① Context Protection         ② Parallelization                    │
│                                                                     │
│  research agent retrieves     4 research agents explore             │
│  1000s of tokens of code      different areas simultaneously        │
│        │                              │                             │
│        ▼                              ▼                             │
│  subagent isolates →           wall-clock time drops                │
│  parent gets 200-token         proportionally                      │
│  distilled summary                                                  │
│                                                                     │
│  ③ Specialization              ✗ When NOT to split                 │
│                                                                     │
│  15-20+ tools degrade          · simple, well-defined task          │
│  selection accuracy             · sequential (each step depends     │
│        │                          on previous)                      │
│        ▼                        · unified context requirements      │
│  5 targeted tools per           · manageable tool count             │
│  specialized agent                                                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

Decompose by CONTEXT requirements, not by problem type.
  ✗ "features agent" + "tests agent" → telephone game
  ✓ "component A agent" + "component B agent" → context coherence
```

## Coordination Patterns

Five patterns, ordered by complexity. Start with the simplest that fits.

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│  ① Orchestrator-Subagent          ② Generator-Verifier              │
│  (GenCode's primary pattern)                                        │
│                                                                     │
│       ┌──────────────┐            ┌───────────┐     ┌───────────┐  │
│       │ Orchestrator  │            │ Generator │────►│ Verifier  │  │
│       └──┬────┬────┬──┘            │           │◄────│           │  │
│          │    │    │               └───────────┘     └───────────┘  │
│          ▼    ▼    ▼                   output    ◄── feedback/pass  │
│        ┌──┐ ┌──┐ ┌──┐                                              │
│        │S1│ │S2│ │S3│             Quality-critical output with      │
│        └──┘ └──┘ └──┘             measurable criteria.              │
│                                   Self-eval consistently fails;     │
│  Clear decomposition with         separate verifier succeeds.       │
│  bounded subtasks.                                                  │
│  Orchestrator = info bottleneck.                                    │
│                                                                     │
│  ③ Agent Teams                    ④ Message Bus                     │
│                                                                     │
│       ┌──────────────┐            ┌────────────────────────┐       │
│       │ Coordinator   │            │      Message Router    │       │
│       └──┬────┬────┬──┘            └──┬────┬────┬────┬──┘          │
│          │    │    │                  │    │    │    │              │
│          ▼    ▼    ▼                  ▼    ▼    ▼    ▼              │
│        ┌──┐ ┌──┐ ┌──┐             ┌──┐ ┌──┐ ┌──┐ ┌──┐            │
│        │W1│ │W2│ │W3│             │A1│ │A2│ │A3│ │A4│            │
│        └──┘ └──┘ └──┘             └──┘ └──┘ └──┘ └──┘            │
│     persistent workers,           pub-sub, event-driven,           │
│     accumulate context            new agents without rewiring       │
│                                                                     │
│  ⑤ Shared State                                                    │
│                                                                     │
│     ┌──┐  ┌──┐  ┌──┐                                              │
│     │A1│  │A2│  │A3│  autonomous agents                            │
│     └─┬┘  └─┬┘  └─┬┘                                              │
│       │     │     │                                                │
│       ▼     ▼     ▼                                                │
│     ┌─────────────────┐                                            │
│     │  Shared Store   │  no central coordinator                    │
│     └─────────────────┘  needs explicit termination                │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘

Evolution:
  Start ──► Orchestrator-Subagent
              │
              ├─ subtasks need sustained context? ──► Agent Teams
              ├─ event-driven workflow?            ──► Message Bus
              └─ real-time shared discoveries?     ──► Shared State
```

## Agent Communication

Three models, each for a different coupling level:

```
① Return Value (foreground)       ② Notification (background)

Parent          Subagent          Parent               Worker
  │                │                │                     │
  ├─ Agent(prompt) │                ├─ Agent(prompt, bg)  │
  │ ──────────────►│                │ ───────────────────►│
  │    (blocks)    ├─ ThinkAct      │  (continues)       ├─ ThinkAct
  │                ├─ tools...      │                     ├─ tools...
  │                ├─ Result        ├─ other work         ├─ completes
  │ ◄──────────────┤                │                     │
  ├─ result in     │                │     notification ◄──┘
  │  tool output   │                │     (queued)
  ▼                ▼                │  (idle)
                                    ├─ receives as user message
Simple, blocks parent.              ▼
Use: quick bounded tasks.           Non-blocking, parallel.
                                    Use: research, implementation.


③ Message Passing (SendMessage)

Parent                          Worker
  │                               │
  ├── Agent(prompt, background)   │
  │ ─────────────────────────────►│
  │                               ├── ThinkAct (turn 1)
  │                               │
  │  SendMessage("check errors")  │
  │ ─────────────────────────────►│  (queued)
  │                               │
  │                               ├── receives at turn boundary
  │                               ├── ThinkAct (turn 2)
  │                               ├── completes
  │     notification ◄────────────┘
  ▼
Bidirectional, mid-flight steering.
Use: failure recovery, follow-up instructions.
```

**Anti-patterns:** polling TaskOutput (use notifications), shared mutable
files (use worktrees), "pass findings to worker B" (coordinator synthesizes),
"continue where you left off" (prompts must be self-contained).

## The Coordinator

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Coordinator                                   │
│                                                                      │
│  The main agent, same LLM, guided by system prompt to orchestrate.   │
│  Not a separate runtime, scheduler, or state machine.                │
│  The intelligence is in the prompt. Infrastructure is plumbing.      │
│                                                                      │
│  Activates naturally when LLM decides task benefits from             │
│  decomposition. No toggle — a continuum, not a binary switch.        │
│                                                                      │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │                    Workflow Phases                              │  │
│  │                                                                │  │
│  │   ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐     │  │
│  │   │ Research  │  │Synthesis │  │Implement │  │ Verify   │     │  │
│  │   │(parallel) │─►│(coordin.)│─►│(parallel)│─►│(parallel)│     │  │
│  │   │          │  │          │  │          │  │          │     │  │
│  │   │ N workers│  │ NO       │  │ workers  │  │ skeptical│     │  │
│  │   │ explore  │  │ workers  │  │ with     │  │ verifiers│     │  │
│  │   │ in //    │  │          │  │ concrete │  │ run code │     │  │
│  │   │          │  │ read all │  │ specs    │  │ not read │     │  │
│  │   │          │  │ results  │  │          │  │          │     │  │
│  │   │          │  │ plan     │  │ worktree │  │ prove it │     │  │
│  │   │          │  │ decompose│  │ if files │  │ works    │     │  │
│  │   │          │  │          │  │ overlap  │  │          │     │  │
│  │   └──────────┘  └──────────┘  └──────────┘  └──────────┘     │  │
│  │                      ▲                                        │  │
│  │                      │                                        │  │
│  │          THE CRITICAL STEP: coordinator understands,           │  │
│  │          not just forwards. This is where synthesis happens.   │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  Tools:                                                              │
│    Agent(background)  spawn worker       TaskOutput(id) read output  │
│    Agent(foreground)  spawn subagent     TaskStop(id)   cancel       │
│    SendMessage(to)    continue worker                                │
│                                                                      │
│  Cardinal rule: coordinator owns understanding, workers own          │
│  execution. Never "based on your findings, fix it."                  │
│                                                                      │
│  Continue vs. Respawn:                                               │
│    Continue ── worker explored relevant files / fixable error        │
│    Respawn  ── needs narrower context / independent verification     │
│               / stuck on wrong approach / unrelated task             │
└─────────────────────────────────────────────────────────────────────┘
```

### Prompt Writing

Workers cannot see the coordinator's conversation. Every prompt must be
self-contained: what to do, why, file paths, constraints, output format.

For continuation (SendMessage): state what specifically failed, what to
change, and why — don't say "try again."

## Notification & Batching

```
Worker completes
    │
    ├── LifecycleHandler (wireTaskLifecycle closure)
    │     ├─ fire TaskCompleted hook (async)
    │     ├─ tracker.CompleteWorker (update tracker)
    │     └─ eventHub.Publish → m.events <- e (consumer channel)
    │
    ▼
m.events ──── drainTurnQueues (at turn boundary) ──── inject as user msg
                                                            │
                                                            ▼
                                                    Coordinator reacts
```

### Notification Format

```xml
<task-notification>
  <task-id>worker-1</task-id>
  <agent-name>research-api</agent-name>
  <status>completed</status>           <!-- completed | failed | killed -->
  <summary>Found 3 relevant endpoints</summary>
  <result>... worker output ...</result>
  <usage>tokens: 12400, tools: 8, duration: 34s</usage>
  <batch>                               <!-- present if part of batch -->
    <batch-id>batch-abc</batch-id>
    <completed>2</completed>
    <total>3</total>
    <remaining>research-auth</remaining>
  </batch>
  <coordinator-hint>                    <!-- advisory, LLM decides -->
    <phase>partial_batch</phase>
    <wait-for-remaining>true</wait-for-remaining>
  </coordinator-hint>
</task-notification>
```

### Injection Rules

1. User-role messages, injected at turn boundaries
2. Multiple completions merged into one message
3. Each fires exactly once (notified flag)

### Decision Table

```
Solo completion      ──► synthesize, report or proceed
Solo failure         ──► continue with corrected instructions
Partial batch        ──► wait for remaining
Batch + failures     ──► wait; optionally continue failed workers
Batch complete       ──► synthesize all, proceed to next phase
Batch + some failed  ──► synthesize successes, retry failures
```

### Batch Lifecycle

```
Coordinator turn
    │
    ├── Agent("research API", bg)   → worker-1 ─┐
    ├── Agent("research DB",  bg)   → worker-2 ─┤ batch-abc
    └── Agent("research auth", bg)  → worker-3 ─┘ total: 3

    worker-1 completes → 1/3
    worker-3 completes → 2/3  (partial, wait)
    worker-2 completes → 3/3  (all done → synthesize)
```

### Worker State

```
Worker
├── TaskID            unique identifier       ├── Progress     tool count, tokens
├── AgentName         human-readable name     ├── PendingMsgs  queued SendMessage
├── AgentType         Explore, general, ...   ├── Result       final output
├── Status            pending|running|done    └── Error        if failed
├── BatchID           which batch (if any)
```

Each worker runs with independent conversation, independent tools,
optional worktree isolation, inherited or overridden permissions.

## Context & Concurrency

```
┌─────────────────────────────────────────────────────────────────────┐
│  Context-Centric Decomposition                                       │
│                                                                      │
│  Good boundaries              Bad boundaries                         │
│  (context-coherent)           (context-incoherent)                   │
│                                                                      │
│  · independent components     · sequential phases of same work       │
│  · separate research Qs       · tightly coupled components           │
│  · blackbox verification      · work requiring shared state          │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│  Context Resets > Compaction                                         │
│                                                                      │
│  Models develop "context anxiety" near perceived limits.             │
│  Clean reset + structured handoff > summarize in place.              │
│                                                                      │
├──────────────────────────────────────────────────────────────────────┤
│  Concurrency Rules                                                   │
│                                                                      │
│  Read-only research ··················· parallel                     │
│  Implementation (disjoint files) ······ parallel                     │
│  Implementation (overlapping files) ··· serialize or worktree        │
│  Verification ························· parallel (independent)       │
│  Failure retry ························ continue same worker          │
└─────────────────────────────────────────────────────────────────────┘
```

## Example: End-to-End Coordination

```
User: Migrate auth from JWT to session tokens

═══ Phase 1: Research (parallel) ═══════════════════════════════════════

  Agent("research JWT usage", bg)         → worker-1 ─┐
  Agent("research session best practices", bg) → worker-2 ─┤ batch
  Agent("identify auth-dependent endpoints", bg) → worker-3 ─┘

  worker-1 ✓  "JWT in 4 files: auth.go, middleware.go, handler.go, test.go"
  worker-3 ✓  "12 endpoints depend on auth, 3 are public"
  worker-2 ✓  "session tokens need: store interface, cookie handling, CSRF"

═══ Phase 2: Synthesis (coordinator, no workers) ═══════════════════════

  Coordinator reads all 3 results, writes concrete specs:
    Spec A: session store (store.go, new file)
    Spec B: middleware swap (middleware.go, handler.go)

═══ Phase 3: Implementation (parallel, worktree-isolated) ══════════════

  Agent("implement session store", bg, worktree, spec-A) → worker-4
  Agent("implement middleware swap", bg, worktree, spec-B) → worker-5

  worker-4 ✗  "compilation error: Store missing Delete method"

  SendMessage(to: worker-4,
    "Add Delete(token string) error to Store interface.
     Caller is middleware.go:42.")

  worker-4 ✓  (after fix)
  worker-5 ✓

  Merge worktrees.

═══ Phase 4: Verification (independent) ════════════════════════════════

  Agent("run all tests, verify migration complete", bg) → worker-6

  worker-6 ✓  "all 47 tests pass, no JWT references remain"
```

## What The Coordinator Is Not

```
Not a scheduler      LLM decides what to launch and when
Not a state machine  no coded phase progression — LLM decides freely
Not a swarm          workers are independent agents, not pool threads
Not required         simple tasks use normal single-agent path
Not a separate system  same LLM, same agent loop, guided by prompt
```

## Related Docs

- [core.md](core.md) — single agent execution model
- [hook.md](hook.md) — app-layer event hooks
- [permission.md](permission.md) — tool permission pipeline
- [architecture.md](architecture.md) — TUI structure, notification injection
