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
- Edit and Bash tools are blocked
- Write is allowed but **restricted to the plan file only**
- Used for understanding codebase and creating implementation plans

**Allowed Tools:** Read, Glob, Grep, WebFetch, WebSearch, TodoWrite, AskUserQuestion, Write (plan file only)

**Blocked Tools:** Edit, Bash

**System Prompt**: In plan mode, additional instructions are injected into the system prompt explaining the 5-phase workflow, tool restrictions, and how to use ExitPlanMode with permission requests.

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

When exiting Plan mode, you are presented with 5 options:

| # | Option | Shortcut | Context | Plan Details | Edit Approval | Next Mode |
|---|--------|----------|---------|--------------|---------------|-----------|
| 1 | **Yes, clear context and auto-accept edits** | shift+tab, 1 | Cleared | Cleared | Auto | Accept |
| 2 | **Yes, and manually approve edits** | 2 | Kept | Kept | Manual | Normal |
| 3 | **Yes, auto-accept edits** | 3 | Kept | Cleared | Auto | Accept |
| 4 | **Yes, manually approve edits** | 4 | Kept | Cleared | Manual | Normal |
| 5 | **Type here to tell Claude what to change** | 5 | Kept | Kept | N/A | Plan |

### Option Descriptions

**Option 1: approve_clear** - Fresh start with automatic approval
- Clears all conversation history
- Clears plan details and exploration results
- Auto-accepts all edits without confirmation
- Use when: You want a clean slate and trust the plan completely

**Option 2: approve_manual_keep** - Keep everything, review changes
- Keeps conversation history
- Keeps plan file and exploration context
- Manually approve each edit
- Use when: You want to review each change carefully while preserving all plan context

**Option 3: approve** - Continue with automatic approval
- Keeps conversation history
- Clears plan-specific details
- Auto-accepts all edits without confirmation
- Use when: You trust the plan and want to proceed quickly

**Option 4: approve_manual** - Standard manual review
- Keeps conversation history
- Clears plan-specific details
- Manually approve each edit
- Use when: You want conservative manual approval without plan details

**Option 5: modify** - Go back and modify the plan
- Keeps conversation history
- Keeps plan file and exploration context
- Returns to plan mode for modifications
- Use when: You want to adjust the approach before execution

**Cancel** - Exit plan mode without executing (ESC key)
- Cancels the plan entirely
- Returns to normal mode
- Use when: You want to abandon the plan

## References

- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Plan Mode Proposal](./proposals/0004-plan-mode.md)
