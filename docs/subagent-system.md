# GenCode Subagent System Documentation

This document describes the Task tool, TaskOutput, TaskStop, and the Subagent system in GenCode.

## Overview

The Subagent system allows GenCode to spawn specialized AI agents for complex tasks. Each agent runs in an isolated context with filtered tool access and returns only the final result to the main conversation.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Main Conversation                         │
│                        (User ↔ LLM Loop)                         │
├─────────────────────────────────────────────────────────────────┤
│                              │                                   │
│                    Task Tool Invocation                          │
│                              │                                   │
│              ┌───────────────┴───────────────┐                   │
│              ▼                               ▼                   │
│     ┌─────────────────┐             ┌─────────────────┐         │
│     │ Foreground Agent│             │ Background Agent│         │
│     │ (blocks, waits) │             │ (returns ID)    │         │
│     └────────┬────────┘             └────────┬────────┘         │
│              │                               │                   │
│              ▼                               ▼                   │
│     Return final result            TaskOutput / TaskStop         │
└─────────────────────────────────────────────────────────────────┘
```

---

## Task Tool

The `Task` tool spawns a subagent to handle complex tasks.

### Schema

```json
{
  "name": "Task",
  "description": "Launch a new agent to handle complex, multi-step tasks autonomously",
  "parameters": {
    "subagent_type": {
      "type": "string",
      "description": "Agent type: Explore, Plan, Bash, Review, general-purpose, or custom",
      "required": true
    },
    "prompt": {
      "type": "string",
      "description": "The task for the agent to perform",
      "required": true
    },
    "description": {
      "type": "string",
      "description": "A short (3-5 word) description of the task"
    },
    "run_in_background": {
      "type": "boolean",
      "description": "Run agent in background, returns task_id immediately",
      "default": false
    },
    "model": {
      "type": "string",
      "description": "Override model: sonnet, opus, haiku",
      "enum": ["sonnet", "opus", "haiku"]
    },
    "max_turns": {
      "type": "integer",
      "description": "Maximum conversation turns before stopping"
    },
    "resume": {
      "type": "string",
      "description": "Agent ID to resume from a previous execution"
    }
  }
}
```

### Execution Flow

1. **Permission Check**: User must approve agent spawning
2. **Context Isolation**: New message array created (not shared with main loop)
3. **System Prompt**: Built dynamically with agent type, task, mode, environment
4. **Tool Filtering**: Only allowed tools exposed to agent
5. **LLM Loop**: Agent runs until completion or max_turns
6. **Result Return**: Only final content returned to main conversation

### Usage Examples

```
# Foreground (blocking)
Task(subagent_type="Explore", prompt="Find all database-related files")

# Background (non-blocking)
Task(subagent_type="Explore", prompt="Analyze codebase", run_in_background=true)
```

---

## TaskOutput Tool

Retrieves output from a background task.

### Schema

```json
{
  "name": "TaskOutput",
  "description": "Retrieve output from a background task",
  "parameters": {
    "task_id": {
      "type": "string",
      "description": "The task ID to get output from",
      "required": true
    },
    "block": {
      "type": "boolean",
      "description": "Wait for task completion",
      "default": true
    },
    "timeout": {
      "type": "number",
      "description": "Max wait time in milliseconds (max 600000)",
      "default": 30000
    }
  }
}
```

### Behavior

| Scenario | block=true | block=false |
|----------|-----------|-------------|
| Task running | Wait up to timeout | Return current status |
| Task completed | Return result | Return result |
| Task failed | Return error + output | Return error + output |
| Timeout reached | Return "still running" + options | N/A |

### Output Format

**For running tasks:**
```
Agent: Explore
Status: still running
Turns: 5
Tokens: 1000

Options:
  - Wait longer: TaskOutput(task_id="xxx", timeout=60000)
  - Check status: TaskOutput(task_id="xxx", block=false)
  - Stop: TaskStop(task_id="xxx")
```

**For completed tasks:**
```
Agent: Explore
Status: completed
Turns: 12
Tokens: 2500
Duration: 45s

