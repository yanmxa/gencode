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
  update.go    Update(): routes msgs to sub-models by type
  view.go      View(): composes sub-model views
  bridges.go   Runtime adapters: root → sub-model interfaces
  run.go       entrypoint / lifecycle
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

### Design Principles

Each sub-model package is a self-contained MVU unit:

| Convention | Rule |
|-----------|------|
| `model.go` | State definition. All fields the package owns live here. |
| `update.go` | `Update()` entry point. Routes msgs to handlers. |
| `view.go` | Pure render. Reads Model, returns string. |
| `on_*.go` | Component handlers: each `on_` file owns one UI component's state + update + view. |
| `runtime.go` | Runtime interface. Sub-model calls up to root through this; never imports root. |

Root implements each sub-model's Runtime via adapter structs in `bridges.go`. If a method only reads/writes its own sub-model's state, it belongs inside the sub-model — not in the Runtime interface.

### Root Model

```go
type model struct {
    input    input.Model       // Source 1: user keyboard → textarea, selectors, approval
    notify   notify.Model      // Source 2: background agent completion → notification queue
    trigger  trigger.Model     // Source 3: cron / async hook / file watcher → event queue
    conv     conv.Model        // Agent Outbox: outbox events → conversation state → chat render
    runtime  runtime.Model     // Shared: provider, session, permission, plan, config
}
```

Root `update.go` dispatches by msg type; root `view.go` composes sub-model views.

### Update Dispatch

```
msg type        → sub-model    → handlers
────────────────────────────────────────────────────
tea.KeyMsg      → m.input      → handleKeypress, handleSubmit
notifyTickMsg   → m.notify     → handleTaskNotificationTick
triggerTickMsg   → m.trigger    → handleCronTick, handleAsyncHookTick
conv.OutboxMsg  → m.conv       → handlePreInfer, OnChunk, PostTool, OnTurn ...
runtimeMsg      → m.runtime    → handleConfigReload, modeToggle
```

### View Composition

```
View()
  ├── m.conv.View()       → chat messages, streaming content, tool results
  ├── m.conv.TrackerView()→ background task progress
  ├── m.input.View()      → textarea + image indicators
  └── m.runtime.View()    → status bar: mode, model, tokens
```

### Directory Structure

