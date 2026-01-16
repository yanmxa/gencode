# Proposal: Todo System (TodoWrite Tool)

- **Proposal ID**: 0005
- **Author**: mycode team
- **Status**: Implemented
- **Created**: 2025-01-15
- **Updated**: 2025-01-16
- **Implemented**: 2025-01-16

## Summary

Implement a TodoWrite tool that enables the agent to create, track, and manage structured task lists during coding sessions. This helps users understand progress, ensures the agent doesn't forget tasks, and provides visibility into multi-step operations.

## Motivation

Currently, gencode has no built-in task tracking. This leads to:

1. **Lost tasks**: Agent may forget steps in complex operations
2. **No visibility**: Users can't see what the agent plans to do
3. **Poor planning**: No structured approach to multi-step tasks
4. **Progress uncertainty**: Users don't know how far along the agent is
5. **Context loss**: Important subtasks get lost in long conversations

A todo system provides structured task management visible to both agent and user.

## Core Design Insight: Dual Purpose

TodoWrite serves **two critical purposes**:

### 1. User Visibility (UI Update)
- Show task progress in CLI
- Let users understand what agent is doing
- Display completion percentage

### 2. Model's External Memory (More Important)

LLMs have no persistent memory across turns. In long conversations:

```
Turn 1:  "I'll do A, B, then C"     ← In model's context
Turn 5:  [many tool calls...]
Turn 10: "Wait, what was I doing?"  ← Context faded, forgot the plan
```

TodoWrite solves this by **externalizing the plan**:

```
Turn 1:  TodoWrite([A, B, C])
         ↓
         Returns: "[>] A  [ ] B  [ ] C  (0/3)"

Turn 10: Model sees previous tool result in context:
         "[x] A  [>] B  [ ] C  (1/3)"
         ↓
         "I completed A, now working on B"
```

The rendered todo list in tool results acts as a **structured self-reminder** that persists across conversation turns.

> "Structure constrains AND enables." - The todo constraints (max items, one in_progress) enable reliable plan tracking.

## Claude Code Reference

Claude Code's TodoWrite tool provides structured task management:

### Tool Definition (from Claude Code Tools.json)
```typescript
TodoWrite({
  todos: [
    {
      content: "Run the build",           // Task description (required)
      status: "pending" | "in_progress" | "completed",  // Task status (required)
      id: "unique-id"                     // Unique identifier (required)
    }
  ]
})
```

### Key Design Principles (from learn-claude-code)

| Rule | Why |
|------|-----|
| Max 20 items | Prevents infinite task lists |
| One in_progress | Forces focus on one thing at a time |
| Required fields | Ensures structured output |

> "Structure constrains AND enables." - Todo constraints enable visible plans and tracked progress.

### Key Behaviors
- Agent creates todos when starting complex tasks (3+ steps)
- Exactly one task should be `in_progress` at a time
- Tasks marked complete immediately after finishing
- Todo list displayed to user in UI
- Model sends complete new list each time (not a diff)
- System returns rendered view for model to see its own plan

### System Reminders (Soft Prompts)
Claude Code uses reminder injection to encourage todo usage:
```
INITIAL_REMINDER = "<reminder>Use TodoWrite for multi-step tasks.</reminder>"
NAG_REMINDER = "<reminder>10+ turns without todo update. Please update todos.</reminder>"
```

### Example Usage
```
User: Fix the failing tests and update the docs

Agent: I'll create a todo list to track this work.
[TodoWrite:
  - Fix failing tests (in_progress)
  - Update documentation (pending)
]

Agent: Let me run the tests first...
[After fixing tests, updates todo]
[TodoWrite:
  - Fix failing tests (completed)
  - Update documentation (in_progress)
]
```

## Architecture Flow Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         TodoWrite System Flow                            │
└─────────────────────────────────────────────────────────────────────────┘

┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   User       │     │    Agent     │     │  TodoManager │
│   Input      │────▶│    Loop      │────▶│   (State)    │
└──────────────┘     └──────────────┘     └──────────────┘
                            │                    │
                            ▼                    ▼
                     ┌──────────────┐     ┌──────────────┐
                     │  TodoWrite   │────▶│   Validate   │
                     │    Tool      │     │   & Store    │
                     └──────────────┘     └──────────────┘
                            │                    │
                            ▼                    ▼
                     ┌──────────────┐     ┌──────────────┐
                     │   Return     │◀────│   Render     │
                     │   Result     │     │   Todos      │
                     └──────────────┘     └──────────────┘
                            │
                            ▼
                     ┌──────────────┐
                     │    CLI UI    │
                     │   Display    │
                     └──────────────┘

Detailed Flow:
==============

1. USER INPUT
   │
   ▼
2. AGENT DECIDES (complex task? 3+ steps?)
   │
   ├─ No  ──▶ Execute directly without todos
   │
   └─ Yes ──▶ Call TodoWrite to create task list
               │
               ▼
3. TODOWRITE TOOL EXECUTION
   │
   ├─ Validate input schema (Zod)
   ├─ Check constraints:
   │   ├─ Max 20 items
   │   ├─ Only 1 in_progress
   │   └─ Required fields present
   │
   ├─ Store in TodoManager
   │
   └─ Return rendered text:
       "[>] Fix tests <- Fixing failing tests
        [ ] Update docs
        (0/2 completed)"
               │
               ▼
4. AGENT SEES RESULT (its own plan visible)
   │
   ▼
5. AGENT WORKS ON TASK
   │
   ├─ Execute tools (Bash, Edit, etc.)
   │
   └─ Update todo status via TodoWrite
               │
               ▼
6. CLI DISPLAYS PROGRESS
   │
   ├─ Show todo box in UI
   └─ Update in real-time

7. REPEAT until all tasks completed
```

## State Management Flow

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Todo State Lifecycle                              │
└─────────────────────────────────────────────────────────────────────────┘

Session Start                          Session End
     │                                      │
     ▼                                      ▼
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  Empty   │───▶│  Active  │───▶│ Complete │───▶│  Saved   │
│  State   │    │  Todos   │    │   All    │    │ (Session)│
└──────────┘    └──────────┘    └──────────┘    └──────────┘
                     │
                     │ TodoWrite calls
                     ▼
              ┌──────────────┐
              │   Replace    │
              │  All Todos   │  (Not incremental - full replacement)
              └──────────────┘

Todo Item States:
=================
  ┌─────────┐     ┌─────────────┐     ┌───────────┐
  │ pending │────▶│ in_progress │────▶│ completed │
  └─────────┘     └─────────────┘     └───────────┘
       ○                ▶                   ✓
```

## Detailed Design

### API Design

```typescript
// src/tools/builtin/todo.ts - Types
type TodoStatus = 'pending' | 'in_progress' | 'completed';

interface TodoItem {
  content: string;      // Task description (imperative form)
  status: TodoStatus;   // Current state
  activeForm: string;   // Present tense for display (e.g., "Running tests")
}

interface TodoWriteInput {
  todos: TodoItem[];    // Complete list (replaces existing)
}

interface TodoState {
  items: TodoItem[];
  updatedAt: Date;
}
```

### Tool Implementation

```typescript
// src/tools/builtin/todo.ts
import { z } from 'zod';
import type { Tool, ToolContext, ToolResult } from '../types.js';

// Validation constraints (matching Claude Code)
const MAX_TODOS = 20;
const MAX_CONTENT_LENGTH = 500;

const TodoItemSchema = z.object({
  content: z.string().min(1).max(MAX_CONTENT_LENGTH),
  status: z.enum(['pending', 'in_progress', 'completed']),
  activeForm: z.string().min(1).max(MAX_CONTENT_LENGTH),
});

const TodoWriteInputSchema = z.object({
  todos: z.array(TodoItemSchema).max(MAX_TODOS),
});

export const todoWriteTool: Tool = {
  name: 'TodoWrite',
  description: `Create and manage a structured task list for your coding session.

