# Singleton Pattern

## First Principle

Every domain module is a **service** — an isolated, stateful unit with a clear contract.
The app layer consumes services through interfaces, never through concrete types or globals.

Rules:

1. One service, one interface, one package.
2. All interfaces are named `Service` within their package: `hook.Service`, `session.Service`, etc.
3. All singletons are accessed via `Default()` returning the interface type.
4. All initialization takes an `Options` struct.
5. The app `model` holds service references (injected at construction), never calls `Default()` at runtime.
6. Services never import each other. Cross-service deps are injected as interface fields in `Options`.

## Service Contract Template

```go
package xxx

import "sync"

// Service is the public contract for this module.
type Service interface {
    // grouped by concern, documented per method
}

// Options holds all dependencies for initialization.
// Cross-service deps are injected as interfaces or callbacks.
type Options struct {
    CWD string
    // ...
}

// ── singleton ──────────────────────────────────────────────

var (
    mu       sync.RWMutex
    instance Service
)

func Initialize(opts Options) error {
    s := &service{/* build from opts */}
    mu.Lock()
    instance = s
    mu.Unlock()
    return nil
}

func Default() Service {
    mu.RLock()
    s := instance
    mu.RUnlock()
    if s == nil {
        panic("xxx: not initialized")
    }
    return s
}

func SetDefault(s Service)  { mu.Lock(); instance = s; mu.Unlock() }  // test
func Reset()                { mu.Lock(); instance = nil; mu.Unlock() } // test

// ── implementation ─────────────────────────────────────────

type service struct {
    mu sync.RWMutex
    // internal state
}
```

## Service Inventory

```
┌──────────────────────────────────────────────────────────────────┐
│                        core/ (pure types)                        │
│  Agent, LLM, Tools, System, Message, Event, ToolSchema           │
│  No singleton. No state. Foundation types only.                  │
└──────────────────────────────────────────────────────────────────┘
       │
       │  (types only, no runtime dep)
       ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Domain Services (14 modules)                   │
│                                                                  │
│  setting.Service      LLM/provider config, permission rules      │
│  llm.Service          Provider registry, current connection      │
│  tool.Service         Tool registration, schema, execution       │
│  plugin.Service       Plugin lifecycle, component resolution     │
│  hook.Service         Event hooks, sync/async execution          │
│  session.Service      Session lifecycle, transcript persistence  │
│  skill.Service        Skill loading, enable/disable              │
│  subagent.Service     Agent definitions, executor factory        │
│  mcp.Service          MCP server connections, tool proxying      │
│  command.Service      Slash commands, custom command loading     │
│  task.Service         Background task lifecycle                  │
│  cron.Service         Scheduled job storage, tick/fire           │
│  orchestration.Service  Worker/batch tracking                    │
│  agent.Service        Agent session build/start/stop/send        │
└──────────────────────────────────────────────────────────────────┘
       │
       │  injected at model construction
       ▼
┌──────────────────────────────────────────────────────────────────┐
│                      app/ model (consumer)                       │
│                                                                  │
│  type model struct {                                             │
│      // sub-models                                               │
│      userInput   input.Model                                     │
│      agentInput  notify.Model                                    │
│      systemInput trigger.Model                                   │
│      conv        conv.Model                                      │
│                                                                  │
│      // services (injected, interface-typed)                     │
│      services    Services                                        │
│                                                                  │
│      // app-local state only                                     │
│      env         Env                                             │
│  }                                                               │
└──────────────────────────────────────────────────────────────────┘
```

## App-Layer Integration

### Services bundle

