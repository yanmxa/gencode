# Proposal: Hooks System

- **Proposal ID**: 0009
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement an event-driven hooks system that allows users to register shell commands or scripts to run automatically in response to specific events (tool execution, prompt submission, session events). This enables automated workflows, quality gates, and custom integrations.

## Motivation

Currently, mycode has no extensibility for automated actions. This limits:

1. **Quality enforcement**: Can't auto-run linters after edits
2. **Notifications**: Can't alert users on completion
3. **Integration**: Can't trigger external systems
4. **Automation**: Can't enforce coding standards automatically
5. **Customization**: No way to extend agent behavior

A hooks system enables powerful automation and customization.

## Claude Code Reference

Claude Code provides a comprehensive hooks system:

### Configuration
```json
// .claude/settings.json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write",
        "type": "command",
        "command": ".claude/hooks/run-linter.sh $FILE_PATH"
      }
    ],
    "Notification": [
      {
        "type": "command",
        "command": "afplay ~/.claude/sounds/notify.mp3"
      }
    ]
  }
}
```

### Available Events
| Event | Description | Context |
|-------|-------------|---------|
| `PreToolUse` | Before tool execution | Tool name, inputs |
| `PostToolUse` | After tool execution | Tool name, inputs, result |
| `UserPromptSubmit` | User sends message | Prompt text |
| `Notification` | Agent needs attention | Message |
| `Stop` | Agent stops working | Reason |
| `SessionStart` | Session begins | Session info |

### Hook Types
1. **Command Mode**: Run shell command directly
2. **Prompt Mode**: Let LLM decide actions (advanced)

### Matcher Patterns
- Simple string: `"Edit"` - matches Edit tool
- Regex: `"Edit|Write"` - matches Edit or Write
- Wildcard: `"*"` - matches all

### Environment Variables
Hooks receive context via environment:
- `$TOOL_NAME` - Name of the tool
- `$FILE_PATH` - Path of affected file
- `$EXIT_CODE` - Tool exit code
- `$SESSION_ID` - Current session

## Detailed Design

### API Design

```typescript
// src/hooks/types.ts
type HookEvent =
  | 'PreToolUse'
  | 'PostToolUse'
  | 'UserPromptSubmit'
  | 'Notification'
  | 'Stop'
  | 'SessionStart'
  | 'SubagentStop';

type HookType = 'command' | 'prompt';

interface HookDefinition {
  matcher?: string | RegExp;  // For tool events
  type: HookType;
  command?: string;           // For command type
  prompt?: string;            // For prompt type
  timeout?: number;           // Max execution time
  blocking?: boolean;         // Wait for completion
}

interface HooksConfig {
  [event: string]: HookDefinition[];
}

interface HookContext {
  event: HookEvent;
  toolName?: string;
  toolInput?: Record<string, any>;
  toolResult?: any;
  prompt?: string;
  sessionId: string;
  cwd: string;
}

interface HookResult {
  success: boolean;
  output?: string;
  error?: string;
  blocked?: boolean;   // Hook blocked the action
  message?: string;    // Message to show user
}
```

```typescript
// src/hooks/hooks-manager.ts
class HooksManager {
  private config: HooksConfig;

  constructor(config: HooksConfig);

  // Register hooks from settings
  loadFromSettings(settings: Settings): void;

  // Trigger hooks for an event
  async trigger(event: HookEvent, context: HookContext): Promise<HookResult[]>;

  // Check if event has hooks
  hasHooks(event: HookEvent): boolean;

  // Get hooks for event
  getHooks(event: HookEvent): HookDefinition[];

  // Add hook programmatically
  addHook(event: HookEvent, hook: HookDefinition): void;

  // Remove hook
  removeHook(event: HookEvent, index: number): void;
}
```

### Implementation Approach

1. **Configuration Loading**: Parse hooks from settings.json
2. **Event Integration**: Hook into tool execution, prompts, etc.
3. **Matcher Evaluation**: Match hooks to relevant events
4. **Command Execution**: Run shell commands with context
5. **Environment Setup**: Pass context via environment variables
6. **Result Handling**: Process hook output and blocking

