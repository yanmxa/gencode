# Architecture

GenCode is a terminal AI agent built on [Bubble Tea](https://github.com/charmbracelet/bubbletea). The core design is an **event-based agent**: the Agent communicates through Inbox/Outbox channels, and the TUI observes events via the Bubble Tea MVU (Model-View-Update) loop. This channel-based, loosely-coupled architecture is designed for extensibility — each agent is an independent goroutine with its own Inbox/Outbox, agents interact only through messages with no shared mutable state, making it straightforward to scale from single-agent to multi-agent orchestration.

## Bubble Tea MVU

```go
type Model interface {
    Init() Cmd                  // called once at startup, returns first Cmd
    Update(Msg) (Model, Cmd)    // receives msg, returns updated Model + side effect
    View() string               // reads Model, returns string to render
}
```

The loop: `Init → Cmd → Msg → Update → Cmd → Msg → ...`, with `View()` after every `Update`.

### Best Practices

1. **Model is data, Update is transition, View is pure render, Cmd is side effect.** All I/O goes through `tea.Cmd` — never do I/O inside Update. Each event returns a new listen Cmd, forming a chain.
2. **Sub-model decomposition.** Split Model by event source. Each sub-model owns its `Update()` and `View()`. The root Model routes by `tea.Msg` type.
3. **Message types are routing keys.** Define concrete msg types — the more precise, the cleaner the switch. No string/int dispatch.
4. **State machine over bool flags.** Use explicit mode enums to control UI behavior. Both Update and View branch on mode, avoiding combinatorial flag explosion.
5. **Sub-models call up via Runtime interfaces, never import root.** The root implements each sub-model's Runtime interface through an adapter struct.
6. **Cmd factories live with their handlers.** Each sub-model owns its Cmd factories (e.g. `conv/` owns `DrainAgentOutbox()`). Cross-cutting Cmds (e.g. `sendToAgent()`) live in root files. No central `cmd.go` — co-location beats centralization at scale.
7. **Project structure mirrors the architecture:**

```
app/
  model.go     root Model{input, notify, trigger, conv, runtime} + Init()
               conv.Runtime event handlers, deps builders, injection handlers
  agent.go     Agent session lifecycle, system prompt, tool set, LLM client
  update.go    Update(): routes msgs to sub-models by type
  view.go      View(): composes sub-model views
  run.go       entrypoint / lifecycle
  init.go      Global infrastructure init, plugin/mcp adapters
  input/       keyboard → input state → textarea render
  notify/      task completion → notification queue → inject into conversation
  trigger/     cron / hook / watcher → event queue → inject into conversation
  conv/        outbox events → conversation state → chat render
  runtime/     shared state: provider, session, permission, config
```

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

## App Structure

### First Principle

Root is **pure glue** — it defines the composite Model, routes messages, composes views, and implements Runtime adapters. All business logic lives in sub-models. If a root file can't be named in two words, the logic belongs in a sub-model.

### Sub-model Convention

Each sub-model package is a self-contained MVU unit:

| File | Rule |
|------|------|
| `model.go` | State definition. All fields the package owns live here. |
| `update.go` | `Update()` entry point. Routes msgs to handlers. Returns `(tea.Cmd, bool)`. |
| `view.go` | Pure render. Reads Model, returns string. |
| `runtime.go` | Runtime interface — the only way to call up to root. Never import root. |
| `on_*.go` | Component files. Each `on_` file owns one component's state + update + view. |
| `*.go` | Data structures and renderers (no `on_` prefix) in non-input packages like `conv/`. |

Root implements each sub-model's Runtime via adapter methods on `*model` in `model.go`.

### Root Model

```go
type model struct {
    userInput   input.Model      // Source 1: user keyboard → textarea, selectors, approval
    agentInput  notify.Model     // Source 2: background agent completion → notification queue
    systemInput trigger.Model    // Source 3: cron / async hook / file watcher → event queue
    conv        conv.Model       // Agent Outbox: outbox events → conversation state → chat render
    runtime     runtime.Model    // Shared: provider, session, permission, plan, config
}
```

### Update — Root is Just a Switch

```go
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:                 return m, m.input.HandleKey(msg)
    case input.SubmitMsg:            return m, m.handleSubmit(msg)     // cross-cutting: input → agent
    case input.CommandMsg:           return m, m.dispatchCommand(msg)  // cross-cutting: input → effect
    case conv.PermissionRequestMsg:  return m, m.input.ShowApproval(msg.Req)
    case input.ApprovalResponseMsg:  return m, m.conv.ResolvePermission(msg)
    case conv.OutboxMsg:             return m, m.conv.HandleOutbox(msg)
    case notify.TickMsg:             return m, m.notify.HandleTick(msg)
    case trigger.TickMsg:            return m, m.trigger.HandleTick(msg)
    // ...
    }
}
```

Cross-cutting coordination is **msg routing**, not business logic. Each `case` is 1-2 lines — take a msg from one sub-model, hand it to another.

### View — Just Compose

```
View()
  ├── m.conv.View()       → chat messages, streaming, tool results
  ├── m.conv.TrackerView()→ background task progress
  ├── m.input.View()      → textarea + image indicators
  └── m.runtime.View()    → status bar: mode, model, tokens
```

### Directory Structure

```
internal/app/
│
│  ── Root: pure glue (6 files) ─────────────────────────────────────────────
│  No business logic. Model + routing + view + agent lifecycle + entrypoint + init.
│
├── model.go          # Model{5 sub-models}, Init(), construction
│                     # conv.Runtime event handlers, message pipeline, session persistence
├── agent.go          # Agent session lifecycle: build, start, stop, send, permission bridge
│                     # Agent tool config, system prompt, tool set, LLM client
├── update.go         # Update(): msg type switch → delegate to sub-models
│                     # Cross-cutting = routing: SubmitMsg → agent, PermReq → input, ...
├── view.go           # View(): compose sub-model views into terminal layout
├── run.go            # Run(): tea.Program setup, entrypoint
├── init.go           # Global infrastructure init, plugin/mcp adapters
│
│  ── input/ ── Source 1: User Input ────────────────────────────────────────
│  Event: tea.KeyMsg
│  Flow:  keyboard → textarea | selector | approval
│         Enter → SubmitMsg → root routes to agent
│         /cmd  → CommandMsg → root dispatches effect
│
├── input/
│   ├── model.go             # Model{Textarea, History, Images, Queue, Selectors}
│   ├── update.go            # Update(): routes to active overlay or textarea
│   ├── view.go              # RenderTextarea(), image indicators
│   ├── runtime.go           # OverlayDeps struct for overlay handler dependency injection
│   ├── command_controller.go # CommandController: slash command dispatch (owns CommandRuntime)
│   ├── submit.go            # HandleSubmit(), prepareUserMessage() (owns SubmitRuntime)
│   ├── approval_flow.go     # ApprovalFlowDeps, HandlePermissionRequest() (owns ApprovalRuntime)
│   ├── prompt_suggestion.go # PromptSuggestion state and commands
│   ├── on_textarea.go       # HandleTextareaUpdate(), HistoryUp/Down()
│   ├── on_queue.go          # Queue: Enqueue(), Dequeue(), selection state
│   ├── on_image.go          # HandleImageSelectKey()
│   ├── on_approval.go       # ApprovalModel: Show() → HandleKeypress()
│   ├── on_approval_bash.go  # bash command preview
│   ├── on_approval_diff.go  # file diff preview
│   ├── on_agent.go          # AgentSelector
│   ├── on_provider.go       # ProviderSelector + on_provider_view.go
│   ├── on_plugin.go         # PluginSelector + on_plugin_command.go, on_plugin_view.go
│   ├── on_mcp.go            # MCPSelector + /mcp commands
│   ├── on_session.go        # SessionSelector
│   ├── on_memory.go         # MemorySelector + /init, /memory commands
│   ├── on_skill.go          # SkillSelector
│   ├── on_search.go         # SearchSelector
│   ├── on_tool_selector.go  # ToolSelector
│   └── on_token_limits.go   # Token limit fetching
│
│  ── notify/ ── Source 2: Background Agent Completion ──────────────────────
│  Event: task.TaskCompleted observer → NotificationQueue.Push()
│  Flow:  tick → PopReady() → BuildContinuationPrompt() → sendToAgent()
│
├── notify/
│   ├── model.go             # Model{NotificationQueue}, Push(), Pop()
│   ├── update.go            # Update(), handleTick()
│   ├── notification.go      # BuildTaskNotification(), MergeNotifications()
│   └── tracker.go           # EnsureBackgroundBatch(), UpdateWorker()
│
│  ── trigger/ ── Source 3: System Events ───────────────────────────────────
│  Event: cron tick | async hook callback | file watcher
│  Flow:  event → queue → tick → sendToAgent()
│
├── trigger/
│   ├── model.go             # Model{CronQueue, AsyncHookQueue}, RenderHookStatus()
│   ├── update.go            # Update(), handleCronTick(), handleAsyncHookTick()
│   └── file_watcher.go      # NewFileWatcher(), SetPaths(), poll()
│
│  ── conv/ ── Agent Outbox → Conversation ──────────────────────────────────
│  Event: core.Event from Agent Outbox channel
│  Flow:  DrainOutbox() → OutboxMsg → handleAgentEvent()
│         PreInfer → OnChunk → PostInfer → PreTool → PostTool → OnTurn
│
├── conv/
│   ├── model.go             # Model{ConversationModel, OutputModel} composite
│   ├── update.go            # handleAgentEvent(): PreInfer/.../OnTurn dispatch
│   ├── runtime.go           # Runtime interface, AgentOutboxMsg, PermBridgeMsg
│   ├── view.go              # RenderMessageRange(), RenderActiveContent()
│   ├── conversation.go      # Append(), ConvertToProvider(), StreamState
│   ├── compact.go           # CompactConversation(), CompactCmd(), CompactState
│   ├── tool.go              # ToolExecState + executeApproved()
│   ├── tool_render.go       # Tool result rendering
│   ├── modal.go             # ModalState type definitions
│   ├── plan.go              # PlanPrompt: Show() → PlanResponseMsg
│   ├── question.go          # QuestionPrompt: Show() → QuestionResponseMsg
│   ├── enterplan.go         # EnterPlanPrompt: Show() → EnterPlanResponseMsg
│   ├── message.go           # RenderAssistantMessage(), RenderUserMessage()
│   ├── markdown.go          # MDRenderer: Render()
│   ├── progress.go          # ProgressHub: SendForAgent(), Check()
│   └── permission_bridge.go # PermissionBridge: PermissionFunc(), Recv()
│
│  ── Shared ────────────────────────────────────────────────────────────────
│
├── runtime/
│   └── model.go             # Model{Provider, Session, Permission, Plan, Config, Tokens}
│
└── kit/
    ├── suggest/             # autocomplete engine
    ├── history/             # input history
    ├── theme.go, styles.go  # colors, lipgloss styles
    ├── listnav.go           # list navigation helpers
    ├── editor.go            # external editor integration
    └── msg.go, save_level.go, util.go
```

## Package Dependencies

```
cmd/gen/              CLI entrypoint
internal/app/         TUI layer (this document)
internal/core/        Agent interface: Inbox/Outbox/Run, Event types, Message
internal/llm/         LLM providers (Anthropic, OpenAI, Google, ...)
internal/tool/        Tool registry and execution
internal/hook/        Event hook system (depends on setting/ for env vars, llm/ for prompt hooks)
internal/setting/     Settings and permissions
internal/...          ...
```

Dependency direction: `cmd/ → app/ → {core/, llm/, tool/, hook/, setting/, ...}`.
Domain packages never import `app/`.

**Lateral dependencies** (same-layer, documented):
- `hook/` → `setting/` (env var resolution), `llm/` (prompt/agent hook execution)
- `session/` → `task/tracker` (serializes tracker tasks into transcripts)

**Decoupled via callback injection** (no direct import):
- `mcp/` ↔ `plugin/` — `mcp.Initialize(cwd, pluginServersCallback)`
- `subagent/` ↔ `plugin/` — `subagent.Initialize(cwd, pluginAgentPathsCallback)`
