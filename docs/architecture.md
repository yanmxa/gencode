# Architecture

GenCode is a terminal AI agent built on [Bubble Tea](https://github.com/charmbracelet/bubbletea). The core design is an **event-based agent**: the Agent communicates through Inbox/Outbox channels, and the TUI observes events via the Bubble Tea MVU (Model-View-Update) loop. This channel-based, loosely-coupled architecture is designed for extensibility — each agent is an independent goroutine with its own Inbox/Outbox, agents interact only through messages with no shared mutable state, making it straightforward to scale from single-agent to multi-agent orchestration.

## Bubble Tea MVU

Bubble Tea 的核心是三个方法：

```go
type Model interface {
    Init() Cmd                           // called once at startup, returns first Cmd
    Update(Msg) (Model, Cmd)             // receives msg, returns updated Model + side effect
    View() string                        // reads Model, returns string to render
}
```

- **Init()**: returns the first `tea.Cmd` (start timers, fetch data). Called once.
- **Update()**: value receiver — gets a copy of Model, must return the updated copy. `tea.Msg` is any type (key event, timer tick, custom event). `tea.Cmd` is an async function that produces the next `tea.Msg` (`nil` = no side effect).
- **View()**: pure function, no side effects. Framework calls it after every `Update()`.

The loop: `Init → Cmd → Msg → Update → Cmd → Msg → ...`, with `View()` after every `Update`.

## Three-Source MVU

The Agent is the central processing unit. **Three input sources** feed its Inbox. The **Outbox** outputs events that mutate the TUI Model and trigger View. Together, 3 input sources + Agent Output form the four paths that update the Model.

```
          Source 1: User          Source 2: Agents         Source 3: System
          (human → agent)        (agent → agent)          (system → agent)
         ┌────────────────┐    ┌──────────────────┐     ┌──────────────────┐
         │ Submit message  │    │ Agent completion  │     │ Cron (scheduled)  │
         │ Slash command   │    │ SendMessage       │     │ Async hook rewake │
         │ Modal response  │    │ Self-inject       │     │ File change       │
         │  (approval/Q&A) │    │  (hook blocked)   │     │                   │
         └───────┬─────────┘    └────────┬──────────┘     └────────┬──────────┘
                 │                       │                          │
                 │    sendToAgent()      │   sendToAgent()          │  sendToAgent()
                 └───────────────►┌──────┴──────┐◄─────────────────┘
                                  │  Agent Inbox │
                                  └──────┬───────┘
                                         │
                                  ┌──────▼───────┐
                                  │   Run Loop    │
                                  │  wait → drain │
                                  │  → LLM → tool │
                                  │  → LLM → ...  │
                                  └──────┬────────┘
                                         │
                                  ┌──────▼───────┐
                                  │ Agent Outbox  │──────► TUI Observation
                                  └──────────────┘        (or to other Agent)
```

All three sources converge at the Agent Inbox via `sendToAgent()`. The Outbox flows one-way to the TUI for rendering or to other Agent.

### Full Cycle

```
   Source 1 (User)       Source 2 (Agents)        Source 3 (System)
   submit ──┐            agentDone ──┐            cronTick ──────┐
   command ──┤           sendMsg ────┤            asyncHook ─────┤
   modalResp ┤           selfInject ─┤            fileChange ────┤
             ▼                       ▼                           ▼
          user/                    agent/                   system/
             │                       │                           │
             └───────────────┬───────┴───────────────────────────┘
                             │ sendToAgent()
                             ▼
   ╔═══════════════════════════════════════════════════════════════════╗
   ║                         Agent                                     ║
   ║                                                                   ║
   ║  ┌──────────────────────────────────────────────────────┐        ║
   ║  │                      Inbox                            │        ║
   ║  └──────────────────────────┬───────────────────────────┘        ║
   ║                             ▼                                     ║
   ║  ┌──────────────────────────────────────────────────────┐        ║
   ║  │  Run Loop: wait(inbox) → drain → think+act cycle     │        ║
   ║  │                                                      │        ║
   ║  │  LLM infer ──► tool exec ──► LLM infer ──► ...      │        ║
   ║  │       │              │                               │        ║
   ║  │       │         PostTool(Agent)                      │        ║
   ║  │       │              │ spawn                         │        ║
   ║  │       ▼              ▼                               │        ║
   ║  │  end_turn     Background Agents                      │        ║
   ║  │       │        ┌─────┐ ┌─────┐                       │        ║
   ║  │       │        │ B1  │ │ B2  │ ...                   │        ║
   ║  │       │        └──┬──┘ └──┬──┘                       │        ║
   ║  │       │           │complete                          │        ║
   ║  │       │           ▼                                  │        ║
   ║  │       │        → Source 2 (agent → agent)            │        ║
   ║  └───────┼──────────────────────────────────────────────┘        ║
   ║          ▼                                                        ║
   ║  ┌──────────────────────────────────────────────────────┐        ║
   ║  │                     Outbox                            │        ║
   ║  │  PreInfer · OnChunk · PostInfer                       │        ║
   ║  │  PreTool  · PostTool · OnTurn · OnStop                │        ║
   ║  └──────────────────────────┬───────────────────────────┘        ║
   ║                             │                                     ║
   ║  ┌────────┐                 │                                     ║
   ║  │PermReq │ ── bridge ──►  │                                     ║
   ║  │Channel │    Source 1     │                                     ║
   ║  └────────┘                 │                                     ║
   ╚═════════════════════════════╪═════════════════════════════════════╝
                                 │
                                 ▼
                        TUI Observation
                   ┌──────────────────────┐
                   │  agentOutboxMsg       │
                   │  → sync conv state    │
                   │  → update tokens      │
                   │  → render streaming   │
                   │                       │
                   │  agentPermMsg         │
                   │  → show approval      │
                   │  → user Y/N           │
                   │  → bridge to Source 1  │
                   └──────────────────────┘
```

**Three input loops**:
- **Source 1**: User submits → `sendToAgent()` → Inbox. If agent is busy, queued until OnTurn drains.
- **Source 2**: Background agent completes → task notification → `sendToAgent()` → Inbox. Also: `SendMessage` cross-agent, self-inject when Stop hook blocks.
- **Source 3**: Timer fires (cron/hook/watcher) → `sendToAgent()` → Inbox (buffered channel).

**Output path**: Agent Outbox → TUI observes events for rendering. PermReq bridges back to Source 1 (approval dialog → user decision → unblock agent).

**Feedback at OnTurn**: When the agent finishes a think+act cycle, the TUI drains queued Source 1, 2, and 3 items back into the Inbox, restarting the loop until all queues are empty.

### Source 1: User Input (human → agent)

```
KeyMsg
  │
  ├─ Modal active? ──→ delegate to approval/plan/question  (TUI-local)
  ├─ Overlay active? ─→ delegate to selector               (TUI-local)
  ├─ Special mode? ───→ image/suggestion/queue navigation   (TUI-local)
  ├─ Shortcut? ───────→ Esc/Ctrl+C/Ctrl+T/...              (TUI-local)
  │
  └─ Enter (Submit)
       │
       ├─ turnActive? ──→ enqueue(input)     (queued → drained at OnTurn)
       ├─ hook blocked? ─→ addNotice         (rejected)
       ├─ isCommand? ────→ dispatchCommand   (TUI-local or feature trigger)
       │
       └─ message
            prepareUserMessage(input, images)
            conv.Append(userMsg)
            ensureAgentSession()
            sendToAgent() ──→ Agent Inbox
```

### Source 2: Agent Events (agent → agent)

```
Background Agent completes  → sendToAgent() ──→ Inbox (buffered channel)
SendMessage (cross-agent)   → sendToAgent() ──→ Inbox (buffered channel)
Self-inject (hook blocked)  → sendToAgent() ──→ Inbox (buffered channel)
```

### Source 3: System Events (system → agent)

```
cron.Tick()    → sendToAgent() ──→ Inbox (buffered channel)
asyncHookCb()  → sendToAgent() ──→ Inbox (buffered channel)
fileWatcher    → sendToAgent() ──→ Inbox (buffered channel)
```

All sources write directly to the buffered Inbox channel. The agent consumes messages at its own pace: `waitForInput()` when idle, `drainInbox()` between turns.

### Agent Output (outbox → Model → View)

The Outbox is the agent's output channel. It is **not** an input source (doesn't feed the Inbox), but it **does** mutate the TUI Model and trigger View re-render.

