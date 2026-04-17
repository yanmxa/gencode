# Core Agent Architecture

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
│  Optional: CWD, MaxTurns, CompactFunc                       │
└─────────────────────────────────────────────────────────────┘
```

## Execution Model

`ThinkAct` is the agent's atomic operation — one full inference-action cycle.

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

## Run Loop (TUI path)

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

## Tool Execution

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
  ThinkAct() loop
    │
    ├─► PreInfer ················· Block | Inject
    │
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
    │    ├─► PreTool ············· Block | Modify
    │    │  tool.Execute()
    │    └─► PostTool ············ observe only
    │
    ▼
  end of turn?
    ├─ tool calls → loop back to PreInfer
    └─ no calls
         ├─► OnTurn ·············· observe only
         └─► back to waitForInput()

  Run() returns
    └─► OnStop ··················· observe only (guaranteed delivery)
```

## Auto Compaction

```
  ThinkAct(ctx)
    for each turn:
      │
      ├─ check: TokensIn >= 95% of LLM.InputLimit()?
      │   │
      │   yes ──► COMPACT
      │   │
      │   no
      │   ▼
      │  PreInfer → streamInfer()
      │                │
      │           prompt_too_long error?
      │                │
      │               yes ──► COMPACT → retry streamInfer
      │               no (other error) → return error
      │
      └─ continue turn loop


  COMPACT:
  ┌────────────────────────────────────────────────────┐
  │                                                    │
  │  CompactFunc(ctx, messages) → summary              │
  │       │                                            │
  │       ▼                                            │
  │  SetMessages([UserMessage("Previous context:\n"    │
  │               + summary)])                         │
  │                                                    │
  │  Summary lives in messages, not system prompt.     │
  │  Next turn's TokensIn reflects the smaller context.│
  │                                                    │
  └────────────────────────────────────────────────────┘

  Cumulative:
    Compaction₁: summary₁ = f(conversation₁)
    Compaction₂: summary₂ = f(summary₁ + conversation₂)
    Compaction₃: summary₃ = f(summary₂ + conversation₃)
                             └── managed by CompactFunc closure
```
