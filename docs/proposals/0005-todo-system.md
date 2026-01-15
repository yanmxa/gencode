# Proposal: Todo System (TodoWrite Tool)

- **Proposal ID**: 0005
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a TodoWrite tool that enables the agent to create, track, and manage structured task lists during coding sessions. This helps users understand progress, ensures the agent doesn't forget tasks, and provides visibility into multi-step operations.

## Motivation

Currently, mycode has no built-in task tracking. This leads to:

1. **Lost tasks**: Agent may forget steps in complex operations
2. **No visibility**: Users can't see what the agent plans to do
3. **Poor planning**: No structured approach to multi-step tasks
4. **Progress uncertainty**: Users don't know how far along the agent is
5. **Context loss**: Important subtasks get lost in long conversations

A todo system provides structured task management visible to both agent and user.

## Claude Code Reference

Claude Code's TodoWrite tool provides structured task management:

### Tool Definition
```typescript
TodoWrite({
  todos: [
    {
      content: "Run the build",           // Task description
      status: "pending" | "in_progress" | "completed",
      activeForm: "Running the build"     // Present tense for display
    }
  ]
})
```

### Key Behaviors
- Agent creates todos when starting complex tasks
- Exactly one task should be `in_progress` at a time
- Tasks marked complete immediately after finishing
- Todo list displayed to user in UI
- Agent uses todos for planning before execution

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

## Detailed Design

### API Design

```typescript
// src/tools/todo/types.ts
type TodoStatus = 'pending' | 'in_progress' | 'completed';

interface TodoItem {
  id: string;
  content: string;
  activeForm: string;
  status: TodoStatus;
  createdAt: Date;
  updatedAt: Date;
}

interface TodoList {
  items: TodoItem[];
  sessionId: string;
  createdAt: Date;
  updatedAt: Date;
}

interface TodoWriteInput {
  todos: Array<{
    content: string;
    status: TodoStatus;
    activeForm: string;
  }>;
}
```

```typescript
// src/tools/todo/todo-tool.ts
const todoWriteTool: Tool<TodoWriteInput> = {
  name: 'TodoWrite',
  description: `Create and manage a structured task list for your current coding session.

Use this tool when:
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
- Include both content (imperative) and activeForm (present continuous)
`,
  parameters: z.object({
    todos: z.array(z.object({
      content: z.string().min(1),
      status: z.enum(['pending', 'in_progress', 'completed']),
      activeForm: z.string().min(1),
    }))
  }),
  execute: async (input, context) => { ... }
};
```

```typescript
// src/tools/todo/todo-manager.ts
class TodoManager {
  private list: TodoList;

  constructor(sessionId: string);

  // Replace entire todo list
  setTodos(todos: TodoWriteInput['todos']): void;

  // Get current todo list
  getTodos(): TodoItem[];

  // Get in-progress task (should be exactly one)
  getInProgress(): TodoItem | null;

  // Get completion percentage
  getProgress(): { completed: number; total: number; percent: number };

  // Serialize for display
  format(): string;
}
```

### Implementation Approach

1. **Tool Registration**: Add TodoWrite to the tool registry
2. **State Management**: Store todo list in session state
3. **UI Integration**: Display todo list in CLI interface
4. **Validation**: Ensure exactly one in_progress task
5. **Persistence**: Save todo state with session

```typescript
// Example tool execution
async function execute(input: TodoWriteInput, context: ToolContext): Promise<ToolResult> {
  const manager = context.getTodoManager();

  // Validate: exactly one in_progress
  const inProgress = input.todos.filter(t => t.status === 'in_progress');
  if (inProgress.length > 1) {
    return {
      success: false,
      error: 'Only one task can be in_progress at a time'
    };
  }

  manager.setTodos(input.todos);

  return {
    success: true,
    output: `Updated todo list (${manager.getProgress().percent}% complete)`
  };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/todo/types.ts` | Create | Todo type definitions |
| `src/tools/todo/todo-tool.ts` | Create | TodoWrite tool implementation |
| `src/tools/todo/todo-manager.ts` | Create | Todo state management |
| `src/tools/todo/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register TodoWrite tool |
| `src/session/types.ts` | Modify | Add todo list to session state |
| `src/cli/components/TodoList.tsx` | Create | Todo list UI component |

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

## Migration Path

1. **Phase 1**: Core TodoWrite tool implementation
2. **Phase 2**: CLI display integration
3. **Phase 3**: Session persistence
4. **Phase 4**: Agent prompting to encourage todo usage

No breaking changes; existing sessions work without todos.

## References

- [Claude Code TodoWrite Documentation](https://code.claude.com/docs/en/tools)
- [Claude Code System Prompt (TodoWrite section)](https://gist.github.com/wong2/e0f34aac66caf890a332f7b6f9e2ba8f)