```
agentOutboxMsg
  │
  ├─ PreInfer  → conv.stream.active = true, commit pending
  ├─ OnChunk   → conv.AppendToLast(text, thinking)
  ├─ PostInfer → update token counts, set tool calls
  ├─ PreTool   → conv.stream.buildingTool = name
  ├─ PostTool  → applySideEffects, conv.Append(toolResult)
  ├─ OnTurn    → stop stream, commit, save session, fire hooks
  └─ OnStop    → cleanup agent session

agentPermMsg
  │
  └─ Agent blocks on permission → TUI shows approval dialog
     → User Y/N → unblock agent (bridges back to Source 1)
```

**Model is mutated by four paths**: Source 1, 2, 3 (→ Inbox) + Agent Output (→ Model). All mutations trigger View.

**OnTurn is the feedback hub**: when the agent finishes a think+act cycle, the TUI drains queued Source 1, 2, and 3 items back into the Inbox. This continues until all queues are empty and no hooks block.

## Model

Five sub-Models — four mutation sources + shared runtime:

```go
type model struct {
    userInput   user.Model      // textarea, history, overlays, approval, modal
    agentInput  agent.Model     // background agent notifications, batch tracking
    systemInput system.Model    // cron scheduler, async hooks, file watcher
    agentOutput output.Model    // conversation, streaming, tool results, agent session
    runtime     runtime.Model   // shared state: provider, permissions, session, config
}
```