```go
// app/services.go

type Services struct {
    Setting       setting.Service
    LLM           llm.Service
    Tool          tool.Service
    Plugin        plugin.Service
    Hook          hook.Service
    Session       session.Service
    Skill         skill.Service
    Subagent      subagent.Service
    MCP           mcp.Service
    Command       command.Service
    Task          task.Service
    Cron          cron.Service
    Orchestration orchestration.Service
    Agent         agent.Service
}

func newServices() Services {
    return Services{
        Setting:       setting.Default(),
        LLM:           llm.Default(),
        Tool:          tool.Default(),
        Plugin:        plugin.Default(),
        Hook:          hook.Default(),
        Session:       session.Default(),
        Skill:         skill.Default(),
        Subagent:      subagent.Default(),
        MCP:           mcp.Default(),
        Command:       command.Default(),
        Task:          task.Default(),
        Cron:          cron.Default(),
        Orchestration: orchestration.Default(),
        Agent:         agent.Default(),
    }
}
```

### Model construction

```go
// app/model.go

func newModel(opts setting.RunOptions) (*model, error) {
    m := &model{
        services: newServices(),
        env:      newEnv(),
        cwd:      appCwd,
    }
    return m, nil
}
```

### Usage in model

```go
m.services.Hook.Execute(ctx, hook.PreToolUse, input)
m.services.LLM.Provider().Stream(...)
m.services.Setting.AllowBypass()
m.services.Session.ID()
m.services.Agent.Start(messages)
m.services.Agent.Send(content, images)
```

### Env after refactor

Only app-local TUI state remains:

```go
type Env struct {
    OperationMode      setting.OperationMode
    SessionPermissions *setting.SessionPermissions
    PlanEnabled        bool
    PlanTask           string
    PlanStore          *plan.Store
    InputTokens        int
    OutputTokens       int
    ThinkingLevel      llm.ThinkingLevel
    ThinkingOverride   llm.ThinkingLevel
    FileCache          *filecache.Cache
    CachedUserInstructions    string
    CachedProjectInstructions string
}
```

## Initialization Flow

```
app/init.go: initInfrastructure()

  1. setting.Initialize(setting.Options{CWD: cwd})
  2. llm.Initialize(llm.Options{})
  3. plugin.Initialize(plugin.Options{CWD: cwd})
  4. tool.Initialize(tool.Options{CWD: cwd})
  5. skill.Initialize(skill.Options{CWD: cwd})
  6. command.Initialize(command.Options{
         CWD:              cwd,
         DynamicProviders: [...],
     })
  7. session.Initialize(session.Options{CWD: cwd})
  8. mcp.Initialize(mcp.Options{
         CWD:           cwd,
         PluginServers: plugin.Default().Servers,
     })
  9. subagent.Initialize(subagent.Options{
         CWD:              cwd,
         PluginAgentPaths: plugin.Default().AgentPaths,
     })
 10. hook.Initialize(hook.Options{
         Settings:       setting.Default().Snapshot(),
         SessionID:      session.Default().ID(),
         CWD:            cwd,
         TranscriptPath: session.Default().TranscriptPath(),
         Completer:      buildHookCompleter(llm.Default().Provider()),
         ModelID:        llm.Default().ModelID(),
         EnvProvider:    plugin.Default().Env,
     })
 11. task.Initialize(task.Options{OutputDir: ...})
 12. cron.Initialize(cron.Options{StoragePath: ...})
 13. orchestration.Initialize(orchestration.Options{})
 14. agent.Initialize(agent.Options{})

app/model.go: newModel()

  services := newServices()   // snapshot all Default() into Services struct
  m := &model{services: services, ...}
```

## Interface Definitions

### `setting.Service`

```go
type Service interface {
    // read
    Snapshot() *Settings                 // current merged settings (cloned)
    AllowBypass() bool                   // whether bypass mode is permitted
    IsGitRepo(cwd string) bool           // convenience check

    // permission
    CheckToolPermission(name string, args map[string]any, sp *SessionPermissions) perm.Decision

    // lifecycle
    Reload() error                       // re-read all settings files
}
```

### `llm.Service`

```go
type Service interface {
    // connection
    Provider() Provider                  // current active provider
    SetProvider(p Provider)              // switch provider
    ModelID() string                     // current model identifier
    CurrentModel() *CurrentModelInfo     // full model metadata
    TokenLimits() (input, output int)    // context window bounds

    // factory
    NewClient(model string, maxTokens int) *Client

    // registry
    ListProviders() []Meta               // all registered providers
    Status() Status                      // connection status
}
```

