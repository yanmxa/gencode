# Proposal: Sandboxing

- **Proposal ID**: 0034
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement sandboxing capabilities to run untrusted code and commands in isolated environments, protecting the host system from potentially harmful operations.

## Motivation

Code execution carries risks:

1. **Malicious commands**: Intentional or accidental damage
2. **Side effects**: Unintended system changes
3. **Resource exhaustion**: CPU/memory abuse
4. **Network access**: Unauthorized connections
5. **File system**: Access to sensitive files

Sandboxing provides isolation and safety.

## Detailed Design

### API Design

```typescript
// src/sandbox/types.ts
type SandboxType = 'docker' | 'firejail' | 'nsjail' | 'vm';

interface SandboxConfig {
  type: SandboxType;
  enabled: boolean;
  image?: string;           // For Docker
  profile?: string;         // For firejail
  limits: ResourceLimits;
  network: NetworkPolicy;
  filesystem: FilesystemPolicy;
}

interface ResourceLimits {
  cpuPercent: number;       // 0-100
  memoryMB: number;
  diskMB: number;
  timeoutSeconds: number;
  maxProcesses: number;
}

interface NetworkPolicy {
  enabled: boolean;
  allowedHosts?: string[];
  blockedPorts?: number[];
}

interface FilesystemPolicy {
  readOnly: string[];       // Read-only paths
  readWrite: string[];      // Read-write paths
  hidden: string[];         // Inaccessible paths
  tempDir: string;
}

interface SandboxResult {
  success: boolean;
  stdout: string;
  stderr: string;
  exitCode: number;
  resourceUsage: {
    cpuTime: number;
    memoryPeak: number;
    wallTime: number;
  };
}
```

### Sandbox Manager

```typescript
// src/sandbox/manager.ts
class SandboxManager {
  private config: SandboxConfig;
  private backend: SandboxBackend;

  constructor(config?: Partial<SandboxConfig>) {
    this.config = {
      type: 'docker',
      enabled: false,
      limits: {
        cpuPercent: 50,
        memoryMB: 512,
        diskMB: 1024,
        timeoutSeconds: 60,
        maxProcesses: 50
      },
      network: { enabled: false },
      filesystem: {
        readOnly: ['/'],
        readWrite: ['/tmp'],
        hidden: ['/etc/passwd', '/etc/shadow'],
        tempDir: '/tmp/mycode-sandbox'
      },
      ...config
    };

    this.backend = this.createBackend();
  }

  async execute(command: string, options?: ExecuteOptions): Promise<SandboxResult> {
    if (!this.config.enabled) {
      throw new Error('Sandbox not enabled');
    }

    return this.backend.execute(command, {
      ...options,
      limits: this.config.limits,
      network: this.config.network,
      filesystem: this.config.filesystem
    });
  }

  private createBackend(): SandboxBackend {
    switch (this.config.type) {
      case 'docker':
        return new DockerSandbox(this.config);
      case 'firejail':
        return new FirejailSandbox(this.config);
      case 'nsjail':
        return new NsjailSandbox(this.config);
      default:
        throw new Error(`Unknown sandbox type: ${this.config.type}`);
    }
  }
}

// Docker implementation
class DockerSandbox implements SandboxBackend {
  async execute(command: string, options: SandboxOptions): Promise<SandboxResult> {
    const containerArgs = [
      'run', '--rm',
      '--network', options.network.enabled ? 'bridge' : 'none',
      '--memory', `${options.limits.memoryMB}m`,
      '--cpus', `${options.limits.cpuPercent / 100}`,
      '--pids-limit', String(options.limits.maxProcesses),
      '--read-only',
      '-v', `${options.filesystem.tempDir}:/workspace:rw`,
      '-w', '/workspace',
      this.config.image || 'ubuntu:22.04',
      '/bin/bash', '-c', command
    ];

    const result = await execAsync(`docker ${containerArgs.join(' ')}`);

    return {
      success: result.code === 0,
      stdout: result.stdout,
      stderr: result.stderr,
      exitCode: result.code,
      resourceUsage: await this.getResourceUsage()
    };
  }
}
```

### Bash Tool Integration

```typescript
// Updated src/tools/bash/bash-tool.ts
async execute(input: BashInput, context: ToolContext): Promise<BashOutput> {
  if (this.config.sandboxMode && sandboxManager.isEnabled()) {
    return this.executeInSandbox(input, context);
  }
  return this.executeDirect(input, context);
}

private async executeInSandbox(
  input: BashInput,
  context: ToolContext
): Promise<BashOutput> {
  const result = await sandboxManager.execute(input.command, {
    cwd: context.cwd,
    timeout: input.timeout
  });

  return {
    success: result.success,
    stdout: result.stdout,
    stderr: result.stderr,
    exit_code: result.exitCode,
    duration_ms: result.resourceUsage.wallTime
  };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/sandbox/types.ts` | Create | Type definitions |
| `src/sandbox/manager.ts` | Create | Sandbox management |
| `src/sandbox/backends/docker.ts` | Create | Docker backend |
| `src/sandbox/backends/firejail.ts` | Create | Firejail backend |
| `src/sandbox/index.ts` | Create | Module exports |
| `src/tools/bash/bash-tool.ts` | Modify | Sandbox integration |

## User Experience

### Enable Sandbox Mode
```
User: /settings sandbox enable

Sandbox mode enabled.
  Backend: Docker
  Memory limit: 512MB
  CPU limit: 50%
  Network: Disabled
  Timeout: 60s

Commands will now run in isolated containers.
```

### Sandboxed Execution
```
Agent: [Bash: npm install && npm test (sandboxed)]

┌─ Sandbox Execution ───────────────────────────────┐
│ Container: mycode-sandbox-abc123                 │
│ Image: node:18-slim                               │
│ Network: disabled                                 │
│ Memory: 512MB max                                 │
└───────────────────────────────────────────────────┘

Running...

✓ Completed in 12.3s
  CPU: 34% peak
  Memory: 287MB peak
  Exit code: 0
```

### Resource Limit Hit
```
Agent: [Bash: stress --vm 4 --vm-bytes 1G (sandboxed)]

⚠️ Sandbox Limit Exceeded

The command exceeded resource limits:
  Memory: 512MB limit exceeded

The container was terminated for safety.
```

## Security Considerations

1. Container escape prevention
2. Network isolation verification
3. Resource limit enforcement
4. Secure image sources
5. Cleanup on completion

## Migration Path

1. **Phase 1**: Docker sandbox
2. **Phase 2**: Firejail for lighter isolation
3. **Phase 3**: Resource monitoring
4. **Phase 4**: Network policies
5. **Phase 5**: Custom images

## References

- [Docker Security](https://docs.docker.com/engine/security/)
- [Firejail](https://firejail.wordpress.com/)
- [gVisor](https://gvisor.dev/)
