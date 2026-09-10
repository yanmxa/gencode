---
package: github.com/genai-io/san/internal/app
layer: app
---

# ui

The Bubble Tea TUI shell. Composes the agent loop, conversation view,
user input, hub-routed agent events, system triggers (cron / async
hook / file watcher), and runtime services into a single `tea.Model`.

## Purpose

This is the only `app`-layer package: everything user-facing lives here.
It owns nothing domain-specific — every business behavior comes from a
`feature`-layer service injected through the `services` struct. The
package's job is **composition and event routing**, not behavior.

## Contract

There is no single `Service` interface — `app` is the top of the layer
stack and nothing else imports it. Instead the contract is the
**sub-model `Runtime` interfaces** that each subpackage exposes upward
to the root model. The root implements them through adapter methods.

```go
package app

// model is the Bubble Tea root model. One per process.
type model struct {
    userInput   input.Model      // Source 1: user keyboard
    eventHub    *hub.Hub         // Source 2: agent-to-agent pub/sub
    events      chan hub.Event   // consumer-owned buffer
    systemInput trigger.Model    // Source 3: cron / async hook / file watcher
    conv        conv.Model       // agent outbox → conversation view
    env         env              // app-local TUI state
    services    services         // explicit feature-layer dependency graph
}

// services is the explicit runtime graph consumed by model.
type services struct {
    Setting         *setting.Settings
    LLM             *llm.Conn
    Tool            *tool.Registry
    Hook            *hook.Engine
    Session         *session.Setup
    Skill           *skill.Registry
    Subagent        *subagent.Registry
    Command         *command.Registry
    BackgroundTasks *task.Manager
    Plan            *todo.Store
    Cron             *cron.Scheduler
    MCP              *mcp.Registry
    Plugin           *plugin.Registry
    Agent            *agent.Session
    Persona          *persona.Registry
    Reminder         *reminder.Service
    SelfLearn        SelfLearnServices
}
```

Each sub-model package (`conv/`, `input/`, `trigger/`, `hub/`, `kit/`)
defines its own narrow `Runtime` interface. Root implements those via
adapter methods on `*model`, never reaching down into root from a
sub-model.

### Known Violations

- **The composition boundary is still inside `internal/app`.**
  `servicesFromDefaults` resolves compatibility package defaults once and the
  model/subagent paths use explicit references afterward. A later CLI/API
  boundary cleanup can move construction into `cmd/san` and make `app.Run`
  accept the completed graph.
- **Project reload still replaces six registry instances.**
  `reloadProjectServices` reinitializes settings, skills, commands, subagents,
  MCP, and personas, then updates the graph. Stable long-lived handles would
  make reload semantics simpler, but this is lower priority than removing
  service locators from runtime paths (already done).

## Internals

Root files (no business logic; pure glue):

| File | Role |
|---|---|
| `model.go` | Root `model` struct + `Init()`. Behaviour split across siblings. |
| `model_lifecycle.go` | Construction + run-option application + task lifecycle wiring + SessionEnd shutdown. |
| `model_session.go` | Session save/load + per-session task storage + fork. |
| `model_scrollback.go` | Render committed messages into terminal scrollback via `tea.Println`. |
| `model_agent_events.go` | `conv.Runtime` callbacks (turn start, tokens, tool results, turn end, stop). |
| `model_compact.go` | Conversation compaction (auto + `/compact`). |
| `model_tool_effects.go` | Side effects from tool calls (cwd, files, agent launches, overflow). |
| `model_workspace.go` | cwd / file change reactions + FileWatcher setup. |
| `model_turn_queue.go` | Turn-end inbox drain + prompt injection + stop-hook gate. |
| `model_deps.go` | Deps builders for sub-features (`overlayDeps`, `triggerDeps`, etc.). |
| `model_actions.go` | Identity switch + slash-command dispatch from selector hotkeys. |
| `update.go` | `Update()` dispatch + `routeFeatureUpdate` + `overlaySelectors`. |
| `update_keys.go` | Keyboard handling + active-modal delegation + Ctrl+O double-tap. |
| `update_resize.go` | Window resize + scrollback reflow. |
| `update_submit.go` | Submit + provider turn + skill invocation. |
| `update_command.go` | Slash command deps + execution. |
| `update_modal.go` | Operation-mode cycle + question-modal protocol. |
| `update_approval.go` | Permission approval flow + bridge response. |
| `update_input_effects.go` | Stream cancel, tool-call cancel, image paste, quit. |
| `view.go` | `View()` — composes sub-model `View()` strings into terminal layout. |
| `agent.go` | Agent session lifecycle helpers (`sendToAgent`, `ContinueOutbox`, `ReconfigureAgentTool`). |
| `services.go` | The explicit `services` graph + `servicesFromDefaults()` compatibility boundary. |
| `env.go` | `env` — app-local TUI state (provider snapshot, permissions, plan, cache). Pure state holder. |
| `hooks.go` | Hook integration glue (LLM completer wiring). |
| `init.go` | Global infrastructure init, plugin/mcp adapter wiring. |
| `run.go` | `Run()` — `tea.Program` entrypoint. |

Sub-model packages:

| Package | Source | Role |
|---|---|---|
| `app/input/` | Source 1 (user keyboard) | Textarea, selectors, approval modals, slash command dispatch. Big surface (37 files, `on_*.go` per component). |
| `app/conv/` | agent outbox | Conversation render state, streaming, message rendering, tool-call rendering, progress trackers. |
| `app/hub/` | Source 2 (agent → agent) | Pub/sub bus for subagent completion events. |
| `app/trigger/` | Source 3 (system) | File watcher, cron poll, async hook callback. |
| `app/kit/` | shared | Reusable TUI widgets (panel, listnav, theme, suggest, history). |

## Lifecycle

- `cmd/san` calls `app.Run()` which builds the `tea.Program`,
  `servicesFromDefaults()` resolves package defaults once, the root model is
  constructed, and `tea.Program.Run()` enters the MVU loop.
- Per turn: user submits → input subpackage → `sendToAgent()` → agent
  inbox → agent processes → outbox events → `conv` updates → re-render.
- On `/plugin install`, `/model`, etc.: `ReloadAfterPluginChange()` calls
  `reloadProjectServices()` and re-wires dependent runtime adapters.

## Tests

The `app` package and its sub-model packages have focused unit tests, backed by
end-to-end integration tests under `tests/integration/`:

```
internal/app/conv/message_test.go              — message rendering.
internal/app/conv/markdown_test.go             — markdown renderer.
internal/app/conv/plan_view_test.go            — plan task view.
internal/app/input/on_approval_test.go         — approval flow.
internal/app/input/on_mcp_test.go              — MCP slash command.
internal/app/input/on_plugin_test.go           — plugin slash command.
internal/app/input/on_provider_test.go         — provider selector.
internal/app/input/on_queue_test.go            — input queueing.
internal/app/input/on_textarea_test.go         — textarea behavior.
```

## See Also

- Code: `internal/app/`
- End-to-end data flow (keystroke → agent → render): [`concepts/data-flow.md`](../../concepts/data-flow.md)
- Rendering pipeline (View(), Markdown, tool blocks): [`concepts/rendering.md`](../../concepts/rendering.md)
- Underlying primitive: [`packages/core.md`](../3-core/core.md) (`Agent` interface)
- Foreground session wrapper: [`packages/agent.md`](../2-feature/agent.md)
- Layer: `app` — top of the stack, may import any feature package.
