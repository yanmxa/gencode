# Proposal: Background Tasks & TaskOutput

- **Proposal ID**: 0017
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a comprehensive background task system with a TaskOutput tool for retrieving results from asynchronous operations. This enables non-blocking execution of long-running tasks like builds, tests, and subagent operations.

## Motivation

Currently, mycode executes all operations synchronously, blocking the conversation:

1. **Blocking execution**: Long builds freeze the conversation
2. **No parallel work**: Can't work while tests run
3. **Timeout issues**: Long tasks may timeout
4. **Poor UX**: User waits with no progress indication
5. **Lost results**: Can't check output of past commands

A background task system enables async operations with result retrieval.

## Claude Code Reference

Claude Code provides TaskOutput tool for background task management:

### TaskOutput Tool
```typescript
TaskOutput({
  task_id: "agent-abc123",
  block: true,      // Wait for completion (default: true)
  timeout: 30000    // Max wait time in ms
})
```

### Background Execution Pattern
```typescript
// Launch background task
Task({
  description: "Run tests",
  prompt: "Execute the full test suite",
  subagent_type: "Bash",
  run_in_background: true
})
// Returns: { task_id: "agent-abc123", output_file: "/path/to/output.log" }

// Check status later
TaskOutput({
  task_id: "agent-abc123",
  block: false  // Non-blocking check
})
// Returns current status and partial output
```

### Key Characteristics
- Works with Task tool and Bash tool
- Supports blocking and non-blocking modes
- Configurable timeout up to 10 minutes
- Returns task status and output
- Available via /tasks command

## Detailed Design

### API Design

```typescript
// src/tasks/types.ts
type TaskStatus = 'pending' | 'running' | 'completed' | 'error' | 'cancelled';
type TaskType = 'bash' | 'agent' | 'remote';

interface BackgroundTask {
  id: string;
  type: TaskType;
  description: string;
  status: TaskStatus;
  started_at: Date;
  completed_at?: Date;
  output_file: string;
  exit_code?: number;
  error?: string;
  metadata?: Record<string, unknown>;
}

interface TaskOutputInput {
  task_id: string;
  block?: boolean;         // Wait for completion (default: true)
  timeout?: number;        // Max wait time in ms (default: 30000, max: 600000)
}

interface TaskOutputOutput {
  success: boolean;
  task_id: string;
  status: TaskStatus;
  output?: string;         // Full or partial output
  exit_code?: number;
  error?: string;
  elapsed_ms?: number;     // Time since task started
  truncated?: boolean;     // Output was truncated
}
```

```typescript
// src/tools/task-output/task-output-tool.ts
const taskOutputTool: Tool<TaskOutputInput> = {
  name: 'TaskOutput',
  description: `Retrieve output from a running or completed background task.

Parameters:
- task_id: The ID of the background task
- block: If true (default), wait for task completion
- timeout: Maximum wait time in milliseconds (default: 30000, max: 600000)

Use block=false for non-blocking status check.
Use /tasks command to see all running/completed tasks.

Returns task status, output content, and exit code if completed.
`,
  parameters: z.object({
    task_id: z.string(),
    block: z.boolean().optional().default(true),
    timeout: z.number().min(0).max(600000).optional().default(30000)
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

```typescript
// src/tasks/task-manager.ts
class TaskManager {
  private tasks: Map<string, BackgroundTask> = new Map();
  private processes: Map<string, ChildProcess> = new Map();
  private outputDir: string;

  constructor(outputDir = '~/.mycode/tasks') {
    this.outputDir = expandPath(outputDir);
    fs.mkdirSync(this.outputDir, { recursive: true });
    this.loadExistingTasks();
  }

  async createBashTask(command: string, description: string): Promise<BackgroundTask> {
    const id = generateTaskId('bash');
    const outputFile = path.join(this.outputDir, `${id}.log`);

    const task: BackgroundTask = {
      id,
      type: 'bash',
      description,
      status: 'running',
      started_at: new Date(),
      output_file: outputFile
    };

    // Spawn process
    const child = spawn('bash', ['-c', command], {
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe']
    });

    // Setup output logging
    const logStream = fs.createWriteStream(outputFile);
    child.stdout?.pipe(logStream);
    child.stderr?.pipe(logStream);

    // Track completion
    child.on('exit', (code) => {
      task.status = code === 0 ? 'completed' : 'error';
      task.completed_at = new Date();
      task.exit_code = code ?? -1;
      this.saveTasks();
    });

    this.tasks.set(id, task);
    this.processes.set(id, child);
    this.saveTasks();

    return task;
  }