### `tool.Service`

```go
type Service interface {
    // registry
    Register(t Tool)                     // add a tool
    Get(name string) Tool                // lookup by name
    All() []Tool                         // all registered tools
    Schemas() []core.ToolSchema          // schemas for LLM

    // execution
    Execute(ctx context.Context, name string, params map[string]any, cwd string) (string, error)
}
```

### `plugin.Service`

```go
type Service interface {
    // query
    List() []Plugin                      // all loaded plugins
    Get(name string) (*Plugin, bool)     // lookup by name

    // install
    Install(ctx context.Context, name string) error
    Uninstall(name string) error
    Sync(ctx context.Context) error      // sync all plugins

    // cross-domain (consumed by other services at init)
    AgentPaths() []AgentPath             // paths for subagent loader
    Servers() []ServerDescriptor         // MCP server descriptors
    Env(key string) string               // plugin env var resolution
    HooksConfig() []*HooksConfig         // hook definitions from plugins
}
```

### `hook.Service`

```go
type Service interface {
    // execution
    Execute(ctx context.Context, event EventType, input HookInput) HookOutcome
    ExecuteAsync(event EventType, input HookInput)
    FilterToolCalls(ctx context.Context, calls []ToolCallInput) FilterToolCallsResult

    // query
    HasHooks(event EventType) bool       // whether any hooks registered for event
    StopHookActive() bool                // whether a Stop hook is currently running

    // reconfigure (after session/provider change)
    SetSettings(s *setting.Settings)
    SetCompleter(c LLMCompleter, modelID string)
    SetSession(sessionID, transcriptPath string)
    SetCwd(cwd string)

    // session-scoped hooks
    AddSessionHook(event EventType, matcher string, fn FunctionHookCallback) string
    RemoveSessionHook(event EventType, hookID string) bool
    ClearSessionHooks()
}
```

### `session.Service`

```go
type Service interface {
    // identity
    ID() string                          // current session ID
    TranscriptPath() string              // path to transcript file
    Summary() string                     // compaction summary

    // persistence
    Save(snap *Snapshot) error           // persist session
    Load(id string) (*Snapshot, error)   // load by ID
    LoadLatest() (*Snapshot, error)      // load most recent
    List() ([]SessionMetadata, error)    // list all sessions

    // lifecycle
    New() string                         // create new session, return ID
    Fork(id string) (*Snapshot, error)   // fork existing session
    Clear()                              // reset current session

    // state mutation
    SetID(id string)                     // update current session ID
    SetSummary(summary string)           // update compaction summary
    SaveMemory(id, memory string) error  // persist session memory
    LoadMemory(id string) (string, error)
}
```

### `skill.Service`

```go
type Service interface {
    // query
    List() []Skill                       // all loaded skills
    Get(name string) (*Skill, bool)      // lookup by name
    IsEnabled(name string) bool          // check if enabled

    // mutation
    SetEnabled(name string, enabled bool, userLevel bool) error
    GetDisabledAt(userLevel bool) map[string]bool

    // system prompt
    PromptSection() string               // rendered section for system prompt
}
```

### `subagent.Service`

```go
type Service interface {
    // query
    ListConfigs() []AgentConfig          // all agent type definitions
    GetConfig(name string) (*AgentConfig, bool)
    IsEnabled(name string) bool
    GetDisabledAt(userLevel bool) map[string]bool

    // mutation
    SetEnabled(name string, enabled bool, userLevel bool) error

    // factory
    NewExecutor(deps ExecutorDeps) *Executor

    // system prompt
    PromptSection() string               // rendered section for system prompt
}
```

### `mcp.Service`

