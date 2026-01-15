# Proposal: Task Tool & Subagent System

- **Proposal ID**: 0003
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a Task tool that launches specialized subagents to handle complex, multi-step tasks autonomously. Subagents run with separate context windows, specific tool access, and defined expertise areas, enabling efficient handling of specialized tasks without polluting the main conversation context.

## Motivation

Currently, mycode uses a single agent for all tasks. This leads to:

1. **Context pollution**: Complex research fills up main context
2. **No specialization**: One agent can't optimize for different task types
3. **No parallelism**: Tasks must run sequentially
4. **Context overflow**: Long operations exhaust context limits
5. **No delegation**: Can't spawn workers for independent subtasks

A subagent system enables parallel, specialized, context-isolated task execution.

## Claude Code Reference

Claude Code's Task tool launches specialized agents with defined capabilities:

### Tool Definition
```typescript
Task({
  description: "Explore authentication code",  // Short description (3-5 words)
  prompt: "Find all authentication-related files...",  // Detailed task
  subagent_type: "Explore",  // Agent specialization
  model: "haiku",            // Optional: model selection
  run_in_background: false   // Optional: async execution
})
```

### Available Agent Types
| Type | Description | Tools Available |
|------|-------------|-----------------|
| `Explore` | Fast codebase exploration | Read, Glob, Grep (read-only) |
| `Plan` | Architecture and design | All read tools, no edit |
| `Bash` | Command execution | Bash only |
| `general-purpose` | Full capabilities | All tools |

### Key Features
- Separate context window per subagent
- Constrained tool access per type
- Background execution option
- Model selection (haiku for simple, sonnet for complex)
- Resume capability with agent ID
- Parallel launch (multiple Task calls in one message)

### Example Usage
```
User: Find where authentication errors are handled

Agent: I'll launch an Explore agent to search the codebase.
[Task:
  description: "Find auth error handling"
  prompt: "Search for authentication error handling..."
  subagent_type: "Explore"
]

Explore Agent Result:
Found authentication error handling in:
- src/auth/errors.ts:45 - AuthError class
- src/middleware/auth.ts:78 - error handler
...

Agent: Based on the exploration, authentication errors are handled in...
```

## Detailed Design

### API Design

```typescript
// src/subagents/types.ts
type SubagentType = 'Explore' | 'Plan' | 'Bash' | 'general-purpose';

interface TaskInput {
  description: string;      // Short description (3-5 words)
  prompt: string;           // Detailed task instructions
  subagent_type: SubagentType;
  model?: 'haiku' | 'sonnet' | 'opus';
  run_in_background?: boolean;
  resume?: string;          // Agent ID to resume
  max_turns?: number;       // Max conversation turns
}

interface TaskOutput {
  success: boolean;
  result?: string;
  agentId: string;          // For resume capability
  outputFile?: string;      // For background tasks
  error?: string;
}

interface SubagentConfig {
  type: SubagentType;
  allowedTools: string[];
  defaultModel: string;
  systemPrompt: string;
  maxTurns: number;
}
```

```typescript
// src/subagents/task-tool.ts
const taskTool: Tool<TaskInput> = {
  name: 'Task',
  description: `Launch a specialized subagent for complex tasks.

Available agent types:
- Explore: Fast codebase exploration (read-only)
- Plan: Architecture design and planning
- Bash: Command execution specialist
- general-purpose: Full capabilities

Guidelines:
- Use Explore for searching/understanding code
- Use Plan for designing implementation approaches
- Use Bash for running commands
- Include 3-5 word description
- Provide detailed prompt with context
- Use haiku for simple, sonnet for complex tasks
- Launch multiple agents in parallel when possible
`,
  parameters: z.object({
    description: z.string().min(1),
    prompt: z.string().min(1),
    subagent_type: z.enum(['Explore', 'Plan', 'Bash', 'general-purpose']),
    model: z.enum(['haiku', 'sonnet', 'opus']).optional(),
    run_in_background: z.boolean().optional(),
    resume: z.string().optional(),
    max_turns: z.number().positive().optional()
  }),
  execute: async (input, context) => { ... }
};
```

```typescript
// src/subagents/subagent.ts
class Subagent {
  private id: string;
  private type: SubagentType;
  private config: SubagentConfig;
  private agent: Agent;

  constructor(type: SubagentType, config?: Partial<SubagentConfig>);

  // Execute task and return result
  async run(prompt: string): Promise<TaskOutput>;

  // Run in background, return immediately
  async runInBackground(prompt: string): Promise<{ agentId: string; outputFile: string }>;

  // Resume from previous execution
  async resume(agentId: string): Promise<TaskOutput>;

  // Get current status
  getStatus(): 'running' | 'completed' | 'error';
}
```

### Implementation Approach