  async createAgentTask(
    subagentType: string,
    prompt: string,
    description: string
  ): Promise<BackgroundTask> {
    const id = generateTaskId('agent');
    const outputFile = path.join(this.outputDir, `${id}.json`);

    const task: BackgroundTask = {
      id,
      type: 'agent',
      description,
      status: 'running',
      started_at: new Date(),
      output_file: outputFile,
      metadata: { subagentType, prompt }
    };

    // Run agent in separate context
    this.runAgentAsync(id, subagentType, prompt, outputFile);

    this.tasks.set(id, task);
    this.saveTasks();

    return task;
  }

  async getOutput(
    taskId: string,
    options: { block: boolean; timeout: number }
  ): Promise<TaskOutputOutput> {
    const task = this.tasks.get(taskId);
    if (!task) {
      return {
        success: false,
        task_id: taskId,
        status: 'error',
        error: 'Task not found'
      };
    }

    // If blocking, wait for completion
    if (options.block && task.status === 'running') {
      await this.waitForCompletion(taskId, options.timeout);
    }

    // Read output
    const output = await this.readOutput(task.output_file);

    return {
      success: true,
      task_id: taskId,
      status: task.status,
      output,
      exit_code: task.exit_code,
      elapsed_ms: Date.now() - task.started_at.getTime()
    };
  }

  private async waitForCompletion(taskId: string, timeout: number): Promise<void> {
    const start = Date.now();
    const task = this.tasks.get(taskId);
    if (!task) return;

    while (task.status === 'running' && Date.now() - start < timeout) {
      await new Promise(r => setTimeout(r, 500));
    }
  }

  async cancelTask(taskId: string): Promise<boolean> {
    const task = this.tasks.get(taskId);
    const process = this.processes.get(taskId);

    if (!task || task.status !== 'running') return false;

    if (process && !process.killed) {
      process.kill('SIGTERM');
    }

    task.status = 'cancelled';
    task.completed_at = new Date();
    this.saveTasks();

    return true;
  }

  listTasks(filter?: { status?: TaskStatus; type?: TaskType }): BackgroundTask[] {
    let tasks = Array.from(this.tasks.values());

    if (filter?.status) {
      tasks = tasks.filter(t => t.status === filter.status);
    }
    if (filter?.type) {
      tasks = tasks.filter(t => t.type === filter.type);
    }

    return tasks.sort((a, b) => b.started_at.getTime() - a.started_at.getTime());
  }

  cleanup(maxAge: number = 24 * 60 * 60 * 1000): number {
    const cutoff = Date.now() - maxAge;
    let removed = 0;

    for (const [id, task] of this.tasks) {
      if (task.status !== 'running' && task.started_at.getTime() < cutoff) {
        // Delete output file
        try { fs.unlinkSync(task.output_file); } catch {}
        this.tasks.delete(id);
        removed++;
      }
    }

    this.saveTasks();
    return removed;
  }
}