```go
type Service interface {
    // connection
    ListServers() []Server               // all configured servers
    Connect(ctx context.Context, name string) error
    ConnectAll(ctx context.Context) error
    Disconnect(name string) error
    Reconnect(ctx context.Context, name string) error

    // tools
    ListTools() []MCPTool                // tools from all connected servers
    NewCaller() *Caller                  // create execution caller

    // config
    EditConfig() (*EditInfo, error)      // open config for editing
    SaveConfig(info *EditInfo) error     // save edited config
}
```

### `command.Service`

```go
type Service interface {
    // query
    Get(name string) (*Info, bool)       // lookup by name
    List() []Info                        // all commands
    ListCustom() []CustomCommand         // user-defined commands
    GetMatching(prefix string) []Info    // autocomplete
}
```

### `task.Service`

```go
type Service interface {
    // lifecycle
    Launch(t BackgroundTask) error       // start a background task
    Get(id string) (BackgroundTask, bool)
    List() []TaskInfo                    // all task statuses
    Stop(id string) error                // graceful stop
    Kill(id string) error                // force kill

    // observer
    SetObserver(obs CompletionObserver)  // lifecycle notification callback
}
```

### `cron.Service`

```go
type Service interface {
    // CRUD
    Add(job Job) error
    Remove(id string) bool
    List() []Job

    // runtime
    Tick(now time.Time) []FiredJob       // advance clock, return fired jobs

    // lifecycle
    Reset()
}
```

### `orchestration.Service`

```go
type Service interface {
    // tracking
    RecordLaunch(launch Launch) string
    RecordCompletion(workerID, result string)

    // messaging
    QueueMessage(workerID, msg string)
    DequeueMessage(workerID string) (string, bool)

    // snapshot
    Workers() []WorkerSnapshot
    Batches() []BatchSnapshot

    // lifecycle
    Reset()
}
```

### `agent.Service`

```go
type Service interface {
    // lifecycle
    Start(messages []core.Message) (<-chan core.Event, error)  // build + run agent, return outbox
    Stop()                                                     // stop agent goroutine
    Active() bool                                              // whether an agent session is running

    // communication
    Send(content string, images []core.Image)                  // push message to inbox
    Outbox() <-chan core.Event                                  // outbox channel for TUI drain

    // permission bridge
    PermissionBridge() *PermissionBridge                        // bridge for TUI approval flow
    SetPendingPermission(req *PermBridgeRequest)                // track pending approval

    // reconfigure
    Reconfigure(opts AgentBuildOptions)                         // rebuild tools/system prompt
}
```

## Migration Phases

```
Phase 1              Phase 2            Phase 3             Phase 4
Unify Accessor       Define Interface   Inject into Model   Clean Env
──────────────       ────────────────   ─────────────────   ──────────
DefaultXxx           type Service       Services struct     remove domain
    │                interface {..}     injected at         fields from Env
    ▼                    │              model construction
Default() returns        ▼                  │
concrete type        Default() returns      ▼
                     Service            m.services.Xxx
    │                    │              replaces all
    ▼                    ▼              Default() calls
mechanical           forces clean       in model code
per-package          API boundary
```

## Parallel Workers

```
 ┌────────────────────────────────────────────────────────────────────┐
 │  Worker 1          Worker 2          Worker 3          Worker 4   │
 │  setting           llm               plugin            tool       │
 │  skill             session           command           mcp        │
 │                                                                    │
 │  (each worker handles 2 packages)                                 │
 └──────────────────────────────┬─────────────────────────────────────┘
                                │ all service interfaces defined
                                ▼
 ┌────────────────────────────────────────────────────────────────────┐
 │  Worker 5          Worker 6          Worker 7                     │
 │  hook              subagent          agent                        │
 │  task, cron,       (heaviest deps)   (agent session)              │
 │  orchestration                                                    │
 └──────────────────────────────┬─────────────────────────────────────┘
                                │ all singletons migrated
                                ▼
                   ┌────────────────────────┐
                   │  Worker 8              │
                   │  app/ Services struct  │
                   │  + flatten Env         │
                   └────────────────────────┘
```
