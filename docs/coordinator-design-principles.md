# Coordinator Design Principles

This document defines the design principles for evolving GenCode from a
"single main loop with optional background subagents" into a coordinator-driven
multi-agent collaboration system.

The goal is not to clone Claude Code feature-for-feature. The goal is to
preserve the runtime ideas that make multi-agent collaboration actually work:
clear orchestration, durable worker state, asynchronous result delivery, and
high-quality synthesis in the main thread.

## Status

Current GenCode already has strong building blocks:

- reusable subagent execution in `internal/agent`
- reusable conversation runtime in `internal/runtime`
- background task execution in `internal/task`
- task/tracker UI in `internal/tool` and `internal/app/render`
- session persistence and subagent transcript persistence

What is still missing is the collaboration contract that ties those pieces
together.

## Problem Statement

GenCode can already launch subagents, including background subagents, but the
current model is still centered on one-off delegation:

- background agents are primarily "tasks to inspect"
- the main thread usually has to poll or explicitly fetch output
- worker results are not yet a first-class input stream back into the main loop
- worker continuation exists at the session level, but is not yet the normal
  orchestration pattern
- task state and task UI are split across multiple models without one explicit
  source of truth for collaboration

That model is sufficient for "run one subagent in the background" but not for
"coordinate several workers, synthesize findings, and keep steering them."

## Core Insight

The essence of a strong multi-agent coding system is not parallelism by itself.

The essence is:

1. the main thread acts as a coordinator
2. workers are durable, steerable actors
3. worker results return asynchronously as new coordinator signals
4. the coordinator performs synthesis instead of delegating understanding away

If any one of those pieces is missing, the system degrades into either:

- background jobs with manual polling
- noisy fan-out without synthesis
- fragile swarms with no durable continuation path

## Design Principles

### 1. Coordinator Is A Runtime Mode

Coordinator behavior must be explicit in runtime semantics, not just implied by
prompt wording or UI affordances.

Coordinator mode means the main thread is responsible for:

- planning delegation
- launching workers in parallel where safe
- reading worker results as internal signals
- synthesizing those results into concrete next instructions
- deciding whether to continue an existing worker or spawn a fresh one

It must not behave like "just another agent that happens to call Agent a lot."

### 2. Synthesis Stays In The Main Thread

Workers may research, implement, verify, or report failure. They must not be
used as a substitute for coordinator understanding.

After parallel research, the coordinator must:

- read the findings
- identify the correct implementation boundary
- write a self-contained next prompt
- explicitly choose continue vs respawn

Anti-patterns to avoid:

- "Based on your findings, fix it"
- "Use the previous research and continue"
- using one worker to summarize another worker

The coordinator owns understanding.

### 3. Workers Are Actors, Not Disposable Processes

A worker is more than a PID or a one-shot background task. It is a collaboration
object with:

- identity
- task description
- progress state
- transcript/session lineage
- pending follow-up messages
- output/result summary
- terminal state

This means worker state must be rich enough to support:

- inspection
- continuation
- cancellation
- retry
- verification handoff

Simple `running/completed/failed` status is necessary but insufficient.

### 4. Async Completion Must Re-Enter The Main Loop

Background completion is not just a UI event.

When a worker finishes, the result must be turned into a structured notification
that becomes new input for the coordinator at the next safe scheduling point.

This is the key behavioral difference between:

- a background task system
- a collaboration system

The main thread should not need to poll `TaskOutput` in the common case.
Polling remains a fallback, not the primary collaboration mechanism.

### 5. One Collaboration Queue, Not Many Ad Hoc Side Channels

All deferred work that should later affect the main thread should flow through a
single queue abstraction with ordering and priority semantics.

This queue should be capable of carrying at least:

- task completion notifications
- task failure notifications
- task stop notifications
- future worker-to-coordinator messages

The coordinator should consume these as part of normal turn scheduling instead
of each subsystem inventing its own callback path.

### 6. Continue vs Respawn Must Be Explicit

The system must treat worker continuation as a first-class orchestration choice.

Use continuation when:

- the worker already explored the relevant files
- the worker just produced errors that need correction
- the worker has useful local context for the next step

Use a fresh worker when:

- implementation needs a narrower context than the earlier research
- verification should be independent
- the previous worker is anchored on the wrong approach
- the next task is unrelated

The coordinator should be able to do either without awkward protocol gaps.

### 7. UI Should Reflect Runtime State, Not Invent It

The UI should display orchestration state that already exists in the runtime.
It should not become the source of truth.

The runtime should own:

- worker identity and lifecycle
- batch/group relationships
- progress and status
- queueable notifications

The UI should render:

- grouped worker launches
- running/completed/failed states
- recent activity
- navigation and management affordances

