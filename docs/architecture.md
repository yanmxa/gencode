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
7. **Project structure mirrors the architecture.** See [Directory Structure](#directory-structure) below.

## Three-Source MVU

Three input sources feed the Agent Inbox. The Outbox outputs events that mutate the TUI Model and trigger View. Together, 3 inputs + Agent Output form the four paths that update the Model.

```
   Source 1 (User)       Source 2 (Agents)        Source 3 (System)
   submit ──┐            agentDone ──┐            cronTick ──────┐
   command ──┤           sendMsg ────┤            asyncHook ─────┤
   modalResp ┤           selfInject ─┤            fileChange ────┤
             ▼                       ▼                           ▼
          input/                  notify/                   trigger/
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
                        TUI Observation (conv/)
                   ┌──────────────────────┐
                   │  AgentOutboxMsg       │
                   │  → sync conv state    │
                   │  → update tokens      │
                   │  → render streaming   │
                   │                       │
                   │  PermBridgeMsg        │
                   │  → show approval      │
                   │  → user Y/N           │
                   │  → bridge to Source 1  │
                   └──────────────────────┘
```

**Three input paths** — all converge at the Inbox via `sendToAgent()`:
- **Source 1 (User)**: submit / command / modal response → if agent busy, queued until OnTurn drains
- **Source 2 (Agents)**: background agent completion / SendMessage / self-inject (hook blocked)
- **Source 3 (System)**: cron tick / async hook callback / file watcher

**Output path**: Outbox → TUI observes events for rendering. PermReq bridges back to Source 1 (approval → user decision → unblock agent).

**OnTurn feedback**: when the agent finishes a think+act cycle, the TUI drains all queued sources back into the Inbox, restarting the loop until all queues are empty.

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

### Agent Output (outbox → Model → View)

```
AgentOutboxMsg
  ├─ PreInfer  → conv.stream.active = true, commit pending
  ├─ OnChunk   → conv.AppendToLast(text, thinking)
  ├─ PostInfer → update token counts, set tool calls
  ├─ PreTool   → conv.stream.buildingTool = name
  ├─ PostTool  → applySideEffects, conv.Append(toolResult)
  ├─ OnTurn    → stop stream, commit, save session, fire hooks
  └─ OnStop    → cleanup agent session
```

### Cmd Chains (side effects)

`tea.Cmd` runs async, returns a `tea.Msg`, which feeds back into `Update()`. Chains form loops.

**Persistent loops** (run continuously while agent is active):

| Chain | Cmd → Msg → next Cmd | Location |
|-------|----------------------|----------|
| Outbox poll | `DrainAgentOutbox()` → `AgentOutboxMsg` → `DrainAgentOutbox()` | conv/runtime.go |
| Perm bridge | `PollPermBridge()` → `PermBridgeMsg` → `PollPermBridge()` | conv/runtime.go |
| Tick timers | `StartTicker()` → `TickMsg` → `StartTicker()` | notify/, trigger/ |

**Key one-shot chains**:
- **Submit**: `HandleSubmit()` → `sendToAgent()` → starts outbox + perm loops if first message
- **Turn end**: `OnTurn` → `drainTurnQueues()` → pops Source 1/2/3 queues → `sendToAgent()` or back to outbox loop
- **Tool exec**: `PreTool` → `ExecuteApproved()` → `ToolResultMsg` → back to agent

**Placement**: each sub-model owns its Cmd factories. Cross-cutting Cmds (`sendToAgent()`, `drainTurnQueues()`) live in root.

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

**Env**: app-local TUI state (`env`) lives in `app/env.go` — provider snapshot, permissions, plan, cache. Pure state holder with no singleton dependencies.

**Services**: domain service singletons (`services`) live in `app/services.go` — 14 interface-typed fields injected at model construction via `newServices()`. Model methods access services through `m.services.*`, never through package-level `Default()` calls at runtime. Sub-models never import services directly — they receive needed references through Deps structs that root assembles and passes in.

### Root Model

```go
type model struct {
    // Sub-models (one per event source / concern)
    userInput   input.Model      // Source 1: user keyboard → textarea, selectors, approval
    agentInput  notify.Model     // Source 2: background agent completion → notification queue
    systemInput trigger.Model    // Source 3: cron / async hook / file watcher → event queue
    conv        conv.Model       // Agent Outbox: outbox events → conversation state → chat render
    env         env              // App-local TUI state: provider, permissions, plan, cache
    services    services         // Domain service singletons, injected at construction

    // Infrastructure
    bgTracker     *notify.BackgroundTracker
    cwd           string
    isGit         bool
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
  └── renderModeStatus()   → status bar: mode, model, tokens
```

### Directory Structure

Each file is annotated with its MVU role: **[M]** model/state, **[U]** update, **[V]** view, **[C]** cmd (side effect factory). Files with multiple roles are listed left to right by importance.

```
internal/app/
│
│  ── Root: pure glue (8 files) ─────────────────────────────────────────────
│  No business logic. Model + services + routing + view + env + agent lifecycle + entrypoint + init.
│  Cross-cutting Cmds: sendToAgent(), drainTurnQueues(), triggerAutoCompact()
│
├── model.go      [M,C] Model{env, services, 4 sub-models, bgTracker}, Init()
│                       conv.Runtime event handlers, session persistence, turn queue drain, deps builders
├── agent.go        [C] Agent session lifecycle: delegates to services.Agent.
│                       buildAgentParams(), ensureAgentSession(), sendToAgent(), ContinueOutbox()
│                       ReconfigureAgentTool() wires subagent executor into tool registry.
├── services.go     [M] services: domain service singletons (14 fields), injected at model construction.
│                       newServices() snapshots all Default() calls. refreshAfterReload() re-snapshots
│                       the 5 services replaced by Initialize() during plugin reload.
├── env.go          [M] env: app-local TUI state only (provider, permissions, plan, cache).
│                       Pure state holder — no singleton service dependencies.
│                       Sub-models never import it — they receive needed state through Deps structs.
├── update.go       [U] Update(): msg type switch → delegate to sub-models
│                       Cross-cutting = routing: SubmitMsg → agent, PermReq → input, ...
├── view.go         [V] View(): compose sub-model views into terminal layout
├── run.go             Run(): tea.Program setup, entrypoint
├── init.go            Global infrastructure init, plugin/mcp adapters
│
│  ── input/ ── Source 1: User Input ────────────────────────────────────────
│  Event: tea.KeyMsg
│  Flow:  keyboard → textarea | selector | approval
│         Enter → SubmitMsg → root routes to agent
│         /cmd  → CommandMsg → root dispatches effect
│
├── input/
│   ├── model.go              [M] Model{Textarea, History, Images, Queue, Selectors}
│   ├── update.go             [U] Update(): routes to active overlay or textarea
│   ├── view.go               [V] RenderTextarea(), image indicators
│   ├── runtime.go            [M] OverlayDeps struct for overlay handler dependency injection
│   ├── submit.go             [C] HandleSubmit(), DrainInputQueue(), ExecuteSubmitRequest()
│   ├── command_controller.go [C] CommandController: slash command dispatch + all /cmd handlers
│   ├── approval_flow.go      [C] HandlePermissionRequest(), DispatchPermissionHookAsync()
│   ├── prompt_suggestion.go  [C] StartPromptSuggestion(), SuggestPromptCmd()
│   ├── on_textarea.go      [U,C] HandleTextareaUpdate(), HandleSuggestionKey()
│   ├── on_queue.go          [M,U] Queue: Enqueue(), Dequeue(), selection state
│   ├── on_image.go            [U] HandleImageSelectKey()
│   ├── on_approval.go      [M,U,V] ApprovalModel: Show() → HandleKeypress()
│   ├── on_approval_bash.go     [V] bash command preview
│   ├── on_approval_diff.go     [V] file diff preview
│   ├── on_agent.go          [M,U,V] AgentSelector
│   ├── on_provider.go       [M,U,V,C] ProviderSelector + connect/auth Cmds
│   ├── on_plugin.go         [M,U,V,C] PluginSelector + install/sync Cmds
│   ├── on_mcp.go            [M,U,V,C] MCPSelector + connect/reconnect Cmds
│   ├── on_session.go        [M,U,V,C] SessionSelector + load/fork Cmds
│   ├── on_memory.go         [M,U,V,C] MemorySelector + editor Cmds
│   ├── on_skill.go          [M,U,V] SkillSelector
│   ├── on_search.go         [M,U,V,C] SearchSelector
│   ├── on_tool_selector.go  [M,U,V] ToolSelector
│   └── on_token_limits.go       [C] Token limit fetch Cmd
│
│  ── notify/ ── Source 2: Background Agent Completion ──────────────────────
│  Event: task.TaskCompleted observer → NotificationQueue.Push()
│  Flow:  tick → PopReady() → BuildContinuationPrompt() → sendToAgent()
│  Cmd chain: StartTicker() → TickMsg → handleTick() → StartTicker() (loop)
│
├── notify/
│   ├── model.go              [M] Model{NotificationQueue}, Push(), Pop()
│   ├── update.go           [U,C] Update(), handleTick() → inject notification Cmd
│   ├── notification.go       [M] BuildTaskNotification(), MergeNotifications()
│   └── tracker.go          [M,C] Background batch/worker tracker, StartTicker()
│
│  ── trigger/ ── Source 3: System Events ───────────────────────────────────
│  Event: cron tick | async hook callback | file watcher
│  Flow:  event → queue → tick → sendToAgent()
│  Cmd chain: StartCronTicker() → TickMsg → handleCronTick() → StartCronTicker() (loop)
│
├── trigger/
│   ├── model.go              [M] Model{CronQueue, AsyncHookQueue}
│   ├── update.go           [U,C] Update(), handleCronTick(), StartCronTicker(), StartAsyncHookTicker()
│   └── file_watcher.go       [C] NewFileWatcher(), SetPaths(), poll()
│
│  ── conv/ ── Agent Outbox → Conversation ──────────────────────────────────
│  Event: core.Event from Agent Outbox channel
│  Flow:  DrainOutbox() → OutboxMsg → handleAgentEvent()
│         PreInfer → OnChunk → PostInfer → PreTool → PostTool → OnTurn
│  Cmd chains: DrainAgentOutbox() (loop), PollPermBridge() (loop)
│
├── conv/
│   ├── model.go              [M] Model{ConversationModel, OutputModel} composite
│   ├── update.go           [U,C] Update() entry, handleAgentEvent(): PreInfer/.../OnTurn dispatch
│   ├── runtime.go          [M,C] Runtime interface, AgentOutboxMsg, DrainAgentOutbox()
│   ├── view.go               [V] RenderMessageRange(), RenderActiveContent()
│   ├── conversation.go       [M] Append(), ConvertToProvider(), StreamState
│   ├── compact.go           [M,C] CompactState, CompactCmd()
│   ├── tool.go              [M,C] ToolExecState, ExecuteApproved()
│   ├── tool_render.go         [V] Tool result rendering
│   ├── modal.go               [M] ModalState type definitions
│   ├── plan.go              [M,U,V] PlanPrompt: Show() → PlanResponseMsg
│   ├── question.go          [M,U,V] QuestionPrompt: Show() → QuestionResponseMsg
│   ├── enterplan.go         [M,U,V] EnterPlanPrompt: Show() → EnterPlanResponseMsg
│   ├── message.go             [V] RenderAssistantMessage(), RenderUserMessage()
│   ├── markdown.go            [V] MDRenderer: Render()
│   ├── progress.go          [M,C] ProgressHub: SendForAgent(), Check()
│   ├── tracker_view.go        [V] RenderTrackerList(), task/batch/worker rendering
│   └── permission_bridge.go [M,C] PermissionBridge, PermBridgeMsg, PollPermBridge()
│
│  ── Shared ────────────────────────────────────────────────────────────────
│
└── kit/
    ├── suggest/             autocomplete engine
    ├── history/             input history
    ├── theme.go, styles.go  colors, lipgloss styles
    ├── listnav.go           list navigation helpers
    ├── editor.go            external editor integration
    └── msg.go, save_level.go, util.go
```

## Package Dependencies

```
cmd/gen/              CLI entrypoint
internal/app/         TUI layer (this document)
internal/core/        Agent interface: Inbox/Outbox/Run, Event types, Message
internal/llm/         LLM providers (Anthropic, OpenAI, Google, ...)
internal/tool/        Tool registry and execution
internal/hook/        Event hook system (depends on setting/ for env vars; LLM completer injected by app/)
internal/setting/     Settings and permissions
internal/...          ...
```

Dependency direction: `cmd/ → app/ → {core/, llm/, tool/, hook/, setting/, ...}`.
Domain packages never import `app/`.

**Lateral dependencies** (same-layer, documented):
- `hook/` → `setting/` (env var resolution)
- `session/` → `task/tracker` (serializes tracker tasks into transcripts)

**Decoupled via function injection** (no direct import):
- `hook/` ← `app/` — `hook.LLMCompleter` injected at init via `runtime.BuildHookCompleter`

**Decoupled via callback injection** (no direct import):
- `mcp/` ↔ `plugin/` — `mcp.Initialize(cwd, pluginServersCallback)`
- `subagent/` ↔ `plugin/` — `subagent.Initialize(cwd, pluginAgentPathsCallback)`


