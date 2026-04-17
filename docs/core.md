# Core Agent Architecture

## Package Structure

```
internal/core/
├── agent.go / agent_impl.go       Agent interface + Run loop
├── system.go / system_impl.go     Layered system prompt
├── hook.go / hook_impl.go         Event hooks
├── llm.go                         LLM streaming interface
├── message.go                     Message, ToolCall, ToolResult
├── tool.go / tool_impl.go         Tool registry
└── system/
    ├── builder.go                 System prompt builder
    ├── memory.go                  GEN.md / CLAUDE.md loader
    └── prompts/                   Embedded templates
```

## Agent Construction

```
┌─────────────────────────────────────────────────────────────┐
│  core.NewAgent(Config)                                      │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────────┐  │
│  │   LLM    │  │  System   │  │  Tools   │  │   Hooks    │  │
│  │ (stream) │  │ (layers)  │  │(registry)│  │ (handlers) │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └─────┬──────┘  │
│       │              │             │              │         │
│       └──────────────┴─────────────┴──────────────┘         │
│                          │                                  │
│                     ┌────┴────┐                              │
│                     │  Agent  │                              │
│                     │         │                              │
│               Inbox ◄────────►  Outbox                      │
│             (chan Message)  (chan Event)                      │
│                     └─────────┘                              │
│                                                             │
│  Optional: Permission, AllowedTools, CWD, MaxTurns          │
└─────────────────────────────────────────────────────────────┘
```

## Agent Interface

```
  Agent
  ├── ID() string
  ├── System() System
  ├── Tools() Tools
  ├── Hooks() Hooks
  ├── Inbox() chan<- Message          // TUI path only
  ├── Outbox() <-chan Event           // events (both paths)
  ├── Messages() []Message            // conversation snapshot
  ├── SetMessages(msgs []Message)     // replace history (compaction, restore)
  │
  ├── Append(ctx, msg Message)        // add message + fire OnMessage hook
  │                                   // unified entry point for both paths
  │
  ├── Run(ctx) error                  // long-running loop (TUI)
  │     loop: waitForInput → ThinkAct
  │
  └── ThinkAct(ctx) (*Result, error)  // single inference cycle (subagent)
        PreInfer → stream → execTools → until end_turn
```

## Execution Model

`ThinkAct` is the agent's atomic operation — one full inference-action cycle.
Two callers drive it in different ways:

```
                      ThinkAct(ctx) → *Result
                      ┌──────────────────────────────────┐
                      │  PreInfer ──► LLM stream          │
                      │                  │                │
                      │             PostInfer             │
                      │                  │                │
                      │            tool calls?            │
                      │             │       │             │
                      │            Yes      No            │
                      │             │       │             │
                      │        execTools  return Result   │
                      │             │                     │
                      │        loop back to PreInfer      │
                      └──────────────────────────────────┘
                           ▲                    ▲
                           │                    │
            ┌──────────────┘                    └──────────────┐
            │                                                  │
     Run() — TUI                                    Direct call — Subagent
     ┌─────────────────┐                            ┌─────────────────────┐
     │  loop:          │                            │  ag.Append(prompt)  │
     │    waitForInput │                            │  ag.ThinkAct(ctx)   │
     │    ingest ──┐   │                            │                     │
     │    ThinkAct │   │                            │  ag.Append(followUp)│
     │    emit     │   │                            │  ag.ThinkAct(ctx)   │
     │  until Stop │   │                            │  ...                │
     └─────────────┘   │                            └─────────────────────┘
                       │
                       └──► both call: OnMessage hook + append to history
```

### Run Loop (TUI path)