This keeps TUI changes incremental and avoids pushing orchestration logic into
Bubble Tea view code.

### 8. Coordination Requires A Single Task Model

GenCode currently has:

- background task runtime state
- tracker/task list state

Those can remain separate implementation layers, but they must represent one
logical collaboration model.

For coordinator work, the system needs a canonical task model that can express:

- parent/child task relationships
- worker ownership
- orchestration batch identity
- last activity
- result summary
- notification state
- continuation eligibility

Without a canonical model, the system will drift into duplicated state and
inconsistent UI.

### 9. Concurrency Policy Must Be Conservative By Default

Parallelism is valuable, but writes must remain bounded.

The default policy should be:

- read-only research can run in parallel
- implementation that touches overlapping files should serialize
- verification can run in parallel with non-overlapping implementation
- cancellation and redirection must be cheap

The runtime should eventually reason about write boundaries explicitly instead
of leaving all concurrency judgment to the model.

### 10. Protocols Matter More Than Features

The order of implementation should favor reusable collaboration protocols over
feature count.

Prefer implementing:

- notification protocol
- worker continuation protocol
- grouped task lifecycle

before implementing:

- teammate swarms
- remote teams
- advanced pane management
- elaborate task UI

If the protocol is sound, richer collaboration features can be added later
without redesigning the core.

## Collaboration Contracts

### Coordinator Contract

The coordinator must be able to:

- spawn one or more workers
- receive structured completion/failure/stop signals
- continue a known worker by ID
- stop a worker by ID
- summarize partial results while other workers still run

### Worker Contract

A worker must:

- have isolated execution context
- expose progress and terminal state
- persist enough state to be resumed/continued
- return structured completion metadata
- tolerate cancellation and redirection

### Notification Contract

At minimum, notifications should encode:

- task ID
- task type
- status
- summary
- optional result payload
- optional usage metadata
- optional output/transcript location

The notification payload should be structured enough that the coordinator can
react without scraping fragile prose.

## Proposed Target Shape For GenCode

The target shape is:

- `internal/runtime`: main and worker loop execution
- `internal/agent`: worker configuration, execution, continuation hooks
- `internal/task`: canonical lifecycle model for background/collaboration tasks
- `internal/tool`: tools for `Agent`, continuation, output, stop, task metadata
- `internal/app`: queue consumption, orchestration rendering, and coordinator UX

The important rule is not the exact package name. The important rule is:

- runtime owns execution
- task layer owns collaboration state
- tool layer owns protocols exposed to the model
- app layer owns interactive presentation and scheduling

## Incremental Rollout

### Phase 1: Coordinator MVP

Deliver the minimum runtime contract:

- explicit coordinator-mode prompt/runtime behavior
- grouped background worker launch state
- structured task notification queue
- automatic notification delivery back into the main loop

Success criteria:

- launch several background agents in one turn
- show them as one coordinated batch in UI
- receive completion notifications without manual polling
- allow the main thread to continue working while they run

### Phase 2: Durable Worker Continuation

Promote workers from "tasks" to "actors":

- explicit continue/send-message tool
- richer worker task state
- clearer resume semantics
- per-worker recent activity and pending message support

Success criteria:

- continue an existing worker after research completes
- continue a failed worker with corrected instructions
- preserve useful worker context between turns

### Phase 3: Coordinated Write Safety

Add orchestration safety:

- file overlap / write-boundary policy
- better cancellation and redirection flows
- improved verification worker pattern

Success criteria:

- avoid overlapping write collisions in common cases
- verification workers can run independently from implementation workers

### Phase 4: Team Expansion

Only after the above contracts are stable:

- `team_name` support
- in-process teammate runtime
- optional pane/worktree/team UI expansion

Success criteria:

- team workflows build on the same coordinator/worker/notification protocol
- no separate ad hoc swarm runtime is required for basic collaboration

## Non-Goals

This design does not require GenCode to:

- replicate Claude Code's entire UI
- implement tmux-based teammates first
- implement remote workers first
- replace direct tools with workers for simple tasks
- force all sessions into coordinator mode

## Review Checklist

Before implementing any multi-agent feature, verify:

- Does this strengthen the coordinator contract, or just add more fan-out?
- Does worker completion re-enter the main loop as a structured signal?
- Can the same worker be continued if it has relevant context?
- Is synthesis still happening in the main thread?
- Is the runtime state canonical, or are we duplicating state in UI code?
- Does the concurrency model stay conservative around writes?
- Does this change make team mode easier later, not harder?

## Related Docs

- [Architecture](architecture.md)
- [Subagent System](subagent-system.md)
- [Task Management](task-management.md)
- [Explore Agent](features/22-explore-agent.md)
