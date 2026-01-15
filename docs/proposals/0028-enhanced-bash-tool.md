# Proposal: Enhanced Bash Tool

- **Proposal ID**: 0028
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Enhance the Bash tool with background execution, streaming output, improved timeout handling, and safety features. This enables running long commands without blocking and provides better control over shell operations.

## Motivation

The current Bash tool has limitations:

1. **Blocking only**: Must wait for command completion
2. **No streaming**: Output only shown after completion
3. **Fixed timeout**: 30 second limit is too restrictive
4. **Limited safety**: No command blocking
5. **No working directory persistence**: Each command is isolated

Enhanced Bash enables flexible, safe shell command execution.

## Claude Code Reference

Claude Code's Bash tool provides rich functionality:

### From Tool Description
```
- timeout: Optional timeout in milliseconds (up to 600000 / 10 minutes)
- run_in_background: Run command without waiting for completion
- description: Clear description of what the command does
- Output truncated at 30000 characters
```

### Key Features
- Configurable timeout up to 10 minutes
- Background execution with task ID
- Command description for clarity
- Output truncation
- Sandboxing option

### Safety Guidelines
```
- Quote paths with spaces
- Use absolute paths
- Avoid cd, prefer absolute paths
- Don't use find/grep/cat (use dedicated tools)
- Chain commands with && or ;
```

## Detailed Design

### API Design

```typescript
// src/tools/bash/types.ts
interface BashInput {
  command: string;
  description?: string;          // What this command does
  timeout?: number;              // Ms, default 120000, max 600000
  run_in_background?: boolean;   // Don't wait for completion
  cwd?: string;                  // Working directory override
  env?: Record<string, string>;  // Additional env vars
}

interface BashOutput {
  success: boolean;
  stdout: string;
  stderr: string;
  exit_code: number;
  duration_ms: number;
  truncated?: boolean;
  // For background tasks
  task_id?: string;
  output_file?: string;
}

interface BashConfig {
  defaultTimeout: number;
  maxTimeout: number;
  maxOutputSize: number;
  blockedCommands: string[];
  requireDescription: boolean;
  sandboxMode: boolean;
}

const DEFAULT_CONFIG: BashConfig = {
  defaultTimeout: 120000,    // 2 minutes
  maxTimeout: 600000,        // 10 minutes
  maxOutputSize: 30000,      // 30K characters
  blockedCommands: [
    'rm -rf /',
    'mkfs',
    'dd if=/dev/zero',
    ':(){:|:&};:',           // Fork bomb
  ],
  requireDescription: false,
  sandboxMode: false
};
```

### Enhanced Bash Tool

```typescript
// src/tools/bash/bash-tool.ts
const bashTool: Tool<BashInput> = {
  name: 'Bash',
  description: `Execute shell commands with configurable options.

Parameters:
- command: The command to execute (required)
- description: What this command does (recommended)
- timeout: Timeout in milliseconds (default: 120000, max: 600000)
- run_in_background: Run without waiting (returns task_id)
- cwd: Working directory override
- env: Additional environment variables

Best practices:
- Always quote paths with spaces
- Use absolute paths when possible
- Use && to chain dependent commands
- Use ; to run commands regardless of previous success
- Prefer dedicated tools over: find, grep, cat, head, tail

For long-running commands, use run_in_background: true
and TaskOutput tool to check results.
`,
  parameters: z.object({
    command: z.string().min(1),
    description: z.string().optional(),
    timeout: z.number().min(0).max(600000).optional(),
    run_in_background: z.boolean().optional(),
    cwd: z.string().optional(),
    env: z.record(z.string()).optional()
  }),
  execute: async (input, context) => {
    return bashExecutor.execute(input, context);
  }
};
```

### Bash Executor

