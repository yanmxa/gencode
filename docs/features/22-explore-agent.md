# Feature 22: Explore Agent

## Overview

`Explore` is a built-in read-only agent used for fast codebase investigation. It is the default agent for questions that require reading, searching, and cross-referencing multiple files before answering.

This feature exists to document the contract that was previously split across the general agent docs, plan mode docs, and subagent notes.

## Contract

| Property | Value |
|----------|-------|
| Agent type | Built-in `Explore` |
| Permission mode | `plan` |
| Tools | Read, Glob, Grep, WebFetch, WebSearch |
| Max turns | 100 |
| Execution style | Foreground in plan mode |

**Use `Explore` when:**
- The task needs reading multiple files before answering.
- The task needs cross-referencing code paths, configs, tests, and docs.
- The task is investigative and should not modify the workspace.

**Do not use `Explore` when:**
- One direct tool call is enough, such as a single `Read`, `Grep`, or `Glob`.
- The task needs file edits or command execution.
- The task should run in the background from plan mode.

## Behavior

- `Explore` inherits the parent model unless explicitly overridden.
- In plan mode, the `Agent` tool schema only exposes read-only built-in agent types.
- `Explore` must not expose `Bash`, `Write`, or `Edit`.
- `Explore` returns a normal agent tool result to the parent conversation; the parent conversation must continue cleanly after the result arrives.
- Interleaved `notice` messages must not prevent the parent conversation from recognizing that the agent finished.

## Relationship To Other Features

- [Feature 8](./8-plan-mode.md) defines the outer read-only session behavior.
- [Feature 10](./10-agents.md) defines the generic agent system and custom agent format.
- `Explore` is the built-in investigative specialization that sits at the intersection of those two features.

## Automated Tests

```bash
go test ./internal/agent -run TestPlanAgentsExposeOnlyReadOnlyTools -count=1
go test ./internal/app -run "TestPlanModeAgentExecutionStartsContinuationWithoutHanging|TestHasAllToolResultsAllowsInterleavedNotices|TestAsyncHookTickDoesNotInjectWhileToolExecutionPending|TestCronTickDoesNotDrainQueueWhileToolExecutionPending" -count=1
go test ./internal/tool -run TestPlanMode_AgentSchema_IsForegroundAndRestricted -count=1
```

Covered:

```
TestPlanAgentsExposeOnlyReadOnlyTools                    — Explore and Plan only expose read-only tools
TestPlanMode_AgentSchema_IsForegroundAndRestricted       — plan-mode Agent schema stays restricted
TestPlanModeAgentExecutionStartsContinuationWithoutHanging — Explore result resumes the parent flow
TestHasAllToolResultsAllowsInterleavedNotices           — notice messages do not break completion detection
TestAsyncHookTickDoesNotInjectWhileToolExecutionPending — async hook cannot interrupt pending Explore execution
TestCronTickDoesNotDrainQueueWhileToolExecutionPending  — cron cannot interrupt pending Explore execution
```