```
internal/app/
│
│  ── Root MVU + cross-cutting ──────────────────────────────────────────────
│
├── model.go          # root Model{input, notify, trigger, conv, runtime}, Init()
├── update.go         # Update(): routes msgs to sub-models by type
├── view.go           # View(): composes sub-model views into terminal layout
├── init.go           # newModel(), initInfrastructure(), applyRunOptions()
├── run.go            # Run(): entrypoint, tea.Program setup
├── runprint.go       # runPrint(): headless non-interactive mode
│
├── bridges.go        # Runtime adapters: root model → sub-model Runtime interfaces
├── submit.go         # handleSubmit() → prepareUserMessage() → sendToAgent()
├── command.go        # slash command registry: /help, /clear, /compact, ...
├── approval.go       # permission approval flow across input ↔ conv sub-models
├── mode.go           # plan/question/enterplan modal request ↔ response
├── agent_config.go   # buildLoopClient(), buildLoopSystem(), buildLoopToolSet()
├── lifecycle.go      # changeCwd(), reloadProjectContext()
├── tool_exec.go      # executeApproved(): dispatches tool calls
│
│  ── input/ ── Source 1: User Input ────────────────────────────────────────
│  Event: tea.KeyMsg
│  Flow:  keyboard → handleKeypress() → textarea | selector | approval
│         Enter    → handleSubmit()   → sendToAgent() → Agent Inbox
│
├── input/
│   ├── model.go             # Model{Textarea, History, Images, Queue, Selectors}
│   ├── update.go            # Update(): routes to active overlay or textarea
│   ├── view.go              # RenderTextarea(), image indicators
│   ├── on_textarea.go       # HandleTextareaUpdate(), HistoryUp/Down()
│   ├── on_queue.go          # Enqueue(), Dequeue() — mid-stream input buffer
│   ├── on_image.go          # HandleImageSelectKey()
│   ├── on_approval.go       # ApprovalModel: Show() → HandleKeypress() → ResponseMsg
│   ├── on_approval_bash.go  # bash command preview
│   ├── on_approval_diff.go  # file diff preview
│   ├── on_agent.go          # AgentSelector: list/toggle subagent definitions
│   ├── on_provider.go       # ProviderSelector: switch LLM provider/model
│   ├── on_provider_view.go  # provider selector rendering
│   ├── on_plugin.go         # PluginSelector: install/enable/disable plugins
│   ├── on_plugin_command.go # /plugin subcommand handlers
│   ├── on_plugin_view.go    # plugin selector rendering
│   ├── on_mcp.go            # MCPSelector: connect/disconnect MCP servers
│   ├── on_session.go        # SessionSelector: resume/fork past sessions
│   ├── on_memory.go         # MemorySelector: edit CLAUDE.md / memory files
│   ├── on_skill.go          # SkillSelector: browse/toggle skills
│   ├── on_search.go         # SearchSelector: pick web search backend
│   └── on_token_limits.go   # HandleTokenLimitCommand()
│
│  ── notify/ ── Source 2: Background Agent Completion ──────────────────────
│  Event: task.TaskCompleted observer → NotificationQueue.Push()
│  Flow:  tick → PopReadyNotifications() → BuildContinuationPrompt()
│              → sendToAgent() → Agent Inbox
│
├── notify/
│   ├── model.go             # Model{NotificationQueue}, Push(), Pop(), PopBatch()
│   ├── update.go            # Update(), handleTaskNotificationTick()
│   ├── on_notification.go   # BuildTaskNotification(), MergeNotifications()
│   └── on_tracker.go        # EnsureBackgroundBatchTracker(), UpdateWorkerTracker()
│
│  ── trigger/ ── Source 3: System Events ───────────────────────────────────
│  Event: cron tick | async hook callback | file watcher
│  Flow:  event → queue → tick → sendToAgent() → Agent Inbox
│
├── trigger/
│   ├── model.go             # Model{CronQueue, AsyncHookQueue}
│   ├── update.go            # Update(), handleCronTick(), handleAsyncHookTick()
│   ├── view.go              # RenderHookStatus()
│   └── on_file_watcher.go   # NewFileWatcher(), SetPaths(), poll()
│
│  ── conv/ ── Agent Outbox → Conversation ──────────────────────────────────
│  Event: core.Event from Agent Outbox channel
│  Flow:  DrainAgentOutbox() → OutboxMsg → handleAgentEvent()
│           PreInfer → OnChunk → PostInfer → PreTool → PostTool → OnTurn
│
├── conv/
│   ├── model.go             # Model{Conversation, Stream, Compact, Modal, Tool, Progress}
│   ├── update.go            # handleAgentEvent(), handlePreInfer/.../handleTurn()
│   ├── runtime.go           # Runtime interface: only cross-cutting operations
│   ├── view.go              # RenderMessageRange(), RenderActiveContent()
│   │   ── conversation state ──
│   ├── conversation.go      # ConversationModel: Append(), AppendToLast(), ConvertToProvider()
│   ├── compact.go           # CompactState: ShouldAutoCompact(), CompactConversation()
│   ├── stream.go            # StreamState: Activate(), Stop(), BuildingTool
│   │   ── tool execution ──
│   ├── tool.go              # ToolExecState: Begin(), Reset(), DrainPendingCalls()
│   ├── tool_selector.go     # ToolSelector: EnterSelect(), Toggle(), Render()
│   │   ── modals ──
│   ├── modal.go             # ModalState type definitions
│   ├── plan.go              # PlanPrompt: Show() → HandleKeypress() → PlanResponseMsg
│   ├── question.go          # QuestionPrompt: Show() → HandleKeypress() → QuestionResponseMsg
│   ├── enterplan.go         # EnterPlanPrompt: Show() → HandleKeypress() → EnterPlanResponseMsg
│   │   ── rendering ──
│   ├── message.go           # RenderAssistantMessage(), RenderUserMessage(), RenderToolCalls()
│   ├── markdown.go          # MDRenderer: Render(), splitTables()
│   ├── progress.go          # ProgressHub: SendForAgent(), Check(), RenderTrackerList()
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