1. **Subagent Registry**: Define configs for each agent type
2. **Tool Filtering**: Restrict tools based on agent type
3. **Context Isolation**: Each subagent gets fresh context
4. **Result Aggregation**: Collect and format subagent results
5. **Background Execution**: Spawn async workers with output files
6. **Resume System**: Store agent state for continuation

```typescript
// Subagent configurations
const SUBAGENT_CONFIGS: Record<SubagentType, SubagentConfig> = {
  'Explore': {
    type: 'Explore',
    allowedTools: ['Read', 'Glob', 'Grep', 'LS'],
    defaultModel: 'haiku',
    systemPrompt: 'You are a codebase exploration specialist...',
    maxTurns: 10
  },
  'Plan': {
    type: 'Plan',
    allowedTools: ['Read', 'Glob', 'Grep', 'LS', 'WebFetch'],
    defaultModel: 'sonnet',
    systemPrompt: 'You are a software architect designing solutions...',
    maxTurns: 5
  },
  'Bash': {
    type: 'Bash',
    allowedTools: ['Bash', 'KillShell'],
    defaultModel: 'haiku',
    systemPrompt: 'You are a shell command specialist...',
    maxTurns: 20
  },
  'general-purpose': {
    type: 'general-purpose',
    allowedTools: ['*'],  // All tools
    defaultModel: 'sonnet',
    systemPrompt: 'You are a general-purpose coding assistant...',
    maxTurns: 20
  }
};
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/subagents/types.ts` | Create | Subagent type definitions |
| `src/subagents/task-tool.ts` | Create | Task tool implementation |
| `src/subagents/subagent.ts` | Create | Subagent class |
| `src/subagents/configs.ts` | Create | Subagent configurations |
| `src/subagents/background-runner.ts` | Create | Background execution |
| `src/subagents/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register Task tool |
| `src/agent/agent.ts` | Modify | Support tool filtering |

## User Experience

### Subagent Launch Display
```
Agent: I'll explore the codebase to understand the authentication system.

┌─ Launching Subagent ──────────────────────────┐
│ Type: Explore                                 │
│ Task: Find auth error handling                │
│ Model: haiku                                  │
└───────────────────────────────────────────────┘

[Explore agent working...]

┌─ Subagent Result ─────────────────────────────┐
│ Found 3 relevant files:                       │
│ • src/auth/errors.ts (AuthError class)        │
│ • src/middleware/auth.ts (error handler)      │
│ • src/utils/auth-helpers.ts (validation)      │
└───────────────────────────────────────────────┘
```

### Parallel Execution
```
Agent: I'll search for both authentication and authorization code.
[Launching 2 Explore agents in parallel...]

┌─ Agent 1: Auth ───────────────────────────────┐
│ Status: Completed                             │
│ Found: 5 files                                │
└───────────────────────────────────────────────┘

┌─ Agent 2: Authorization ──────────────────────┐
│ Status: Completed                             │
│ Found: 3 files                                │
└───────────────────────────────────────────────┘
```

### Background Task
```
Agent: I'll start a long-running analysis in the background.
[Task running in background: agent-abc123]

You can continue working. Use /task-status abc123 to check progress.
```

## Alternatives Considered

### Alternative 1: Single Agent with Mode Switching
Switch modes instead of spawning agents.

**Pros**: Simpler, shared context
**Cons**: Context pollution, no parallelism
**Decision**: Rejected - Context isolation is key benefit

### Alternative 2: Process-Based Workers
Spawn actual OS processes.

**Pros**: True isolation, crash protection
**Cons**: Overhead, complexity, IPC
**Decision**: Rejected for initial implementation

### Alternative 3: Fixed Agent Types Only
No custom agent configurations.

**Pros**: Simpler
**Cons**: Less flexibility
**Decision**: Rejected - Custom agents enable future extensibility

## Security Considerations

1. **Tool Restrictions**: Strictly enforce tool allowlists
2. **Resource Limits**: Limit max turns and execution time
3. **Output Isolation**: Subagent can't access main context
4. **Permission Inheritance**: Subagents respect main permission settings
5. **Background Cleanup**: Clean up stale background tasks

## Testing Strategy

1. **Unit Tests**:
   - Tool filtering logic
   - Config validation
   - Result formatting

2. **Integration Tests**:
   - Subagent lifecycle
   - Background execution
   - Resume functionality

3. **Manual Testing**:
   - Various agent types
   - Parallel execution
   - Complex multi-step tasks

## Migration Path

1. **Phase 1**: Core Explore and Plan agents
2. **Phase 2**: Bash agent and background execution
3. **Phase 3**: Resume capability
4. **Phase 4**: Custom agent definitions
5. **Phase 5**: Parallel execution optimization

No breaking changes to existing functionality.

## References

- [Claude Code Task Tool Documentation](https://code.claude.com/docs/en/tools)
- [Claude Code Subagent System](https://github.com/Piebald-AI/claude-code-system-prompts)
- [Understanding Claude Code Full Stack](https://alexop.dev/posts/understanding-claude-code-full-stack/)
