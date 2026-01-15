# Proposal: KillShell Tool

- **Proposal ID**: 0016
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a KillShell tool that terminates background shell processes spawned by the Bash tool. This enables the agent to manage long-running processes, cancel stuck commands, and clean up resources properly.

## Motivation

With background task execution (Bash with `run_in_background: true`), the agent needs control over running processes:

1. **Stuck processes**: Commands may hang and need termination
2. **Resource cleanup**: Long-running servers need graceful shutdown
3. **Task cancellation**: User may want to abort running operations
4. **Process management**: Track and control multiple background tasks
5. **Memory control**: Prevent runaway processes from consuming resources

A KillShell tool provides explicit control over background process lifecycle.

## Claude Code Reference

Claude Code's KillShell tool works with background Bash commands:

### Tool Definition
```typescript
KillShell({
  shell_id: "bg-abc123"  // ID returned by background Bash command
})
```

### Integration with Bash Tool
```typescript
// Background command returns shell_id
Bash({
  command: "npm run dev",
  run_in_background: true
})
// Returns: { task_id: "bg-abc123", ... }

// Later, terminate the process
KillShell({
  shell_id: "bg-abc123"
})
```

### Key Characteristics
- Works with shell_id from background tasks
- Sends appropriate signals (SIGTERM, then SIGKILL)
- Returns success/failure status
- Available via /tasks command to view running tasks

## Detailed Design

### API Design

```typescript
// src/tools/kill-shell/types.ts
interface KillShellInput {
  shell_id: string;      // ID of the background shell to kill
  signal?: 'SIGTERM' | 'SIGKILL' | 'SIGINT';  // Signal to send (default: SIGTERM)
  force?: boolean;       // Force kill after timeout (default: true)
  timeout?: number;      // Ms to wait before SIGKILL (default: 5000)
}

interface KillShellOutput {
  success: boolean;
  shell_id: string;
  signal_sent: string;
  force_killed?: boolean;   // True if SIGKILL was needed
  exit_code?: number;
  error?: string;
}

// Related: Background shell tracking
interface BackgroundShell {
  id: string;
  pid: number;
  command: string;
  started_at: Date;
  status: 'running' | 'completed' | 'killed' | 'error';
  output_file: string;
}
```

```typescript
// src/tools/kill-shell/kill-shell-tool.ts
const killShellTool: Tool<KillShellInput> = {
  name: 'KillShell',
  description: `Terminate a running background shell process.

Parameters:
- shell_id: The ID of the background shell to kill
  (returned by Bash tool with run_in_background=true)
- signal: Signal to send (SIGTERM, SIGKILL, SIGINT). Default: SIGTERM
- force: If true, send SIGKILL after timeout. Default: true
- timeout: Milliseconds to wait before force kill. Default: 5000

Use /tasks command to see running background shells and their IDs.

Returns success status and whether force kill was required.
`,
  parameters: z.object({
    shell_id: z.string(),
    signal: z.enum(['SIGTERM', 'SIGKILL', 'SIGINT']).optional().default('SIGTERM'),
    force: z.boolean().optional().default(true),
    timeout: z.number().int().positive().optional().default(5000)
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **Shell Registry**: Track all background shells
2. **Process Lookup**: Find process by shell_id
3. **Signal Sending**: Send requested signal
4. **Wait/Force**: Wait for exit or force kill
5. **Cleanup**: Remove from registry, close files

```typescript
// src/tools/kill-shell/shell-registry.ts
class ShellRegistry {
  private shells: Map<string, BackgroundShell> = new Map();

  register(shell: BackgroundShell): void {
    this.shells.set(shell.id, shell);
  }

  get(id: string): BackgroundShell | undefined {
    return this.shells.get(id);
  }

  remove(id: string): boolean {
    return this.shells.delete(id);
  }

  listRunning(): BackgroundShell[] {
    return Array.from(this.shells.values())
      .filter(s => s.status === 'running');
  }