```typescript
// Hook execution
async function executeHook(hook: HookDefinition, context: HookContext): Promise<HookResult> {
  if (hook.type === 'command') {
    // Build environment
    const env = {
      ...process.env,
      TOOL_NAME: context.toolName || '',
      FILE_PATH: context.toolInput?.file_path || '',
      SESSION_ID: context.sessionId,
      CWD: context.cwd,
    };

    // Expand variables in command
    const command = expandVariables(hook.command, context);

    // Execute with timeout
    const result = await execWithTimeout(command, {
      env,
      cwd: context.cwd,
      timeout: hook.timeout || 30000
    });

    return {
      success: result.exitCode === 0,
      output: result.stdout,
      error: result.stderr,
      blocked: result.exitCode !== 0 && hook.blocking
    };
  }

  // Prompt mode - more complex, uses LLM
  // ...
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/hooks/types.ts` | Create | Hook type definitions |
| `src/hooks/hooks-manager.ts` | Create | Core hooks management |
| `src/hooks/executor.ts` | Create | Hook execution logic |
| `src/hooks/index.ts` | Create | Module exports |
| `src/agent/agent.ts` | Modify | Integrate hook triggers |
| `src/tools/registry.ts` | Modify | Tool execution hooks |
| `src/config/settings-manager.ts` | Modify | Load hooks config |

## User Experience

### Configuration
Users configure hooks in `.mycode/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "type": "command",
        "command": "npm run lint:fix $FILE_PATH",
        "blocking": false
      }
    ],
    "Stop": [
      {
        "type": "command",
        "command": "afplay ~/.mycode/sounds/done.mp3"
      }
    ]
  }
}
```

### Hook Execution Display
```
Agent: [Write] Created src/component.tsx

Running hooks...
  ▶ PostToolUse: npm run lint:fix src/component.tsx
  ✓ Lint completed (2 issues fixed)
```

### Blocking Hook
```
Agent: [Edit] Modified src/auth.ts

Running hooks...
  ▶ PostToolUse: npm test -- --related src/auth.ts
  ✗ Tests failed (2 failures)

Hook blocked further execution. Please fix the failing tests.
```

### Hook Status
```
> /hooks

Active Hooks:
┌────────────────────┬─────────────────────────────────────┐
│ Event              │ Action                              │
├────────────────────┼─────────────────────────────────────┤
│ PostToolUse        │ npm run lint:fix (Edit|Write)       │
│ Stop               │ afplay done.mp3                     │
│ SessionStart       │ echo "Welcome!"                     │
└────────────────────┴─────────────────────────────────────┘
```

## Alternatives Considered

### Alternative 1: Plugin-Based Hooks
Hooks as JavaScript/TypeScript modules.

**Pros**: More powerful, type-safe
**Cons**: Requires JS knowledge, security concerns
**Decision**: Deferred - start with shell commands

### Alternative 2: Built-in Actions Only
Predefined hook actions (lint, test, notify).

**Pros**: Simpler, safer
**Cons**: Limited flexibility
**Decision**: Rejected - custom commands are essential

### Alternative 3: Webhook-Based
HTTP webhooks instead of shell commands.

**Pros**: Remote integration
**Cons**: Complexity, security, network dependency
**Decision**: Deferred - can add alongside shell hooks

## Security Considerations

1. **Command Injection**: Sanitize context values in commands
2. **Timeout Enforcement**: Prevent runaway hooks
3. **Resource Limits**: Limit concurrent hook executions
4. **Path Restrictions**: Consider limiting executable paths
5. **Output Limits**: Truncate large hook outputs
6. **Privilege Escalation**: Hooks run with user's permissions

```typescript
// Safe variable expansion
function expandVariables(command: string, context: HookContext): string {
  return command
    .replace(/\$FILE_PATH/g, shellescape([context.toolInput?.file_path || '']))
    .replace(/\$TOOL_NAME/g, shellescape([context.toolName || '']))
    // ... etc
}
```

## Testing Strategy

1. **Unit Tests**:
   - Matcher evaluation
   - Environment building
   - Variable expansion

2. **Integration Tests**:
   - Hook triggering from tools
   - Blocking behavior
   - Multiple hooks

3. **Manual Testing**:
   - Various hook configurations
   - Error handling
   - Timeout behavior

## Migration Path

1. **Phase 1**: Basic PostToolUse and Stop hooks
2. **Phase 2**: All event types
3. **Phase 3**: Blocking hooks
4. **Phase 4**: /hooks command and UI
5. **Phase 5**: Prompt-mode hooks

No breaking changes to existing functionality.

## References

- [Claude Code Hooks Documentation](https://code.claude.com/docs/en/hooks)
- [Understanding Claude Code Full Stack](https://alexop.dev/posts/understanding-claude-code-full-stack/)
- [Git Hooks](https://git-scm.com/docs/githooks) (similar concept)