Each sub-Model owns its component state, update routing, and view rendering.
The root `update.go` dispatches messages to the appropriate sub-Model;
the root `view.go` composes their views into the final layout.

## Update

```
msg → Update(msg) → handler mutates Model → return tea.Cmd
                                                    ↓
                                          framework calls View()
                                                    ↓
                                          View() reads Model → render → terminal
                                                    ↓
                                          tea.Cmd produces new msg → loop

  msg type      → updates      → handler
  ──────────────────────────────────────────────────
  user msgs     → m.userInput   → handleKey, handleSubmit, ...
  agent msgs    → m.agentInput  → handleTaskNotif, ...
  system msgs   → m.systemInput → handleCronTick, handleAsyncHook, ...
  outbox msgs   → m.agentOutput → handleOutboxEvent, handlePermRequest, ...
  runtime msgs  → m.runtime     → handleConfigReload, modeToggle, providerSwitch
```

## View

```
View() → reads Model → renders terminal

  Model field   → renders
  ──────────────────────────────────────────────────
  userInput     → input textarea, overlay selector, modal dialog
  agentInput    → task tracker (background agent progress)
  systemInput   → (inline notices when cron/hook injects)
  agentOutput   → chat messages, streaming content, tool results
  runtime       → status bar: mode, model name, thinking level, tokens
```

## App Directory Structure

Target layout — files organized by **input source** (who triggered the mutation).

Each sub-package uses a flat component pattern: `update.go` and `view.go` only
do routing, each component lives in its own `_xxx.go` file containing its model,
update, and view logic. No nested sub-packages for individual UI components.

```
internal/app/
│
│  ── Core MVU ──────────────────────────────────
├── model.go                    # Model struct, Init(), agent session builder
├── update.go                   # Update() top-level dispatch
├── view.go                     # View() layout composition
├── init.go                     # Infrastructure initialization
│
├── user/                       # User Input source
│   ├── model.go                #   main Model (textarea, history, images, queue)
│   ├── update.go               #   routing: key → component update
│   ├── view.go                 #   routing: component → render
│   ├── _approval.go            #   Approval: model + update + view
│   ├── _provider.go            #   Provider selector: model + update + view
│   ├── _session.go             #   Session selector: model + update + view
│   ├── _search.go              #   Search engine selector: model + update + view
│   ├── _skill.go               #   Skill selector
│   ├── _memory.go              #   Memory selector
│   ├── _mcp.go                 #   MCP server selector
│   ├── _plugin.go              #   Plugin selector
│   └── _agent.go               #   Agent/subagent selector
│
├── agent/                      # Agent Input source
│   ├── model.go                #   State: notifications, batch tracking
│   ├── update.go               #   routing: notification → handler
│   ├── _notification.go        #   notification queue + build logic
│   └── _tracker.go             #   background worker/batch tracking
│
├── system/                     # System Input source
│   ├── model.go                #   State: cron, async hooks
│   ├── update.go               #   routing: tick → handler
│   ├── view.go                 #   cron status rendering
│   └── _file_watcher.go        #   file change detection
│
├── output/                     # Agent Output
│   ├── model.go                #   streaming, progress, permission bridge
│   ├── update.go               #   routing: outbox event → handler
│   ├── view.go                 #   chat messages, streaming, tool results
│   ├── _conversation.go        #   message history, stream state
│   ├── _modal.go               #   plan approval, question prompts
│   ├── _compact.go             #   context compaction
│   ├── _toolui.go              #   tool selector + execution state
│   ├── _progress.go            #   progress hub for background agents
│   └── _render.go              #   markdown rendering, message formatting
│
├── runtime/                    # Shared Runtime State
│   ├── model.go                #   Model: provider, permissions, session, plan, config
│   ├── update.go               #   config reload, mode toggle, provider switch
│   ├── view.go                 #   status bar: mode, model name, thinking, tokens
│   ├── _provider.go            #   LLM connection, model info, token tracking
│   ├── _session.go             #   session store, ID, summary, compaction
│   ├── _permission.go          #   operation mode, session permissions, disabled tools
│   └── _plan.go                #   plan mode state
│
├── kit/                        # Shared UI utilities
│   ├── suggest/                #   autocomplete
│   └── history/                #   input history
│
```

Agent builder (buildCoreAgent, ensureAgentSession, startAgentLoop) belongs in `model.go` — it's Model initialization, not an Update handler.

## Package Dependencies

```
cmd/gen/              CLI entrypoint
internal/app/         TUI layer (this document)
internal/core/        Agent interface: Inbox/Outbox/Run, Event types, Message
internal/llm/         LLM providers (Anthropic, OpenAI, Google, ...)
internal/tool/        Tool registry and execution
internal/hook/        Event hook system
internal/config/      Settings and permissions
internal/...          ...
```

Dependency direction: `cmd/ → app/ → {core/, provider/, tool/, hooks/, config/, ...}`. Domain packages never import app.