  cleanup(): void {
    // Remove completed/killed shells older than 1 hour
    const cutoff = Date.now() - 3600000;
    for (const [id, shell] of this.shells) {
      if (shell.status !== 'running' && shell.started_at.getTime() < cutoff) {
        this.shells.delete(id);
      }
    }
  }
}

export const shellRegistry = new ShellRegistry();
```

```typescript
// Core kill implementation
async function killShell(input: KillShellInput): Promise<KillShellOutput> {
  const { shell_id, signal = 'SIGTERM', force = true, timeout = 5000 } = input;

  const shell = shellRegistry.get(shell_id);
  if (!shell) {
    return {
      success: false,
      shell_id,
      signal_sent: signal,
      error: `Shell not found: ${shell_id}`
    };
  }

  if (shell.status !== 'running') {
    return {
      success: true,
      shell_id,
      signal_sent: 'none',
      error: `Shell already ${shell.status}`
    };
  }

  try {
    // Send initial signal
    process.kill(shell.pid, signal);
    shell.status = 'killed';

    // Wait for process to exit
    const exited = await waitForExit(shell.pid, timeout);

    if (!exited && force) {
      // Force kill if still running
      process.kill(shell.pid, 'SIGKILL');
      await waitForExit(shell.pid, 1000);

      return {
        success: true,
        shell_id,
        signal_sent: signal,
        force_killed: true
      };
    }

    return {
      success: exited,
      shell_id,
      signal_sent: signal,
      force_killed: false
    };
  } catch (error) {
    // Process may already be dead
    if ((error as NodeJS.ErrnoException).code === 'ESRCH') {
      shell.status = 'completed';
      return {
        success: true,
        shell_id,
        signal_sent: signal,
        error: 'Process already terminated'
      };
    }
    throw error;
  }
}

async function waitForExit(pid: number, timeout: number): Promise<boolean> {
  const start = Date.now();
  while (Date.now() - start < timeout) {
    try {
      process.kill(pid, 0);  // Check if process exists
      await new Promise(r => setTimeout(r, 100));
    } catch {
      return true;  // Process exited
    }
  }
  return false;  // Timeout
}
```

### Enhanced Bash Tool Integration

```typescript
// Update to src/tools/builtin/bash.ts
interface BashInput {
  command: string;
  timeout?: number;
  run_in_background?: boolean;  // NEW
  description?: string;
}

interface BashOutput {
  success: boolean;
  stdout: string;
  stderr: string;
  exit_code: number;
  // NEW: For background tasks
  task_id?: string;
  output_file?: string;
}

async function executeBash(input: BashInput, context: ToolContext): Promise<BashOutput> {
  if (input.run_in_background) {
    return executeInBackground(input, context);
  }
  return executeSync(input, context);
}

