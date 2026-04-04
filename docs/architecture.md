# GenCode 架构分析报告

## 阶段一：架构梳理

### 1. 模块职责表

| Package | 职责（一句话） | 主要依赖 |
|---------|--------------|---------|
| `internal/core` | 可复用的 Agent 主循环（Stream/Collect/ExecTool/FilterToolCalls） | client, tool, hooks, permission, system |
| `internal/app` | Bubble Tea TUI 顶层模型，事件分发与状态协调 | core, app/* 子包, provider, config |
| `internal/app/conversation` | 对话消息列表、流状态、Chunk 消息类型 | message |
| `internal/app/render` | 消息/工具/Markdown 渲染为终端字符串 | lipgloss, message |
| `internal/agent` | 子 Agent 执行器、注册表、AGENT.md 加载器、启用状态存储 | core, hooks, session, task, provider |
| `internal/provider` | LLM Provider 抽象（Anthropic/OpenAI/Google/Moonshot/Alibaba）+ 流式 API | message |
| `internal/client` | 对 provider 的薄包装，追踪 token 用量，提供 Complete/Stream 接口 | provider, message |
| `internal/tool` | 工具注册表（Read/Write/Bash/Agent/Skill 等）、Schema、执行 | message, permission |
| `internal/hooks` | Hook 引擎：按事件匹配并同步/异步执行 shell 命令 | config |
| `internal/config` | 多层级配置加载（user/project/local）、permissions、hook 配置解析 | — |
| `internal/session` | 会话 JSONL 持久化、元数据索引、历史加载 | message |
| `internal/system` | 系统提示词构建（指令/技能/Agent/记忆注入） | config |
| `internal/mcp` | MCP 协议实现：client、registry、stdio/HTTP/SSE transport | provider |
| `internal/task` | 后台任务管理（Agent 任务、Bash 任务）、进度追踪 | — |
| `internal/skill` | 技能注册表、加载（SKILL.md）、启用状态 | — |
| `internal/plan` | Plan 文件存储与命名 | — |
| `internal/permission` | 权限检查接口（Permit/Reject/Prompt） | — |
| `internal/cron` | Cron 调度器：解析表达式、定时触发 prompt | — |
| `internal/ui/*` | 共享 UI 组件：suggest、progress hub、theme、history | lipgloss |
| `internal/image` | 图片处理（剪贴板读取、base64 编码） | — |
| `internal/log` | DEV_DIR 层级日志，支持 Agent turn 追踪 | zap |
| `internal/plugin` | 插件系统：加载、安装、注册（Agent/MCP/Skill/Hook） | — |
| `internal/worktree` | Git worktree 隔离支持 | — |
| `internal/message` | 核心消息类型（Message/ToolCall/StreamChunk/CompletionResponse） | — |
| `internal/options` | CLI 运行时选项 | — |

---

### 2. TUI 层分析（internal/app/）

#### Model/Update/View 组织方式

```
internal/app/
├── model.go           — model 结构体定义 + Init + commitMessages
├── update.go          — Update(msg) 总分发路由
├── view.go            — View() 渲染入口（443 行）
├── run.go             — TUI 程序入口（tea.NewProgram）
├── handler_input.go   — 键盘事件处理（590 行）
├── handler_stream.go  — 流式响应处理（startLLMStream/waitForChunk）
├── handler_tool.go    — 工具执行调度（parallel/sequential）
├── handler_approval.go— 权限请求处理
├── handler_command.go — /slash 命令处理
├── handler_*.go       — 各功能域处理器
├── conversation/      — 对话状态（消息列表、流状态）
├── render/            — 渲染函数
└── {provider,session,skill,agent,mcp,plugin,memory,tool,mode}/
                       — 各功能域的 State + Model（selector/命令）
```

**model struct** 采用"Domain State"模式：每个功能域拥有自己的 `State` 结构体，内嵌进 `model`。Selector 组件（`*Model`）存放于对应 State 中。

#### Msg 类型和 Cmd 流转路径

```
tea.KeyMsg          → handleKeypress() → delegateToActiveModal / handleInputKey
tea.WindowSizeMsg   → update.go → m.width/height 更新 + resize
appconv.ChunkMsg    → handleStreamChunk() → AppendToLast / handleStreamDone
apptool.ExecResultMsg→ handleToolResult() → AddToolResult → startContinueStream
appprovider.SelectedMsg→ handleProviderSelected()
appsession.SelectedMsg → handleSessionSelected()
progress.UpdateMsg  → update progress display
cron.TickMsg        → check cron queue
```

#### 状态管理

| 状态域 | 所在结构 | 说明 |
|-------|---------|------|
| 对话消息 | `m.conv.Messages` | `[]ChatMessage`，含 role/content/toolCalls/toolResult |
| 流状态 | `m.conv.Stream` | `StreamState{Active, Ch, Cancel}` |
| 当前 Provider | `m.provider.LLM` | `provider.LLMProvider` |
| 工具执行 | `m.tool.ExecState` | `PendingCalls/CurrentIdx/Cancel` |
| 模式 | `m.mode` | `PlanMode/DisabledTools/Question/PlanApproval` |
| Selector 激活 | `m.{provider,session,skill,...}.Selector.active` | bool 字段 |

#### TUI Core Loop

```
用户输入事件（Bubble Tea 事件队列）
    ↓
Update(msg tea.Msg) → 分发到各 handler
    ↓                     ↑
  Cmd 返回          Model 状态更新
    ↓
tea.Cmd 执行（goroutine）
  └─ waitForChunk() 阻塞读 channel → ChunkMsg
  └─ ExecResultMsg goroutine → 工具完成消息
    ↓
View() 重渲染（由 Bubble Tea 自动调用）
    ↓
等待下一事件
```

---

### 3. Agent 层分析（internal/agent/ + internal/core/）

#### Agent Core Loop（core/core.go `Loop.Run`）

```
for {
  0. ctx.Done() 检查 → StopCancelled
  1. Pre-stream 紧凑检查（token 近上限 → compactAndReplace）
  2. l.Stream(ctx) → <-chan StreamChunk
     Collect() 同步收集 → CompletionResponse
  3. AddResponse() → 追加 assistant 消息
  4. DecideCompletion(stopReason, toolCalls):
     ├─ CompletionRecoverMaxTokens → 注入恢复 prompt，continue
     ├─ CompletionStopMaxOutputRecovery → terminate(exhausted)
     ├─ CompletionEndTurn → 执行 Stop hooks → terminate(end_turn)
     └─ CompletionRunTools:
        ├─ FilterToolCalls() → hooks pre-check（blocked/allowed）
        ├─ for each allowed: ExecTool() → AddToolResult()
        └─ firePostToolHook()
  5. turnCount >= maxTurns → terminate(max_turns)
  6. 继续下一轮
}
```

#### 循环终止条件

| 终止原因 | StopReason |
|---------|-----------|
| 正常结束（无 tool call） | `end_turn` |
| Stop hook 阻断 | `stop_hook` |
| 达到最大轮次 | `max_turns` |
| 上下文取消（用户中断） | `cancelled` |
| max_output_tokens 恢复耗尽 | `max_output_recovery_exhausted` |

#### Tool 调度

```go
ExecTool(ctx, tc):
  1. ParseToolInput(tc.Input) → params map
  2. Permission.Check(name, params) → Permit/Reject/Prompt
  3. Reject → ErrorResult; Permit/Prompt → runTool()
  4. runTool():
     a. tool.Get(name) → 内置工具 → ExecutePreparedTool()
     b. MCP.IsMCPTool(name) → MCPCaller.CallTool()
```

**并发模式**：`core.Loop.Run` 中工具**串行**执行（TUI 层的并行由 `apptool.ExecuteParallel` 负责）。

#### 流式响应处理

```
provider.Stream() → goroutine → chan StreamChunk
  emit: ChunkTypeText / ChunkTypeThinking / ChunkTypeToolStart / ChunkTypeToolInput / ChunkTypeDone
  ↓（对于同步 agent）
core.Collect() 同步消费
  ↓（对于 TUI）
waitForChunk() tea.Cmd 异步消费，每 chunk 产生一个 Update 周期
```

#### 子 Agent

- 通过 `agent.Executor.Run()` 运行，底层复用 `core.Loop`
- **共享上下文**：userInstructions、projectInstructions、isGit、mcpGetter、hookEngine
- **独立上下文**：自有 system prompt（来自 AGENT.md）、自有 permission mode、自有 tool set
- **生命周期**：前台 agent 在调用方 goroutine 阻塞；后台 agent 由 `RunBackground()` 新建 goroutine，通过 `task.AgentTask` 追踪

---

### 4. 两个 Loop 的交汇

#### TUI 如何触发 Agent 执行

```
handleSubmit() / handleCommandSubmit()
    ↓
handleStartLLMStream()
    → m.loop.Stream(ctx)  // 返回 <-chan StreamChunk，不阻塞
    → waitForChunk() 作为 tea.Cmd 返回
```

工具执行（触发子 Agent）：
```
handleStartToolExecution(toolCalls)
    → apptool.ExecuteParallel(ctx, toolCalls, ...)  // 独立 goroutine
      → tool.ExecutePreparedTool() → agent.Executor.Run()
      → 完成后发 ExecResultMsg 到 TUI
```

#### Agent 流式输出回传 TUI

```
m.loop.Stream(ctx) 返回 <-chan StreamChunk
    ↓
waitForChunk() = func() tea.Msg { chunk := <-ch; return convertChunkToMsg(chunk) }
    ↓ （Bubble Tea 将 ChunkMsg 注入 Update）
handleStreamChunk(ChunkMsg)
    → AppendToLast(text, thinking)  // 更新当前消息内容
    → return tea.Batch(waitForChunk(), spinner.Tick)
```

#### 用户中断（Ctrl+C / Esc）如何传播

```
Ctrl+C → handleInputKey:
  m.conv.Stream.Cancel()  // 取消 provider goroutine 的 context
  m.tool.Cancel()         // 取消正在执行的工具 goroutine
  fireSessionEnd()
  tea.Quit

Esc（流进行中）→ handleStreamCancel():
  m.conv.Stream.Cancel()  // provider goroutine 退出，channel 关闭
  cancelPendingToolCalls()
  m.conv.MarkLastInterrupted()
  commitMessages()
```

---

### 5. 核心数据流（端到端）

```
① 用户在终端输入 "帮我读取 main.go"，按 Enter
      ↓
② TUI（tea.KeyMsg/KeyEnter）→ handleSubmit()
      ↓
③ handleCommandSubmit() 未匹配 → handleStartLLMStream()
   m.loop.SetMessages(conv.ToProvider())
   ch := m.loop.Stream(ctx)   ← core.Loop.Stream() → client.Stream() → provider.Stream()
   return waitForChunk()
      ↓
④ LLM API 流式返回（Anthropic SSE）
   provider goroutine 解析 → StreamChunk{Type:text/tool_start/done}
      ↓
⑤ waitForChunk() 读取 chunk → ChunkMsg → Update(ChunkMsg)
   handleStreamChunk() → AppendToLast() → View() 实时显示流式文字
      ↓
⑥ ChunkTypeDone → handleStreamDone() → DecideCompletion()
   → CompletionRunTools（有 Read tool call）
      ↓
⑦ handleStartToolExecution([ToolCall{name:"Read", input:{path:"main.go"}}])
   ExecuteParallel() → goroutine → tool.ExecutePreparedTool(Read)
      ↓
⑧ Read 工具读取文件，返回内容 → ExecResultMsg
      ↓
⑨ handleToolResult() → conv.AppendToolResult()
   → startContinueStream() → 重新 Stream（携带工具结果）
      ↓
⑩ LLM 再次响应（使用文件内容）→ 重复 ④-⑤
   最终无 tool call → CompletionEndTurn → commitMessages()
      ↓
⑪ tea.Println() 将渲染后的消息追加到终端滚动区
```

---

## 阶段二：问题识别

| 文件路径 | 问题类型 | 描述 | 影响程度 |
|---------|---------|------|---------|
| `internal/app/handler_input.go:149-169` | 重复代码 | `m.conv.Stream.Cancel() + m.tool.Cancel() + fireSessionEnd() + tea.Quit` 在 CtrlC、CtrlD、"exit" 三处重复，且 "exit" 分支遗漏 `m.tool.Cancel()` | 高 |
| `internal/app/handler_input.go:254-277` | 重复代码 | `delegateToActiveModal` 中 8 个完全同构的 `if m.X.Selector.IsActive() { return true, m.X.Selector.HandleKeypress(msg) }` 块，无法利用 Go 接口抽象 | 高 |
| `internal/config/permission.go` | 文件过大/职责混杂 | 894 行，混合：权限规则解析、工具名通配匹配、测试辅助数据；单一文件承担 config 解析 + 运行时匹配两个职责 | 高 |
| `internal/session/store.go` | 文件过大 | 882 行，混合：JSONL 读写、元数据索引构建、消息格式转换、会话搜索；单 Store 类型职责过重 | 中 |
| `internal/app/plugin/model.go` | 文件过大/职责混杂 | 826 行，混合：selector 状态机、渲染（render.go 另 607 行）、命令处理；TUI Model 与渲染逻辑未分离 | 中 |
| `internal/app/mcp/model.go` | 文件过大/职责混杂 | 812 行，混合：MCP selector 状态、服务器详情渲染、命令执行（commands.go 另 473 行） | 中 |
| `internal/app/render/message_tool.go` | 文件过大 | 600 行，包含 15+ 个不相关的格式化函数（AgentLabel、ToolArgs、ByteSize、LineCount 等），应按渲染对象分拆 | 中 |
| `internal/hooks/engine.go` | 文件过大/函数过长 | 608 行，`executeCommandBidirectional`（约 120 行）处理 stdin/stdout 双向通信，逻辑密集；`getMatchingHooks` + `extractCommands` 逻辑分散 | 中 |
| `internal/agent/executor.go:443-499` | 函数过长 | `buildSystemPrompt` 56 行，`formatToolProgress` 21 行；整体 executor.go 603 行，`prepareRunConfig` 含 model 解析、权限设置、display name 三个子职责 | 中 |
| `internal/app/handler_input.go` | 函数职责混杂 | `handleInputKey`（117 行）同时处理图片键、建议键、历史、提交、退出，职责过多；但已通过子函数部分分解 | 低 |
| `internal/app/model.go:configureLoop` | 职责扩散 | `configureLoop` 57 行，同时构建 client/system/tool 三个对象；随功能增长持续膨胀 | 低 |
| 跨文件 | 命名不一致 | `Model`/`State` 命名在不同子包语义不同：有些 `Model` 是 selector（provider），有些是命令面板（mcp/plugin） | 低 |

---

## 阶段三：重构方案

### 1. [P0] 提取 `quitWithCancel()` 消除三处重复退出序列

**现状**：`handler_input.go` 中 CtrlC、CtrlD、"exit" 三处各自执行相同的 cancel+quit 序列，且 "exit" 分支遗漏了 `m.tool.Cancel()`。

**目标**：单一退出入口，行为一致，消除隐性 bug。

**做法**：在 `handler_input.go` 中添加私有方法：
```go
// quitWithCancel cancels any active stream and tool execution before quitting.
func (m *model) quitWithCancel() (tea.Cmd, bool) {
    if m.conv.Stream.Cancel != nil {
        m.conv.Stream.Cancel()
    }
    if m.tool.Cancel != nil {
        m.tool.Cancel()
    }
    m.fireSessionEnd("prompt_input_exit")
    return tea.Quit, true
}
```
CtrlC/CtrlD 分支替换为 `return m.quitWithCancel()`；`handleSubmit` 的 "exit" 分支替换为 `return m.quitWithCancel()` 并去掉 `true`（handleSubmit 返回单 Cmd）。

---

### 2. [P1] 用接口切片替换 `delegateToActiveModal` 中 8 个同构 selector 分支

**现状**：8 个完全相同结构的 `if m.X.Selector.IsActive()` 块，每新增 selector 都需手动添加。

**目标**：利用 Go 接口，一次遍历完成分发，扩展零成本。

**做法**：在 `handler_input.go` 中定义接口并收集：
```go
type selectorDispatcher interface {
    IsActive() bool
    HandleKeypress(tea.KeyMsg) tea.Cmd
}

func (m *model) selectorDispatchers() []selectorDispatcher {
    return []selectorDispatcher{
        &m.provider.Selector,
        &m.tool.Selector,
        &m.skill.Selector,
        &m.agent.Selector,
        &m.mcp.Selector,
        &m.plugin.Selector,
        &m.session.Selector,
        &m.memory.Selector,
    }
}
```

`delegateToActiveModal` 末尾 8 个 if 块替换为：
```go
for _, sel := range m.selectorDispatchers() {
    if sel.IsActive() {
        return true, sel.HandleKeypress(msg)
    }
}
```

---

### 3. [P1] 拆分 `internal/config/permission.go`

**现状**：894 行，混合配置解析（`PermissionRules`、`HookConfig`）与运行时工具名匹配逻辑。

**目标**：配置数据类型与匹配逻辑分文件，便于独立测试。

**做法**：
- `permission.go` → 保留类型定义和加载（`PermissionRules`、`Allow/Deny` 规则解析）
- 新建 `permission_matcher.go` → 提取 `matchToolName()`、`GlobMatch()`、权限检查运行时逻辑

---

### 4. [P1] 提取 `session/store.go` 的格式转换职责

**现状**：882 行，`Store` 承担 JSONL IO + 元数据索引 + 消息格式转换三个职责。

**目标**：Store 只负责持久化，格式转换独立。

**做法**：
- `store.go` → 保留 CRUD（`Save/Load/Delete/List`）
- 新建 `convert.go` → 提取 `ConvertToProvider()`、`BuildConversationText()` 等转换函数

---

### 5. [P2] 拆分 `internal/app/render/message_tool.go`

**现状**：600 行，15+ 个不相关渲染函数混在一起。

**目标**：按渲染对象分组，提高可导航性。

**做法**：
- `render_agent.go` → `FormatAgentLabel`、`formatAgentDefinition`、`buildDoneStats`
- `render_size.go` → `FormatToolResultSize`、`formatByteSize`、`formatLineCount`
- `render_tool.go` → `RenderToolCalls`、`RenderToolResultInline`、`ExtractToolArgs`
- `message_tool.go` → 保留 task/skill 相关渲染
