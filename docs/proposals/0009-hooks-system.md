# Proposal: Hooks System

- **Proposal ID**: 0009
- **Author**: mycode team
- **Status**: Implemented
- **Created**: 2025-01-15
- **Updated**: 2026-01-18
- **Implemented**: 2026-01-18 (Phase 1), 2026-01-18 (Phase 2)

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

## Implementation Notes

### Phase 1 Implementation (2026-01-18)

**Status**: ✅ Core infrastructure implemented and tested

Phase 1 focused on building the core hooks system without integrating into the agent loop, to avoid conflicts with the ongoing Task Tool (Proposal 0003) implementation.

### Files Created

| File | Description |
|------|-------------|
| `src/hooks/types.ts` | Type definitions for hooks system |
| `src/hooks/executor.ts` | Hook execution engine with bash command support |
| `src/hooks/matcher.ts` | Pattern matching for filtering hooks by tool name |
| `src/hooks/hooks-manager.ts` | Core orchestration class for hook management |
| `src/hooks/utils.ts` | Utility functions for variable expansion, validation, formatting |
| `src/hooks/index.ts` | Module exports |

### Files Modified

| File | Changes |
|------|---------|
| `src/config/types.ts` | Added `hooks` field to Settings interface |

### Tests Created

| Test File | Tests | Description |
|-----------|-------|-------------|
| `tests/hooks/matcher.test.ts` | 13 tests | Pattern matching functionality |
| `tests/hooks/utils.test.ts` | 25 tests | Utility functions |
| `tests/hooks/hooks-manager.test.ts` | 10 tests | HooksManager orchestration |
| `tests/hooks/executor.test.ts` | 9 tests | Hook execution |

**Total**: 57 tests, all passing ✅

### Key Implementation Details

1. **Executor Pattern**: Uses Node.js `spawn()` with proper timeout handling, stdio collection, and exit code interpretation (0=success, 2=block, other=warn)

2. **Environment Variables**: Hooks receive context through both environment variables ($TOOL_NAME, $FILE_PATH, etc.) and JSON via stdin

3. **Pattern Matching**: Supports wildcard ("*"), exact match, and regex patterns for tool filtering

4. **Type Safety**: Full TypeScript implementation with comprehensive type definitions

5. **Testing**: Comprehensive unit test coverage with integration tests for real command execution

### Configuration Example

Users can configure hooks in `.gen/settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "echo 'File modified: $FILE_PATH'",
            "timeout": 5000
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "afplay ~/.gen/sounds/done.mp3"
          }
        ]
      }
    ]
  }
}
```

### Phase 2 - Integration (Completed 2026-01-18)

**Status**: ✅ Fully integrated into agent loop

Phase 2 completed with full integration:
1. ✅ Integrated hooks into agent loop (`src/agent/agent.ts`)
2. ✅ Added PreToolUse hook trigger point (with blocking support)
3. ✅ Added PostToolUse hook trigger point
4. ✅ Added PostToolUseFailure hook trigger point
5. ✅ Added SessionStart hooks (startSession + resumeSession)
6. ✅ Added Stop hooks (conversation end)
7. ✅ Integrated into CLI (`src/cli/index.tsx`, `src/cli/components/App.tsx`)
8. ✅ Created integration tests (`tests/integration/hooks-integration.test.ts`)

**Modified Files:**
| File | Changes |
|------|---------|
| `src/agent/agent.ts` | Added HooksManager, trigger points for all events, blocking logic |
| `src/cli/components/App.tsx` | Added hooksConfig prop and initialization |
| `src/cli/index.tsx` | Pass hooks config from settings to App |
| `tests/integration/hooks-integration.test.ts` | Integration tests with real agent |
| `tests/config/hooks-config.test.ts` | Tests for Claude Code fallback and merge behavior (7 tests) |

**Key Features:**
- **Blocking Hooks**: PreToolUse hooks with exit code 2 block tool execution
- **Event Coverage**: All tool events (Pre/Post/Failure) + session events (Start/Stop)
- **Automatic Loading**: Hooks config loaded from `.gen/settings.json` on CLI startup
- **Full Context**: Hooks receive session ID, tool name, input, result, timestamps

### Compatibility Notes

- **No Breaking Changes**: Phase 1 adds new infrastructure without modifying existing functionality
- **Claude Code Compatible**: Settings format matches Claude Code's hooks configuration
- **Configuration Fallback**: Hooks config supports Claude Code fallback mechanism
  - Loads from both `.gen/settings.json` and `.claude/settings.json`
  - `.gen` settings take priority over `.claude` at the same level
  - When `.gen` has no hooks, automatically falls back to `.claude` hooks
  - Hooks from both sources are merged (arrays concatenated, objects deep-merged)
  - Allows seamless migration from Claude Code while enabling GenCode-specific customizations
- **Extensible**: Easy to add new hook types (prompt hooks) and events in future

### Performance Considerations

- Hooks run with configurable timeouts (default: 60s)
- Parallel execution by default (configurable)
- Output truncation at 30KB to prevent memory issues
- Proper process cleanup with SIGTERM → SIGKILL escalation

### Security Considerations

- Uses `spawn()` with array arguments (not shell string concatenation)
- Environment variables are properly escaped
- Path sanitization prevents traversal attacks
- Sensitive directories are blocked (/etc, /var, /sys, /proc, /dev)
- Commands run with user's permissions (no privilege escalation)