async function executeInBackground(
  input: BashInput,
  context: ToolContext
): Promise<BashOutput> {
  const shellId = generateShellId();
  const outputFile = path.join(os.tmpdir(), `mycode-${shellId}.log`);

  const child = spawn('bash', ['-c', input.command], {
    cwd: context.cwd,
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe']
  });

  // Setup output logging
  const logStream = fs.createWriteStream(outputFile);
  child.stdout?.pipe(logStream);
  child.stderr?.pipe(logStream);

  // Register in shell registry
  shellRegistry.register({
    id: shellId,
    pid: child.pid!,
    command: input.command,
    started_at: new Date(),
    status: 'running',
    output_file: outputFile
  });

  // Detach process
  child.unref();

  return {
    success: true,
    stdout: '',
    stderr: '',
    exit_code: -1,
    task_id: shellId,
    output_file: outputFile
  };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/kill-shell/types.ts` | Create | Type definitions |
| `src/tools/kill-shell/kill-shell-tool.ts` | Create | Tool implementation |
| `src/tools/kill-shell/shell-registry.ts` | Create | Background shell tracking |
| `src/tools/kill-shell/index.ts` | Create | Module exports |
| `src/tools/builtin/bash.ts` | Modify | Add background execution |
| `src/tools/index.ts` | Modify | Register KillShell tool |
| `src/cli/commands/tasks.ts` | Create | /tasks command |

## User Experience

### View Running Tasks
```
User: /tasks

Active Background Tasks:
┌────────────────────────────────────────────────────────────┐
│ ID          Command              Started       Status     │
├────────────────────────────────────────────────────────────┤
│ bg-a1b2c3   npm run dev          5 min ago     running    │
│ bg-d4e5f6   pytest --watch       2 min ago     running    │
│ bg-g7h8i9   docker compose up    10 min ago    running    │
└────────────────────────────────────────────────────────────┘

Use KillShell tool with shell_id to terminate a task.
```

### Kill Background Process
```
User: Stop the npm dev server

Agent: I'll terminate the background npm process.

[KillShell: shell_id="bg-a1b2c3"]

┌─ Process Terminated ──────────────────────────────┐
│ Shell ID: bg-a1b2c3                              │
│ Command: npm run dev                              │
│ Signal: SIGTERM                                   │
│ Status: Terminated gracefully                     │
└───────────────────────────────────────────────────┘
```

### Force Kill Hung Process
```
Agent: [KillShell: shell_id="bg-stuck", timeout=3000]

┌─ Process Force Killed ────────────────────────────┐
│ Shell ID: bg-stuck                                │
│ Signal: SIGTERM → SIGKILL                         │
│ Status: Force killed after 3s timeout             │
│ Note: Process did not respond to SIGTERM          │
└───────────────────────────────────────────────────┘
```

### Shell Not Found
```
Agent: [KillShell: shell_id="invalid-id"]

Error: Shell not found: invalid-id
Use /tasks to see available background shells.
```

## Alternatives Considered

### Alternative 1: Kill via Bash
Use `kill` command through Bash tool.

**Pros**: No new tool needed
**Cons**: Requires PID tracking, shell escaping risks
**Decision**: Rejected - Dedicated tool is safer

### Alternative 2: Automatic Cleanup Only
Kill processes automatically on session end.

**Pros**: Simpler, no user action needed
**Cons**: No mid-session control
**Decision**: Rejected - Manual control is essential

### Alternative 3: Process Groups
Use process groups for batch termination.

**Pros**: Kill entire process trees
**Cons**: More complex, may affect other processes
**Decision**: Deferred - Can add later

## Security Considerations

1. **Registry Isolation**: Only kill processes we spawned
2. **PID Validation**: Verify PID still belongs to expected command
3. **No Arbitrary Signals**: Only allow safe signals
4. **Cleanup on Exit**: Kill orphaned processes on mycode exit
5. **User Confirmation**: Consider confirmation for long-running tasks

```typescript
// Safety check before kill
function validateShell(shell: BackgroundShell): boolean {
  try {
    // Verify process exists and matches
    const procCommand = getProcessCommand(shell.pid);
    return procCommand.includes(shell.command.slice(0, 20));
  } catch {
    return false;
  }
}
```

## Testing Strategy

1. **Unit Tests**:
   - Shell registration/lookup
   - Signal sending
   - Timeout handling
   - Force kill logic

2. **Integration Tests**:
   - Background command execution
   - Graceful termination
   - Force kill scenarios
   - Process not found

3. **Manual Testing**:
   - Long-running servers
   - Hung processes
   - Multiple concurrent tasks

## Migration Path

1. **Phase 1**: Basic kill with SIGTERM
2. **Phase 2**: Force kill with timeout
3. **Phase 3**: Shell registry and /tasks command
4. **Phase 4**: Process group support
5. **Phase 5**: Cleanup automation

Requires Enhanced Bash Tool (0028) for full functionality.

## References

- [Node.js Child Process](https://nodejs.org/api/child_process.html)
- [Unix Signals](https://man7.org/linux/man-pages/man7/signal.7.html)
- [Process Groups and Sessions](https://www.win.tue.nl/~aeb/linux/lk/lk-10.html)
- [Claude Code Background Tasks](https://code.claude.com/docs/en/tools)
