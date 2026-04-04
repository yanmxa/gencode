# GenCode Architecture Review Roadmap

Branch: `architecture-review`

Date: 2026-04-04

## Scope

This document consolidates the architecture review for GenCode, a Bubble Tea based AI coding assistant CLI, and turns the findings into an executable refactor roadmap.

Note: the current TUI implementation is centered in `internal/app/` and `internal/ui/`, not `internal/tui/`.

## Phase 1 Summary

### Package Responsibilities

| Package | Responsibility | Main Dependencies |
|---|---|---|
| `internal/app` | Top-level Bubble Tea application, state aggregation, message routing, stream startup, tool/approval/session/provider orchestration | `bubbletea`, `internal/core`, `internal/app/*`, `internal/provider`, `internal/tool`, `internal/system` |
| `internal/app/*` | TUI feature modules for input, output, conversation, provider, plugin, MCP, mode, approval, tool execution, selectors | `bubbletea`, feature-specific internal packages |
| `internal/agent` | Subagent registry, loading, execution, session resume/persist, worktree isolation | `internal/core`, `internal/provider`, `internal/tool`, `internal/session`, `internal/mcp`, `internal/worktree` |
| `internal/core` | Shared agent loop runtime: system prompt, streaming, tool filtering, tool execution, stop conditions, compaction | `internal/client`, `internal/system`, `internal/tool`, `internal/hooks`, `internal/message`, `internal/permission` |
| `internal/tool` | Built-in tools, tool registry, schemas, tool set filtering, Agent tool, task tools | `internal/provider`, `internal/task`, `internal/tool/ui` |
| `internal/provider/*` | LLM provider implementations and streaming adapters | `internal/provider`, `internal/message` |
| `internal/session` | Session persistence, transcript conversion, subagent session storage, tool-result overflow persistence | `internal/message`, `internal/tool` |
| `internal/config` | Settings loading, permission rules, command safety, workdir constraints | standard library, `mvdan.cc/sh` |
| `internal/hooks` | Hook engine for session/tool/permission/stop/notification events | `internal/config`, `internal/log` |
| `internal/mcp` | MCP registry, clients, tool calling, transport lifecycle | `internal/mcp/transport`, `internal/provider` |
| `internal/task` | Background task abstraction and manager for bash and subagent tasks | `context`, `sync`, `os/exec` |
| `internal/system` | System prompt composition and memory loading | `embed`, `internal/client`, `internal/log` |
| `internal/ui/*` | Shared non-domain UI helpers: theme, progress, suggestions, history | Bubble Tea ecosystem |

### TUI Organization

- Top-level state lives in `internal/app/model.go`.
- `Update()` in `internal/app/update.go` performs:
  - base event handling
  - feature routing
  - textarea fallback
- `View()` in `internal/app/view.go` performs:
  - selector/modal precedence
  - active content rendering
  - input and status rendering

### TUI Core Loop

1. User input enters Bubble Tea as `tea.Msg`.
2. `model.Update()` routes the message.
3. State is mutated in the relevant feature model.
4. A `tea.Cmd` is returned.
5. The command produces the next `tea.Msg`:
   - stream chunk
   - tool result
   - approval request
   - progress update
6. `View()` re-renders.
7. The app waits for the next event.

### Agent Core Loop

`internal/agent.Executor` builds a `core.Loop`; `internal/core.Loop.Run()` drives the actual loop:

1. accept user prompt
2. build system prompt and tool set
3. send LLM request
4. collect stream into a completion response
5. parse tool calls
6. filter calls via hooks
7. execute tools
8. inject tool results back into the conversation
9. continue until:
   - no tool calls
   - max turns reached
   - cancellation
   - stop hook block
   - max-output recovery exhausted

### Where TUI and Agent Meet

- Main chat does not call `agent.Executor`; it directly drives `core.Loop.Stream()`.
- Subagents run as the `Agent` tool and are invoked via the tool execution path.
- Main chat token streaming returns to TUI through:
  - provider stream channel
  - `waitForChunk()`
  - `appconv.ChunkMsg`
  - `updateStream()`
- Subagent progress returns to TUI through:
  - `_onProgress`
  - `internal/ui/progress` global channel
  - `progress.UpdateMsg`

### End-to-End Data Flow

1. User presses Enter in terminal.
2. TUI appends a user message.
3. TUI configures `core.Loop`.
4. Provider streaming starts.
5. Chunks are converted into Bubble Tea messages.
6. Assistant text is rendered incrementally.
7. If tool calls appear, TUI dispatches tool execution.
8. Tool results are appended as conversation messages.
9. TUI starts the next LLM turn.
10. Final assistant output is committed to scrollback and rendered.

## Phase 2 Summary

### High Impact Findings

| File Path | Problem Type | Description |
|---|---|---|
| `internal/app/tool/run.go` + `internal/app/handler_input.go` + `internal/app/handler_tool.go` | Cancellation propagation gap | Tool execution mostly uses `context.Background()`. `Esc` stops UI state but does not reliably cancel active tools, MCP calls, or foreground subagents. |
| `internal/app/tool/run.go` + `internal/core/core.go` | Duplicate tool runtime | TUI and shared core each implement their own tool execution path. Behavior can drift. |
| `internal/app/handler_stream.go` + `internal/core/core.go` | Duplicate loop state machine | Main chat and subagent execution each maintain similar loop semantics for continuation, truncation recovery, tool chaining, and stop handling. |
| `internal/agent/executor.go` | Mixed responsibilities | `Executor.Run()` combines validation, isolation, runtime assembly, MCP connection, session restore, hooks, logging, and result mapping. |
| `internal/config/permission.go` + `internal/permission/permission.go` | Split permission vocabulary | Two permission systems define overlapping concepts and tool safety sets. |
| `internal/app/prompt_suggest.go` + `internal/app/handler_compact.go` + `internal/app/provider/handler.go` | TUI-runtime coupling | UI layer directly performs provider and runtime orchestration work. |

