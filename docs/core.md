# Core Agent Architecture

## Agent Construction

```
┌─────────────────────────────────────────────────────────────┐
│  core.NewAgent(Config)                                      │
│                                                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │   LLM    │  │  System   │  │  Tools   │                  │
│  │ (stream) │  │ (layers)  │  │(registry)│                  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                  │
│       │              │             │                        │
│       └──────────────┴─────────────┘                        │
│                          │                                  │
│                     ┌────┴────┐                              │
│                     │  Agent  │                              │
│                     │         │                              │
│               Inbox ◄────────►  Outbox                      │
│             (chan Message)  (chan Event)                      │
│                     └─────────┘                              │
│                                                             │
│  Optional: CWD, MaxTurns, CompactFunc                       │
│                                                             │
│  core.Agent has NO dependency on hooks.                     │
│  Hooks are an app-layer concern — see hook.md.              │
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
                       └──► both paths: append to conversation history
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

core.Agent knows nothing about hooks — it only sees `core.Tools` (which
may be wrapped with a permission decorator). For hook integration around
tool execution, see [permission.md](permission.md#hook-integration).

```
  tool calls from LLM
        │
        ▼
  ┌─── EMIT + RESOLVE (sequential) ─────────────┐
  │  for each call:                              │
  │    emit PreTool event (outbox)               │
  │    tools.Get(name) → tool (or nil → error)   │
  └──────────────────────────────────────────────┘
        │
        ▼
  ┌─── EXECUTE (parallel) ──────────────────────┐
  │  tool.Execute(ctx, params)                   │
  │    └─ if wrapped by WithPermission:          │
  │       IsSafeTool? → skip check               │
  │       PermissionFunc → Permit/Reject/Prompt  │
  │       Prompt → blocks on PermissionBridge    │
  │  panic recovery per goroutine                │
  └──────────────────────────────────────────────┘
        │
        ▼
  ┌─── RECORD (sequential) ─────────────────────┐
  │  append ToolResult to conversation           │
  │  emit PostTool event (outbox)                │
  └──────────────────────────────────────────────┘
```

## System Prompt Layers

```
Priority    Band             Source
─────────────────────────────────────────────────
  0-99      Identity         base template
100-199     Environment      provider, cwd, git, model
200-299     Instructions     user GEN.md, project GEN.md
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

## Outbox Events

core.Agent emits events to its Outbox channel at each lifecycle point.
The TUI observes these for rendering. **These are NOT hook events** —
hooks are an app-layer concern (see [hook.md](hook.md)).

```
  Agent Lifecycle                  Outbox Event            TUI Action
  ──────────────────────────────────────────────────────────────────────────

  Run() starts
    │
    ▼
  waitForInput()
    │ message arrives
    │
    ▼
  ThinkAct() loop
    │
    ├─► PreInfer ················· start stream spinner
    │
    ▼
  streamInfer()
    │
    ├─► OnChunk (per token) ······ append to streaming view
    │
    ├─► PostInfer ················ update token counts, set tool calls
    │
    ▼
  execTools()
    │
    │  for each tool call:
    │    ├─► PreTool ············· show "building tool" status
    │    │  tool.Execute()
    │    └─► PostTool ············ append tool result, apply side effects
    │
    ▼
  end of turn?
    ├─ tool calls → loop back to PreInfer
    └─ no calls
         ├─► OnTurn ·············· commit messages, save session, drain queues
         └─► back to waitForInput()

  Run() returns
    └─► OnStop ··················· cleanup agent session
```

Hook events (PreToolUse, PostToolUse, Stop, etc.) are fired by the
app layer in response to these outbox events — not by core.Agent itself.

## Auto Compaction

Compaction is an **app-level** concern, not a core.Agent responsibility.
The TUI triggers it when context usage exceeds 95%.

```
  TUI: ProcessTurnEnd()
    │
    ├─ check: InputTokens >= 95% of context limit?
    │   │
    │   yes ──► triggerAutoCompact()
    │   no  ──► continue
    │
    ▼

  COMPACT (app/conv/compact.go):
  ┌────────────────────────────────────────────────────┐
  │                                                    │
  │  CompactConversation(ctx, llmClient, msgs, focus)  │
  │       │                                            │
  │       ▼                                            │
  │  LLM summarizes conversation → summary string     │
  │                                                    │
  └────────────────────────────────────────────────────┘
    │
    ▼

  HandleCompactResult():
  ┌────────────────────────────────────────────────────┐
  │                                                    │
  │  1. conv.Clear() — wipe all messages               │
  │  2. Inject summary as user message:                │
  │     <session-summary>...</session-summary>         │
  │  3. Restore recently accessed files (filecache)    │
  │  4. If auto-continue: sendToAgent(resumePrompt)    │
  │                                                    │
  │  Summary lives in messages, not system prompt.     │
  │  Next turn's TokensIn reflects the smaller context.│
  │                                                    │
  └────────────────────────────────────────────────────┘

  Cumulative: each compaction sees the previous summary
  as a user message in the conversation being compacted.
```