Output:
[Agent's final response]
```

---

## TaskStop Tool

Stops a running background task.

### Schema

```json
{
  "name": "TaskStop",
  "description": "Stop a running background task by its ID",
  "parameters": {
    "task_id": {
      "type": "string",
      "description": "The task ID to stop",
      "required": true
    }
  }
}
```

### Stop Mechanism

1. **Graceful Stop**: Cancel context (`t.cancel()`)
2. **Wait**: Poll status for up to 2 seconds
3. **Force Kill**: If still running, mark as killed

### Context Cancellation Points

The agent executor checks for cancellation at 3 points:

```go
// 1. Main loop start
for turnCount < maxTurns {
    select {
    case <-ctx.Done():
        return cancelled
    default:
    }
}

// 2. Tool execution loop
for _, tc := range response.ToolCalls {
    select {
    case <-ctx.Done():
        return cancelled
    default:
    }
}

// 3. LLM streaming
for chunk := range streamChan {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
}
```

### Limitations

| Task Type | TaskStop Works? | Reason |
|-----------|-----------------|--------|
| Background Agent | ✅ Yes | Registered in TaskManager |
| Background Bash | ✅ Yes | Registered in TaskManager |
| Foreground Agent | ❌ No | Not registered, no task_id |
| Foreground Bash | ❌ No | Not registered, no task_id |

**Note**: Foreground tasks can be cancelled by pressing `Esc` in the TUI.

---

## Built-in Subagents

All built-in agents are defined in `internal/agent/registry.go`.

### Explore

Fast codebase exploration and understanding.

| Property | Value |
|----------|-------|
| Permission Mode | `plan` (read-only) |
| Tools | Read, Glob, Grep, WebFetch, WebSearch |
| Max Turns | 30 |
| Model | inherit (from parent) |

**Use Cases:**
- Find files by pattern
- Search code for keywords
- Answer questions about the codebase

### Plan

Software architect for designing implementation plans.

| Property | Value |
|----------|-------|
| Permission Mode | `plan` (read-only) |
| Tools | Read, Glob, Grep, WebFetch, WebSearch |
| Max Turns | 50 |
| Model | inherit |

**Use Cases:**
- Design implementation strategies
- Identify critical files
- Consider architectural trade-offs

### Bash

Command execution specialist.

| Property | Value |
|----------|-------|
| Permission Mode | `default` |
| Tools | Bash, Read, Glob, Grep |
| Max Turns | 30 |
| Model | inherit |

**Use Cases:**
- Run complex command sequences
- Git operations
- Build and test commands

### Review

Code review specialist.

| Property | Value |
|----------|-------|
| Permission Mode | `plan` (read-only) |
| Tools | Read, Glob, Grep, Bash |
| Max Turns | 30 |
| Model | inherit |

**Use Cases:**
- Analyze code changes
- Identify issues
- Suggest improvements

### general-purpose

Full-access agent for complex tasks.

| Property | Value |
|----------|-------|
| Permission Mode | `default` |
| Tools | All except Task |
| Max Turns | 50 |
| Model | inherit |

**Use Cases:**
- Research complex questions
- Multi-step tasks
- When other agents are too restrictive

---

## System Prompt Construction

The system prompt for each agent is built dynamically in `executor.go:buildSystemPrompt()`:

```
┌─────────────────────────────────────────────────────────────────┐
│                        System Prompt                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. AGENT IDENTITY                                               │
│     "You are a specialized AI agent within GenCode..."          │
│                                                                  │
│  2. AGENT TYPE                                                   │
│     ## Agent Type: {config.Name}                                 │
│     {config.Description}                                         │
│                                                                  │
│  3. TASK CONTEXT                                                 │
│     ## Your Task                                                 │
│     {req.Prompt}                                                 │
│                                                                  │
│  4. MODE INSTRUCTIONS (based on PermissionMode)                  │
│     ├─ plan:    "Read-Only mode, do not modify files"           │
│     └─ dontAsk: "Autonomous mode, full autonomy"                │
│                                                                  │
│  5. CUSTOM SYSTEM PROMPT (if config.SystemPrompt exists)        │
│     ## Additional Instructions                                   │
│     {config.SystemPrompt}                                        │
│                                                                  │
│  6. ENVIRONMENT                                                  │
│     - Working directory: /path/to/project                        │
│     - Platform: darwin                                           │
│     - Date: 2026-02-03                                           │
│                                                                  │
│  7. GUIDELINES                                                   │
│     - Focus on completing your assigned task efficiently         │
│     - Return a clear summary when your task is complete          │
│     - If you encounter errors, report them clearly               │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Example System Prompt (Explore Agent)

```
You are a specialized AI agent within GenCode, an AI coding assistant.

## Agent Type: Explore
Fast codebase exploration and understanding. Use for finding files,
searching code, and answering questions about the codebase.

## Your Task
Find all files in the src/ directory that import the database module.
List them with their import paths.

## Mode: Read-Only
You are in read-only mode. You can only use tools that read information
(Read, Glob, Grep, WebFetch, WebSearch). Do not attempt to modify any files.

## Environment
- Working directory: /Users/myan/Workspace/gencode
- Platform: darwin
- Date: 2026-02-03

## Guidelines
- Focus on completing your assigned task efficiently
- Return a clear summary when your task is complete
- If you encounter errors, report them clearly
```

---

## Custom Agents

You can create custom agents by adding `.md` files to the agents directory.

### Search Paths (Priority Order)

1. `.gen/agents/*.md` (project-level)
2. `~/.gen/agents/*.md` (user-level)
3. `.claude/agents/*.md` (project-level, Claude Code compatible)
4. `~/.claude/agents/*.md` (user-level, Claude Code compatible)

### Custom Agent Format

```markdown
---
name: ml-engineer
description: Machine Learning Engineering specialist
model: inherit
permission-mode: dontAsk
max-turns: 50
tools:
  mode: allowlist
  allow:
    - Read
    - Write
    - Bash
    - Glob
    - Grep
---

# ML Engineering Agent

You are specialized in implementing machine learning models and data pipelines.

## Expertise
- PyTorch and TensorFlow
- Data preprocessing
- Model training and evaluation
- Hyperparameter tuning

## Guidelines
- Always validate data before training
- Use appropriate cross-validation strategies
- Document model architectures clearly
```

### Configuration Options

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique agent identifier |
| `description` | string | Short description for LLM selection |
| `model` | string | `inherit`, `sonnet`, `opus`, `haiku` |
| `permission-mode` | string | `plan`, `default`, `acceptEdits`, `dontAsk` |
| `max-turns` | int | Maximum conversation turns |
| `tools.mode` | string | `allowlist` or `denylist` |
| `tools.allow` | []string | Tools to allow (allowlist mode) |
| `tools.deny` | []string | Tools to deny (denylist mode) |

### Permission Modes

| Mode | Description |
|------|-------------|
| `plan` | Read-only, only Read/Glob/Grep/WebFetch/WebSearch allowed |
| `default` | Normal permission handling |
| `acceptEdits` | Auto-approve Write/Edit |
| `dontAsk` | Full autonomy, all operations auto-approved |

---

## Context Isolation

Agents run in completely isolated contexts:

```
Main Conversation                    Agent Conversation
─────────────────                    ──────────────────
messages = [                         messages = [
  {user: "Analyze auth"},              {user: prompt},  ← Only this
  {assistant: "Using Task..."},        {assistant: ...},
  {toolResult: final_output},  ←───────{assistant: ...},
  {assistant: "Based on..."}           {assistant: final}  ← Only this returns
]                                    ]

❌ Agent's internal messages NOT shared
❌ Agent's tool calls NOT exposed
✅ Only final result returned
```

---

## File Locations

| File | Purpose |
|------|---------|
| `internal/tool/task.go` | Task tool implementation |
| `internal/tool/taskoutput.go` | TaskOutput tool |
| `internal/tool/taskstop.go` | TaskStop tool |
| `internal/agent/types.go` | Core type definitions |
| `internal/agent/registry.go` | Built-in agent configs |
| `internal/agent/loader.go` | Custom agent loading |
| `internal/agent/executor.go` | Agent LLM loop |
| `internal/agent/adapter.go` | Tool/Agent package bridge |
| `internal/task/agent_task.go` | Background task wrapper |
| `internal/task/manager.go` | Task lifecycle management |