### Medium Impact Findings

| File Path | Problem Type | Description |
|---|---|---|
| `internal/ui/progress/progress.go` | Implicit loop communication | Subagent progress flows through a global singleton channel rather than an explicit per-session transport. |
| `internal/app/provider/model.go` | Large multi-responsibility file | Provider selector mixes state, data loading, connection logic, interaction, and rendering. |
| `internal/app/plugin/model.go` | Large multi-responsibility file | Plugin selector packs three UI products into one giant model. |
| `internal/tool/schema.go` | Giant centralized function | Tool schemas are maintained in one very long function. |
| `internal/provider/*/client.go` | Repeated streaming patterns | Provider streaming implementations duplicate chunk-building patterns. |
| `internal/app/handler_input.go` | Large input pipeline | Key routing, modal handling, submit flow, history, images, and hooks are tightly packed together. |
| `internal/app/model.go` | Large constructor | `newModel()` is a broad initialization funnel. |
| `internal/app/render/message.go` | Oversized rendering module | Tool-specific rendering logic is heavily centralized. |

## Phase 3 Refactor Strategy

### P0

1. unify shared tool execution runtime
2. propagate cancel context into tools and subagents
3. converge main chat loop and subagent loop semantics on `core.Loop`
4. split `agent.Executor.Run()`
5. consolidate permission vocabulary

### P1

1. move provider/runtime orchestration out of TUI handlers
2. replace global progress channel with a scoped sink
3. split giant selector and schema files

### P2

1. extract provider streaming helpers
2. split input submit pipeline from key routing
3. normalize constants and error messages

## Execution Roadmap

### Group 0: Behavior Protection

#### Commit 1

`test: pin current behavior around loop/tool cancellation`

- add regression coverage for:
  - tool result continuation
  - stale result suppression after cancel
  - max token recovery
  - parallel tool completion ordering

#### Commit 2

`test: add characterization tests for permission/runtime parity`

- lock down current behavior across:
  - main chat
  - shared core loop
  - subagent execution

### Group 1: P0 Convergence

#### Commit 3

`refactor(core): introduce shared tool execution runtime`

- add a shared executor abstraction
- move common parsing, lookup, MCP routing, and result wrapping into it

#### Commit 4

`refactor(app): route TUI tool execution through shared runtime`

- keep `ExecResultMsg` and `ExecDoneMsg`
- make `internal/app/tool/run.go` a thin adapter

#### Commit 5

`refactor(agent): route core loop tool execution through shared runtime`

- make `core.Loop.ExecTool()` use the same runtime

#### Commit 6

`fix(app): propagate cancel context into tool execution`

- add execution context to `app/tool.ExecState`
- cancel active tool execution on `Esc` and quit

#### Commit 7

`fix(agent): propagate cancel into foreground subagent execution`

- ensure the same context reaches:
  - `AgentTool`
  - `agent.Executor`
  - `core.Loop`
  - tool/MCP execution

#### Commit 8

`refactor(agent): split Executor.Run into smaller steps`

- extract:
  - request validation
  - workspace prep
  - loop assembly
  - conversation restore
  - result mapping

#### Commit 9

`refactor(permission): unify runtime permission vocabulary`

- remove duplicated safety/tool-classification logic
- keep one source of truth

### Group 2: P1 Decoupling

#### Commit 10

`refactor(app): extract conversation runtime service`

- move stream startup, compact, token-limit fetch, and suggestion generation behind a service interface

#### Commit 11

`refactor(app): move agent tool configuration out of UI handlers`

- provider UI should emit intent, not build runtime executors

#### Commit 12

`refactor(progress): replace global progress channel with scoped sink`

- progress should be bound to the active TUI/runtime instance

#### Commit 13

`refactor(tool): split schema registry into per-tool builders`

- each tool exposes its own schema builder
- aggregator keeps current external API

#### Commit 14

`refactor(provider-ui): split provider selector model`

- split by:
  - state
  - data loading
  - update
  - render

#### Commit 15

`refactor(plugin-ui): split plugin selector by tab`

- separate installed, discover, and marketplace views

### Group 3: P2 Cleanup

#### Commit 16

`refactor(provider): extract shared streaming helpers`

- start with OpenAI and Anthropic
- then propagate to others

#### Commit 17

`refactor(app): split input submit pipeline from key routing`

- isolate:
  - key routing
  - modal routing
  - submit preprocessing
  - submit execution

#### Commit 18

`chore: normalize tool constants and error message helpers`

- replace ad hoc string literals
- centralize common runtime error messages

## Recommended First Iteration

If only one iteration is planned, stop after Commit 9.

That delivers the highest-value structural fixes:

- real cancellation propagation
- one shared tool runtime
- reduced drift between main chat and subagent execution
- simpler agent executor
- one permission vocabulary

## Acceptance Criteria

### After Group 1

- `Esc` cancels active foreground tool work, not just UI state
- main chat and subagent tool execution use the same core semantics
- `Executor.Run()` is readable as orchestration rather than implementation detail
- permission behavior does not depend on which runtime path executed the tool

### After Group 2

- `internal/app` no longer directly assembles runtime internals
- progress transport is explicit
- `provider/model.go`, `plugin/model.go`, and `tool/schema.go` are materially reduced

### After Group 3

- provider stream adapters share reusable helpers
- input flow is easier to test and extend
- tool names and common runtime errors are normalized