Use when:
- Complex multi-step tasks (3+ steps)
- User provides multiple tasks
- Non-trivial work requiring planning

Do NOT use for:
- Single straightforward tasks
- Trivial operations
- Pure informational requests

Guidelines:
- Only ONE task should be in_progress at a time
- Mark tasks completed IMMEDIATELY after finishing
- Include both content (imperative) and activeForm (present continuous)`,

  parameters: TodoWriteInputSchema,

  execute: async (input: TodoWriteInput, context: ToolContext): Promise<ToolResult> => {
    // Validate: only one in_progress
    const inProgress = input.todos.filter(t => t.status === 'in_progress');
    if (inProgress.length > 1) {
      return {
        success: false,
        output: '',
        error: 'Only one task can be in_progress at a time',
      };
    }

    // Store in context (will be managed by TodoManager)
    context.setTodos(input.todos);

    // Render for model to see its own plan
    const rendered = renderTodos(input.todos);

    return {
      success: true,
      output: rendered,
    };
  },
};

// Render todos as text (returned to model)
function renderTodos(todos: TodoItem[]): string {
  if (todos.length === 0) {
    return 'No todos.';
  }

  const lines = todos.map(item => {
    switch (item.status) {
      case 'completed':
        return `[x] ${item.content}`;
      case 'in_progress':
        return `[>] ${item.content} <- ${item.activeForm}`;
      default:
        return `[ ] ${item.content}`;
    }
  });

  const completed = todos.filter(t => t.status === 'completed').length;
  lines.push(`\n(${completed}/${todos.length} completed)`);

  return lines.join('\n');
}
```

### TodoManager (State Management)

```typescript
// src/todo/todo-manager.ts
export class TodoManager {
  private items: TodoItem[] = [];

  setTodos(todos: TodoItem[]): void {
    this.items = [...todos];
  }

  getTodos(): TodoItem[] {
    return [...this.items];
  }

  getInProgress(): TodoItem | null {
    return this.items.find(t => t.status === 'in_progress') ?? null;
  }

  getProgress(): { completed: number; total: number; percent: number } {
    const completed = this.items.filter(t => t.status === 'completed').length;
    const total = this.items.length;
    const percent = total > 0 ? Math.round((completed / total) * 100) : 0;
    return { completed, total, percent };
  }

  clear(): void {
    this.items = [];
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/builtin/todo.ts` | Create | TodoWrite tool + types |
| `src/todo/todo-manager.ts` | Create | Todo state management |
| `src/todo/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register TodoWrite tool |
| `src/tools/types.ts` | Modify | Add setTodos to ToolContext |
| `src/agent/agent.ts` | Modify | Integrate TodoManager |
| `src/cli/components/TodoList.tsx` | Create | Todo list UI component |
| `src/cli/components/Messages.tsx` | Modify | Display todos in UI |

## User Experience

### Todo Display
Show current todos in the CLI:

```
┌─ Tasks ────────────────────────────────────┐
│ ✓ Fix failing tests                        │
│ ▶ Updating documentation                   │
│ ○ Run final build verification             │
│                                            │
│ Progress: 1/3 (33%)                        │
└────────────────────────────────────────────┘
```

### Status Indicators
- `○` - Pending (not started)
- `▶` - In Progress (currently working)
- `✓` - Completed (finished)

### Agent Messages
When agent updates todos:

```
📋 Todo list updated:
  ✓ Fix failing tests
  ▶ Updating documentation
  ○ Run final build verification
```

### Progress Visibility
Users see real-time progress as agent works:

```
Agent: Starting documentation update...
[Todo: "Updating documentation" → in_progress]

