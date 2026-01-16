# Operating Modes

> Reference: [Claude Code](https://claude.ai/code) operating modes

GenCode has **3 operating modes** that can be cycled with `Shift+Tab`:

```
Normal (default)
    ↓ Shift+Tab
Plan (⏸ plan mode on)
    ↓ Shift+Tab
Accept (⏵⏵ accept edits on)
    ↓ Shift+Tab
Normal
    ...
```

## Mode Comparison

| Mode | Status Indicator | Edit Confirmation | Available Tools |
|------|------------------|-------------------|-----------------|
| **Normal** | _(none)_ | Required | All |
| **Plan** | `⏸ plan mode on (shift+tab to cycle)` | N/A (edits blocked) | Read-only |
| **Accept** | `⏵⏵ accept edits on (shift+tab to cycle)` | Auto-approved | All |

## Mode Details

### 1. Normal Mode (Default)

- Normal execution mode
- Every Write/Edit operation requires user confirmation
- All tools available
- Default mode when GenCode starts

### 2. Plan Mode

- Read-only exploration mode for designing implementation approaches
- Write, Edit, and Bash tools are blocked
- Used for understanding codebase and creating implementation plans

**Allowed Tools:** Read, Glob, Grep, WebFetch, WebSearch, TodoWrite, AskUserQuestion

**Blocked Tools:** Write, Edit, Bash

### 3. Auto-accept Mode

- Automatic edit acceptance mode
- Write/Edit operations are automatically approved without confirmation
- Useful when trusting the agent or after approving a plan
- **Caution**: May cause unintended file changes

## Plan Mode Workflow

When in Plan mode, the agent goes through 5 phases:

```
EnterPlanMode
     ↓
┌─────────────┐
│Understanding│ ← Explore codebase
└──────┬──────┘
       ↓
┌─────────────┐
│   Design    │ ← Design approach
└──────┬──────┘
       ↓
┌─────────────┐
│   Review    │ ← (Optional) Clarify requirements
└──────┬──────┘
       ↓
┌─────────────┐
│   Final     │ ← Write plan to file
└──────┬──────┘
       ↓
ExitPlanMode
       ↓
┌─────────────┐
│  Approval   │ ← User approves/modifies/cancels
└─────────────┘
```

## Plan Approval Options

When exiting Plan mode:

| Option | Description | Next Mode |
|--------|-------------|-----------|
| **approve** | Accept plan, auto-accept edits | Accept |
| **approve_manual** | Accept plan, manually approve each edit | Normal |
| **modify** | Return to modify the plan | Plan |
| **cancel** | Cancel plan entirely | Normal |

## References

- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Plan Mode Proposal](./proposals/0004-plan-mode.md)