```
          Inbox                              Outbox
            │                                  ▲
            ▼                                  │
┌───────────────────────────────────────────────────────────┐
│                                                           │
│   ┌─────────┐                                             │
│   │  WAIT   │◄──────────────────────────────────┐         │
│   │ (block) │                                   │         │
│   └────┬────┘                                   │         │
│        │ message arrives                        │         │
│        ▼                                        │         │
│   ┌─────────┐                                   │         │
│   │  DRAIN  │  non-blocking drain of            │         │
│   │         │  accumulated messages             │         │
│   └────┬────┘                                   │         │
│        ▼                                        │         │
│   ┌──────────────────────────────────────┐      │         │
│   │        ThinkAct(ctx) → Result        │      │         │
│   │                                      │      │         │
│   │  PreInfer ──► LLM stream ──► PostInfer      │         │
│   │                                │     │      │         │
│   │                          tool calls? │      │         │
│   │                           │       │  │      │         │
│   │                          Yes      No │      │         │
│   │                           │       │  │      │         │
│   │                           ▼       │  │      │         │
│   │                      ┌─────────┐  │  │      │         │
│   │                      │execTools│  │  │      │         │
│   │                      │ Gate    │  │  │      │         │
│   │                      │ Execute │  │  │      │         │
│   │                      │ Record  │  │  │      │         │
│   │                      └────┬────┘  │  │      │         │
│   │                           │       │  │      │         │
│   │                      loop back    │  │      │         │
│   │                      to PreInfer  │  │      │         │
│   │                                   │  │      │         │
│   │                              OnTurn ─┘      │         │
│   └──────────────────────────────────────┘      │         │
│                                    │            │         │
│                                    └────────────┘         │
│                                                           │
│   SigStop / ctx.Done ──► OnStop ──► return                │
└───────────────────────────────────────────────────────────┘
```

## Tool Execution (`execTools`)

```
  tool calls from LLM
        │
        ▼
  ┌─── GATE (sequential) ───────────────────────┐
  │  for each call:                              │
  │    Permission ──deny──► error result, skip   │
  │        │                                     │
  │      allow                                   │
  │        │                                     │
  │    PreTool hook                               │
  │        ├─ block ──► error result, skip       │
  │        ├─ modify ──► update input            │
  │        └─ pass ──► add to execution queue    │
  └──────────────────────────────────────────────┘
        │
        ▼
  ┌─── EXECUTE (parallel) ──────────────────────┐
  │  tool.Execute(ctx, params)                   │
  │  panic recovery per goroutine                │
  └──────────────────────────────────────────────┘
        │
        ▼
  ┌─── RECORD (sequential) ─────────────────────┐
  │  append ToolResult to conversation           │
  │  emit PostTool event                         │
  └──────────────────────────────────────────────┘
```

## System Prompt Layers

```
Priority    Band             Source
─────────────────────────────────────────────────
  0-99      Identity         base template
100-199     Environment      provider, cwd, git, model
200-299     Instructions     user GEN.md, project GEN.md
300-399     Memory           session summary
400-499     Capabilities     skills, agents, deferred tools
500-599     Guidelines       tool usage, git workflow
600-699     Mode             plan mode
700+        Extra            skill invocation, coordinator
─────────────────────────────────────────────────
                    │
                    ▼
            System.Prompt()
         (cached, rebuild on change)
```

## Hooks Lifecycle

Hooks observe and intercept every phase of the agent lifecycle. Each event is both emitted to Outbox (for TUI observation) and fired through Hooks (for interception).

```
  Agent Lifecycle                  Hook Events             Action Capabilities
  ──────────────────────────────────────────────────────────────────────────

  Run() starts
    │
    ├─► OnStart ·················· observe only
    │
    ▼
  waitForInput()
    │ message arrives
    ├─► OnMessage ················ observe only
    │
    ▼
  thinkAct() loop
    │
    ├─► PreInfer ················· Block: stop turn
    │                              Inject: add ephemeral context
    │                                      to system prompt (one turn)
    ▼
  streamInfer()
    │
    ├─► OnChunk (per token) ······ observe only
    │
    ├─► PostInfer ················ observe only
    │
    ▼
  execTools()
    │
    │  for each tool call:
    │    │
    │    ├─► PreTool ············· Block: skip this tool
    │    │                         Modify: rewrite tool input
    │    │
    │    │  tool.Execute()
    │    │
    │    └─► PostTool ············ observe only
    │
    ▼
  end of turn?
    ├─ tool calls → loop back to PreInfer
    └─ no calls
         │
         ├─► OnTurn ·············· observe only
         │
         └─► back to waitForInput()

  Run() returns
    │
    └─► OnStop ··················· observe only (guaranteed delivery)
            │
            ▼
         Hooks.Drain()  wait for async hooks
         close(Outbox)
```