Agent: Documentation updated successfully.
[Todo: "Update documentation" → completed]
[Todo: "Run final build verification" → in_progress]
```

## Alternatives Considered

### Alternative 1: Simple Checklist
Plain text checklist without structured data.

**Pros**: Simpler implementation
**Cons**: No programmatic tracking, harder to display
**Decision**: Rejected - Structured data enables better UI and tracking

### Alternative 2: Hierarchical Todos
Support nested subtasks.

**Pros**: Better organization for complex tasks
**Cons**: Added complexity, harder to track progress
**Decision**: Deferred - Start simple, add later if needed

### Alternative 3: Persistent Todos Across Sessions
Keep todos across multiple sessions.

**Pros**: Long-term project tracking
**Cons**: Scope creep, different use case
**Decision**: Rejected - Session-scoped is cleaner for this use case

## Security Considerations

1. **Input Validation**: Sanitize todo content to prevent injection
2. **Size Limits**: Limit number of todos and content length
3. **Persistence**: Todo state stored with session, same security model

## Testing Strategy

1. **Unit Tests**:
   - Todo validation (content, status)
   - Progress calculation
   - Single in_progress enforcement

2. **Integration Tests**:
   - Tool registration and execution
   - Session persistence
   - UI rendering

3. **Manual Testing**:
   - Multi-step task workflows
   - Agent todo behavior
   - UI display across terminal sizes

## Implementation Steps

### Step 1: Create TodoManager (State Management)

Create `src/todo/` directory with:
- `types.ts` - Type definitions
- `todo-manager.ts` - State management class
- `index.ts` - Module exports

### Step 2: Create TodoWrite Tool

Create `src/tools/builtin/todo.ts`:
- Zod schema for input validation
- Tool definition with description
- Execute function with validation
- Render function for text output

### Step 3: Extend ToolContext

Modify `src/tools/types.ts`:
- Add `setTodos()` method to ToolContext
- Add `getTodos()` method to ToolContext

### Step 4: Integrate with Agent

Modify `src/agent/agent.ts`:
- Create TodoManager instance
- Pass to ToolContext
- Expose getTodos for CLI

### Step 5: Register Tool

Modify `src/tools/index.ts`:
- Import todoWriteTool
- Add to default registry

### Step 6: Create CLI Component

Create `src/cli/components/TodoList.tsx`:
- Display current todos
- Show progress bar
- Status icons (○ ▶ ✓)

### Step 7: Integrate with Messages

Modify `src/cli/components/Messages.tsx` or `App.tsx`:
- Show TodoList component when todos exist
- Update on todo changes

### Step 8: Add System Prompt Guidance

Modify `src/prompts/system/`:
- Add TodoWrite usage guidance
- Include when to use / not use

## Migration Path

1. **Phase 1**: Core TodoWrite tool implementation
2. **Phase 2**: CLI display integration
3. **Phase 3**: Session persistence
4. **Phase 4**: Agent prompting to encourage todo usage

No breaking changes; existing sessions work without todos.

## Implementation Notes

### Files Created/Modified

| File | Action | Description |
|------|--------|-------------|
| `src/tools/builtin/todowrite.ts` | Existed | Core tool + state management |
| `src/tools/types.ts` | Existed | TodoItem, TodoWriteInput schemas |
| `src/tools/index.ts` | Existed | Tool registration |
| `src/prompts/tools/todowrite.txt` | Modified | Added dual purpose explanation |
| `src/cli/components/TodoList.tsx` | Created | CLI display component |
| `src/cli/components/App.tsx` | Modified | Integrated TodoList display |

### Key Implementation Details

1. **Global State**: Todos stored in module-level variable, accessed via `getTodos()`
2. **Dual Purpose**: Tool description explains both UI visibility and model memory
3. **Constraints**: Max 20 items, only 1 in_progress at a time
4. **Rendering**: Returns formatted text for model to see its own plan
5. **CLI Integration**: TodoList component displays with progress bar

## References

- [Claude Code TodoWrite Documentation](https://code.claude.com/docs/en/tools)
- [Claude Code System Prompt (TodoWrite section)](https://gist.github.com/wong2/e0f34aac66caf890a332f7b6f9e2ba8f)
