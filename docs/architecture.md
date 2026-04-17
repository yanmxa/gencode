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
6. **Project structure mirrors the architecture:**

```
app/
  model.go     root Model definition + Init
  update.go    Update dispatch (routes by msg type)
  view.go      View layout composition
  command.go   tea.Cmd factory functions
  run.go       entrypoint / lifecycle
  user/        user input sub-model (keyboard → state → textarea)
  output/      agent outbox sub-model (outbox events → state → chat rendering)
```

Best Practices :
1. sub-model分治：把 Model 拆成多个子 model，每个子 model 有自己的 Update() 和 View()。父 model 做路由分发
2. 消息即事件，用类型作路由：类型越精准，update 里面分支月干净
3. Cmd是唯一的副作用出口：所有的 I/O都通过tea.cmd触发，每次收到一个事件，返回一个新的listen Cmd,形成链式监听
4. View 椿萱软，不持有状态
5. 状态机管理模式切换；用显示state mode枚举控制ui行为，update, view都根据mode分支，避免一堆bool flag组合爆炸
6.  实际项目结构建议

  app/
    model.go          # 主 model 定义 + Init/Update/View 入口
    update.go         # Update 路由（按 msg 类型分发）
    command.go        # 所有 tea.Cmd 工厂函数
    run.go            # 启动/lifecycle
    output/           # 各类输出渲染（View 侧）
    user/             # 用户输入处理（Update 侧）

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

Five sub-Models — four mutation sources + shared runtime. Each sub-Model's
fields are defined in its own `model.go`; the root model only holds references.

```go
type model struct {
    userInput   user.Model      // user/model.go
    agentInput  agent.Model     // agent/model.go
    systemInput system.Model    // system/model.go
    agentOutput output.Model    // output/model.go
    runtime     runtime.Model   // runtime/model.go
}
```

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

### Directory Structure

Organized by **input source** (who triggers mutation) and **responsibility**. Sub-packages stay flat. `on_` prefix for component files in input-source packages (`user/`, `agent/`, `system/`).

```
internal/app/
│
│  ── Core MVU ──────────────────────────────────
├── model.go              # root Model (5 sub-model refs), Init()
├── update.go             # Update() top-level dispatch
├── view.go               # View() layout composition
├── init.go               # infrastructure init (newModel, provider, hooks)
│
│  ── Cross-cutting (spans multiple sub-models) ─
├── bridges.go            # Runtime adapters: root → sub-model interfaces
├── output_adapter.go     # output.Runtime implementation
├── submit.go             # user submit → agent pipeline
├── command.go            # slash command registry
├── mode.go               # mode switching (plan/auto-accept/bypass)
├── lifecycle.go          # cwd change, config reload, memory refresh
├── agent_config.go       # LLM loop + agent tool wiring
├── approval.go           # cross-sub-model permission approval
├── tool_exec.go          # tool execution dispatch
├── run.go                # entrypoint, TUI program setup
├── runprint.go           # headless print mode
│
├── user/                 # Source 1: User Input
│   ├── model.go          #   textarea, history, images, queue, selectors
│   ├── update.go         #   overlay message routing
│   ├── view.go           #   textarea + image rendering
│   ├── on_textarea.go    #   text input, history, suggestion
│   ├── on_queue.go       #   message queue (mid-stream buffer)
│   ├── on_image.go       #   image paste
│   ├── on_approval.go    #   tool approval dialog
│   ├── on_approval_bash.go, on_approval_diff.go
│   ├── on_agent.go       #   agent selector
│   ├── on_mcp.go         #   MCP server selector
│   ├── on_memory.go      #   memory editor
│   ├── on_plugin.go, on_plugin_command.go, on_plugin_view.go
│   ├── on_provider.go, on_provider_view.go
│   ├── on_search.go, on_session.go, on_skill.go
│   └── on_token_limits.go
│
├── agent/                # Source 2: Agent Input
│   ├── model.go          #   notification queue, batch tracking
│   ├── update.go         #   tick → notification routing
│   ├── on_notification.go
│   └── on_tracker.go     #   background worker/batch tracking
│
├── system/               # Source 3: System Input
│   ├── model.go          #   cron queue, async hook queue
│   ├── update.go         #   tick → handler routing
│   ├── view.go           #   hook status rendering
│   └── on_file_watcher.go
│
├── output/               # Agent Output (outbox → Model → View)
│   ├── model.go          #   conv, modal, tool state, spinner, progress
│   ├── update.go         #   outbox event dispatch
│   ├── runtime.go        #   Runtime interface definition
│   ├── view.go           #   message + streaming + tool result rendering
│   ├── on_conversation.go, on_compact.go
│   ├── on_modal.go, on_modal_plan.go, on_modal_question.go, on_modal_enterplan.go
│   ├── on_tool.go, on_progress.go
│   ├── on_message.go, on_markdown.go
│   └── permission_bridge.go
│
├── runtime/              # Shared Runtime State
│   └── model.go          #   provider, session, permission, plan, config
│
└── kit/                  # Shared UI Utilities
    ├── suggest/, history/
    ├── theme.go, theme_selector.go, styles.go
    ├── listnav.go, editor.go, msg.go, save_level.go, util.go
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