### Hook Interception Points

```
                  can Block?    can Modify?    can Inject?
                  ─────────     ──────────     ──────────
  PreInfer           yes            -             yes
  PreTool            yes           yes             -
  all others          -             -              -
```

Only `PreInfer` and `PreTool` are interceptors; all other events are observation-only.

### Hook Execution Model

```
  event fires
      │
      ▼
  match hooks by EventType + Matcher (source filter)
      │
      ├─ Async hooks ──► goroutine (observe only, cannot block/modify)
      │
      ├─ Once hooks ──► atomic CAS, fire at most once
      │
      └─ Sync hooks ──► sequential execution
              │
              ▼
         merge Actions:
           Block:  any true → short-circuit
           Inject: concatenate
           Modify: last writer wins
           Meta:   merge maps
              │
              ▼
         return merged Action to agent
```

## Dual Execution Paths

```
┌──────────────────────────────────────────────────────────────────┐
│                                                                  │
│  TUI (main agent)                Subagent                        │
│  ────────────────                ────────                        │
│                                                                  │
│  ag := core.NewAgent(cfg)        ag := core.NewAgent(cfg)        │
│                                  ag.SetMessages(saved)  // opt   │
│  go ag.Run(ctx)                                                  │
│                                  ag.Append(ctx, prompt)          │
│  ag.Inbox() ← user message      result := ag.ThinkAct(ctx)      │
│  ag.Outbox() → TUI events                                       │
│                                  ag.Append(ctx, followUp)        │
│  loop:                           result2 := ag.ThinkAct(ctx)     │
│    wait → drain → ThinkAct       ...                             │
│    emit events to Outbox                                         │
│  until SigStop                   (hooks fire internally,         │
│                                   events emit to Outbox)         │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### TUI Event Handling

```
┌─────────┐     Inbox      ┌───────────┐    Outbox     ┌─────────┐
│   TUI   │ ──Message──►   │   Agent   │  ──Event──►   │   TUI   │
│ (input) │                │  Run loop │                │(output) │
└─────────┘                └───────────┘                └────┬────┘
                                                             │
                           ┌─────────────────────────────────┘
                           │
                    switch event.Type:
                      PreInfer  → activate stream
                      OnChunk   → append text
                      PostInfer → token counts
                      PreTool   → spinner
                      PostTool  → side effects, result
                      OnTurn    → commit, save, drain queues
                      OnStop    → cleanup
```

## Gaps (ThinkAct as foundation for subagents)

Features currently in `runtime.Loop` that `core.Agent` needs:

```
  Gap                          Where it lives now          How to bridge
  ─────────────────────────────────────────────────────────────────────────

  AllowedTools whitelist       tool.Set.Allow +            Config.AllowedTools
  (skip Permission check)      permission.PermitAll()      checked in execTools

  Token budget check           Loop pre-stream check       PreInfer hook with
  + auto compaction            + reactive on error         token info, or Config

  Reactive compaction          Loop catches prompt_too_    streamInfer retry
  on prompt-too-long           long, compacts, retries     or PreInfer hook

  Interactive tools            Loop.SetQuestionHandler     Config field or
  (AskUser)                    + tool.InteractiveTool      tool context

  Compaction context           RunOptions.SessionMemory    Config fields
  (session memory, focus)      + CompactFocus

  Agent identity               Loop.SetAgentContext        Config.AgentType
  in hook events               (agentID, agentType)        (ID already exists)

  OnToolStart / OnToolDone     RunOptions callbacks        Outbox PreTool /
  progress callbacks           for progress tracking       PostTool events
  ─────────────────────────────────────────────────────────────────────────

  Resolved by Append + ThinkAct design:
    ✓ Message injection — Append() before each ThinkAct() call
    ✓ Message hooks — Append() fires OnMessage, same as Run() path
    ✓ Conversation history — messages accumulate across ThinkAct() calls
```