export const taskManager = new TaskManager();
```

### CLI Command: /tasks

```typescript
// src/cli/commands/tasks.ts
function tasksCommand(args: string[]): void {
  const tasks = taskManager.listTasks();

  if (tasks.length === 0) {
    console.log('No background tasks.');
    return;
  }

  console.log('\nBackground Tasks:');
  console.log('─'.repeat(70));

  for (const task of tasks) {
    const elapsed = formatDuration(Date.now() - task.started_at.getTime());
    const statusIcon = {
      running: '⏳',
      completed: '✓',
      error: '✗',
      cancelled: '⊘',
      pending: '○'
    }[task.status];

    console.log(`${statusIcon} ${task.id.padEnd(15)} ${task.description.slice(0, 30).padEnd(32)} ${elapsed}`);
  }

  console.log('─'.repeat(70));
  console.log('Use TaskOutput tool to get task output.');
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tasks/types.ts` | Create | Task type definitions |
| `src/tasks/task-manager.ts` | Create | Task lifecycle management |
| `src/tasks/index.ts` | Create | Module exports |
| `src/tools/task-output/task-output-tool.ts` | Create | TaskOutput tool |
| `src/tools/task-output/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register TaskOutput tool |
| `src/cli/commands/tasks.ts` | Create | /tasks command |
| `src/cli/index.ts` | Modify | Register /tasks command |

## User Experience

### Launch Background Task
```
User: Run the tests but don't wait for them

Agent: I'll start the tests in the background.

[Bash: command="npm test", run_in_background=true]

┌─ Background Task Started ─────────────────────────┐
│ Task ID: bash-a1b2c3d4                           │
│ Command: npm test                                 │
│ Status: Running                                   │
│ Output: ~/.mycode/tasks/bash-a1b2c3d4.log        │
└───────────────────────────────────────────────────┘

You can continue working. Use /tasks to check status.
```

### Check Task Status (Non-blocking)
```
User: How are those tests doing?

Agent: [TaskOutput: task_id="bash-a1b2c3d4", block=false]

┌─ Task Status ─────────────────────────────────────┐
│ Task ID: bash-a1b2c3d4                           │
│ Status: Running (2m 15s elapsed)                  │
│ Recent Output:                                    │
│   PASS src/tools/edit.test.ts                    │
│   PASS src/tools/glob.test.ts                    │
│   Running: src/agent/agent.test.ts...            │
└───────────────────────────────────────────────────┘

Tests are still running (45/60 passed so far).
```

### Wait for Completion
```
Agent: [TaskOutput: task_id="bash-a1b2c3d4", block=true, timeout=60000]

┌─ Task Completed ──────────────────────────────────┐
│ Task ID: bash-a1b2c3d4                           │
│ Status: Completed                                 │
│ Exit Code: 0                                      │
│ Duration: 3m 42s                                  │
│ Summary:                                          │
│   60 tests passed                                 │
│   0 tests failed                                  │
│   Coverage: 87.3%                                 │
└───────────────────────────────────────────────────┘
```

### List All Tasks
```
User: /tasks

Background Tasks:
┌────────────────────────────────────────────────────────────────┐
│ Status  ID              Description              Elapsed      │
├────────────────────────────────────────────────────────────────┤
│ ✓       bash-a1b2c3d4   Run npm test             3m 42s       │
│ ⏳      agent-e5f6g7h8  Explore auth code        45s          │
│ ✗       bash-i9j0k1l2   Build docker image       12m 3s       │
└────────────────────────────────────────────────────────────────┘
```

## Alternatives Considered

### Alternative 1: Promise-based Only
Use JavaScript promises without task registry.

**Pros**: Simpler implementation
**Cons**: No persistence, no /tasks visibility
**Decision**: Rejected - Registry provides better UX

### Alternative 2: Worker Threads
Use Node.js worker threads.

**Pros**: Better isolation
**Cons**: More complex, limited shell access
**Decision**: Deferred - Can add for CPU-intensive tasks

### Alternative 3: External Task Queue
Use Redis/BullMQ for task management.

**Pros**: Robust, distributed
**Cons**: Heavy dependency, overkill for CLI
**Decision**: Rejected - Keep it simple

## Security Considerations

1. **Task Isolation**: Each task runs in separate process
2. **Output Sanitization**: Don't expose sensitive data in logs
3. **Resource Limits**: Limit concurrent tasks and output size
4. **Cleanup**: Regular cleanup of old task files
5. **Permission**: Only access own tasks

```typescript
const LIMITS = {
  maxConcurrentTasks: 10,
  maxOutputSize: 10 * 1024 * 1024,  // 10MB
  taskRetention: 24 * 60 * 60 * 1000  // 24 hours
};
```

## Testing Strategy

1. **Unit Tests**:
   - Task creation and tracking
   - Output retrieval
   - Blocking vs non-blocking
   - Timeout handling

2. **Integration Tests**:
   - Background bash execution
   - Agent task execution
   - Concurrent tasks
   - Cleanup logic

3. **Manual Testing**:
   - Long-running tasks
   - Task cancellation
   - /tasks command

## Migration Path

1. **Phase 1**: Basic TaskManager and TaskOutput tool
2. **Phase 2**: /tasks CLI command
3. **Phase 3**: Agent background execution
4. **Phase 4**: Task persistence across sessions
5. **Phase 5**: Task notifications on completion

Integrates with KillShell (0016) and Enhanced Bash (0028).

## References

- [Node.js Child Process](https://nodejs.org/api/child_process.html)
- [Claude Code TaskOutput Documentation](https://code.claude.com/docs/en/tools)
- [BullMQ - Node.js Job Queue](https://docs.bullmq.io/)