```typescript
// src/tools/bash/executor.ts
class BashExecutor {
  private config: BashConfig;
  private taskManager: TaskManager;

  constructor(config?: Partial<BashConfig>) {
    this.config = { ...DEFAULT_CONFIG, ...config };
    this.taskManager = new TaskManager();
  }

  async execute(input: BashInput, context: ToolContext): Promise<BashOutput> {
    // Safety check
    const safetyResult = this.checkSafety(input.command);
    if (!safetyResult.safe) {
      return {
        success: false,
        stdout: '',
        stderr: safetyResult.reason,
        exit_code: -1,
        duration_ms: 0
      };
    }

    // Determine execution mode
    if (input.run_in_background) {
      return this.executeBackground(input, context);
    }

    return this.executeForeground(input, context);
  }

  private async executeForeground(
    input: BashInput,
    context: ToolContext
  ): Promise<BashOutput> {
    const timeout = Math.min(
      input.timeout || this.config.defaultTimeout,
      this.config.maxTimeout
    );

    const cwd = input.cwd || context.cwd;
    const start = Date.now();

    try {
      const { stdout, stderr } = await execAsync(input.command, {
        cwd,
        timeout,
        maxBuffer: this.config.maxOutputSize * 2,
        env: { ...process.env, ...input.env },
        shell: '/bin/bash'
      });

      return {
        success: true,
        stdout: this.truncate(stdout),
        stderr: this.truncate(stderr),
        exit_code: 0,
        duration_ms: Date.now() - start,
        truncated: stdout.length > this.config.maxOutputSize
      };
    } catch (error) {
      const execError = error as ExecException;
      return {
        success: false,
        stdout: this.truncate(execError.stdout || ''),
        stderr: this.truncate(execError.stderr || execError.message),
        exit_code: execError.code || 1,
        duration_ms: Date.now() - start
      };
    }
  }

  private async executeBackground(
    input: BashInput,
    context: ToolContext
  ): Promise<BashOutput> {
    const taskId = generateTaskId('bash');
    const outputFile = path.join(os.tmpdir(), `mycode-${taskId}.log`);
    const cwd = input.cwd || context.cwd;

    const child = spawn('bash', ['-c', input.command], {
      cwd,
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, ...input.env }
    });

    // Setup output logging
    const logStream = fs.createWriteStream(outputFile);
    child.stdout?.pipe(logStream);
    child.stderr?.pipe(logStream);

    // Register with task manager
    this.taskManager.register({
      id: taskId,
      type: 'bash',
      command: input.command,
      description: input.description,
      pid: child.pid!,
      outputFile,
      startedAt: new Date(),
      status: 'running'
    });

    // Handle completion
    child.on('exit', (code) => {
      this.taskManager.complete(taskId, code || 0);
    });

    // Detach
    child.unref();

    return {
      success: true,
      stdout: `Background task started: ${taskId}`,
      stderr: '',
      exit_code: 0,
      duration_ms: 0,
      task_id: taskId,
      output_file: outputFile
    };
  }

  private checkSafety(command: string): { safe: boolean; reason: string } {
    // Check blocked patterns
    for (const blocked of this.config.blockedCommands) {
      if (command.includes(blocked)) {
        return {
          safe: false,
          reason: `Command contains blocked pattern: ${blocked}`
        };
      }
    }

    // Warn about dangerous patterns
    const dangerous = [
      { pattern: /rm\s+-rf\s+\//, reason: 'Recursive delete from root' },
      { pattern: />\s*\/dev\/sd/, reason: 'Writing to block device' },
      { pattern: /chmod\s+-R\s+777/, reason: 'Insecure permissions' },
    ];

    for (const { pattern, reason } of dangerous) {
      if (pattern.test(command)) {
        return { safe: false, reason };
      }
    }

    return { safe: true, reason: '' };
  }

  private truncate(output: string): string {
    if (output.length <= this.config.maxOutputSize) {
      return output;
    }

    const half = Math.floor(this.config.maxOutputSize / 2);
    const truncated = output.length - this.config.maxOutputSize;

    return `${output.slice(0, half)}\n\n... (${truncated} characters truncated) ...\n\n${output.slice(-half)}`;
  }
}

export const bashExecutor = new BashExecutor();
```

### Streaming Support

```typescript
// src/tools/bash/streaming.ts
async function* executeWithStreaming(
  command: string,
  options: SpawnOptions
): AsyncGenerator<StreamEvent> {
  const child = spawn('bash', ['-c', command], {
    ...options,
    stdio: ['ignore', 'pipe', 'pipe']
  });

  // Stream stdout
  if (child.stdout) {
    child.stdout.on('data', (data: Buffer) => {
      // Yield to event loop
    });
  }

  // Stream stderr
  if (child.stderr) {
    child.stderr.on('data', (data: Buffer) => {
      // Yield to event loop
    });
  }

  // Wait for completion
  const exitCode = await new Promise<number>((resolve) => {
    child.on('exit', (code) => resolve(code || 0));
  });

  yield { type: 'exit', code: exitCode };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/bash/types.ts` | Create | Type definitions |
| `src/tools/bash/bash-tool.ts` | Modify | Enhanced tool |
| `src/tools/bash/executor.ts` | Create | Command execution |
| `src/tools/bash/safety.ts` | Create | Safety checks |
| `src/tools/bash/streaming.ts` | Create | Streaming output |
| `src/tools/bash/index.ts` | Modify | Updated exports |
| `src/tasks/task-manager.ts` | Modify | Background task support |

## User Experience

### Foreground with Description
```
Agent: [Bash: command="npm run build", description="Build the project"]

Running: Build the project

✓ Command completed (4.2s)
stdout:
  > mycode@1.0.0 build
  > tsc

  Build completed successfully.
```

### Background Execution
```
Agent: [Bash: command="npm test", run_in_background=true, description="Run tests"]

┌─ Background Task Started ─────────────────────────┐
│ Task ID: bash-abc123                             │
│ Command: npm test                                 │
│ Output: /tmp/mycode-bash-abc123.log              │
│                                                   │
│ Use /tasks to check status                        │
│ Use TaskOutput to get results                     │
└───────────────────────────────────────────────────┘
```

### Streaming Output (Long Command)
```
Agent: [Bash: command="npm install", timeout=300000]

Running: npm install
────────────────────────────────────────────────────
added 1 package in 1s
⠋ Installing dependencies...
added 50 packages in 5s
⠙ Installing dependencies...
added 200 packages in 15s
✓ added 523 packages in 32s
────────────────────────────────────────────────────
Exit code: 0
Duration: 32.4s
```

### Safety Block
```
Agent: [Bash: command="rm -rf /"]

⚠️ Command Blocked

This command matches a blocked pattern:
  "rm -rf /" - Recursive delete from root

This operation is not allowed for safety.
If you need to delete files, specify exact paths.
```

### Output Truncation
```
Agent: [Bash: command="cat large-file.log"]

Output (truncated):
────────────────────────────────────────────────────
[2024-01-15 10:00:00] Starting application...
[2024-01-15 10:00:01] Loading configuration...
...

... (45,678 characters truncated) ...

[2024-01-15 10:15:42] Request completed
[2024-01-15 10:15:43] Shutting down...
────────────────────────────────────────────────────
```

## Alternatives Considered

### Alternative 1: PTY-based Execution
Use pseudo-terminal for full interactivity.

**Pros**: Interactive commands work
**Cons**: Complex, security concerns
**Decision**: Deferred - Consider for advanced use

### Alternative 2: Sandboxed Containers
Run commands in Docker/Podman.

**Pros**: Full isolation
**Cons**: Heavy, requires container runtime
**Decision**: Optional via sandboxMode config

### Alternative 3: PowerShell Support
Add Windows PowerShell support.

**Pros**: Cross-platform
**Cons**: Different syntax, complexity
**Decision**: Deferred - Focus on Bash first

## Security Considerations

1. **Command Blocking**: Block dangerous patterns
2. **Path Injection**: Validate file paths
3. **Environment Isolation**: Don't leak sensitive env vars
4. **Resource Limits**: Limit CPU/memory usage
5. **Timeout Enforcement**: Kill hung processes

```typescript
const SAFETY_PATTERNS = [
  /rm\s+-rf\s+\//,
  /mkfs/,
  /dd\s+if=.*of=\/dev/,
  />\s*\/etc/,
  /curl.*\|\s*bash/,  // Pipe to shell
];
```

## Testing Strategy

1. **Unit Tests**:
   - Command parsing
   - Safety checks
   - Output truncation
   - Timeout handling

2. **Integration Tests**:
   - Background execution
   - Streaming output
   - Error handling

3. **Security Tests**:
   - Blocked commands
   - Injection attempts
   - Resource limits

## Migration Path

1. **Phase 1**: Configurable timeout
2. **Phase 2**: Background execution
3. **Phase 3**: Streaming output
4. **Phase 4**: Enhanced safety
5. **Phase 5**: Sandboxing option

Backward compatible with existing Bash usage.

## References

- [Node.js Child Process](https://nodejs.org/api/child_process.html)
- [Bash Reference Manual](https://www.gnu.org/software/bash/manual/)
- [Shell Command Injection Prevention](https://owasp.org/www-community/attacks/Command_Injection)
- [Container Sandboxing](https://docs.docker.com/engine/security/)
