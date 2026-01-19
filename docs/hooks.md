# Hooks System

The hooks system allows you to automatically execute shell commands in response to events like tool execution, session start/stop, and more. This enables powerful automation workflows such as running linters after file edits, playing sounds on completion, or enforcing quality gates.

## Configuration

Hooks are configured in your settings file (`.gen/settings.json`).

### Basic Example

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "npm run lint:fix $FILE_PATH",
            "timeout": 5000
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'Agent completed'"
          }
        ]
      }
    ]
  }
}
```

## Available Events

| Event | Description | Context Available |
|-------|-------------|-------------------|
| `PreToolUse` | Before tool execution | Tool name, input |
| `PostToolUse` | After successful tool execution | Tool name, input, result |
| `PostToolUseFailure` | After failed tool execution | Tool name, input, error |
| `SessionStart` | When session starts or resumes | Session ID |
| `Stop` | When agent completes | Session ID |

## Hook Configuration

### Matcher Patterns

Filter which tools trigger the hook:

- **Exact match**: `"Write"` - Only matches Write tool
- **Regex pattern**: `"Write|Edit"` - Matches Write or Edit tools
- **Wildcard**: `"*"` or `""` - Matches all tools
- **No matcher needed**: For session events like `SessionStart`, `Stop`

### Hook Properties

```typescript
{
  "type": "command",           // Hook type (only "command" supported currently)
  "command": "echo $TOOL_NAME", // Shell command to execute
  "timeout": 60000,            // Max execution time in ms (default: 60000)
  "blocking": true,            // Wait for completion (default: false)
  "statusMessage": "Running..." // Optional message to display
}
```

### Environment Variables

Hooks receive context through environment variables:

- `$TOOL_NAME` - Name of the tool being executed
- `$FILE_PATH` - File path (for Read, Write, Edit tools)
- `$SESSION_ID` - Current session ID
- `$CWD` - Current working directory

### JSON Context via stdin

Hooks also receive a JSON object via stdin with complete context:

```json
{
  "session_id": "abc123",
  "cwd": "/path/to/project",
  "hook_event_name": "PostToolUse",
  "tool_name": "Write",
  "tool_input": {
    "file_path": "/path/to/file.ts",
    "content": "..."
  }
}
```

## Blocking Hooks

Hooks can block tool execution using exit codes:

- **Exit code 0**: Success, allow the action
- **Exit code 2**: Block the action (only for `PreToolUse`)
- **Other exit codes**: Warning, but don't block

### Example: Block Bash commands

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "exit 2",
            "blocking": true
          }
        ]
      }
    ]
  }
}
```

## Common Use Cases

### Auto-format on File Save

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "prettier --write $FILE_PATH",
            "statusMessage": "Formatting code..."
          }
        ]
      }
    ]
  }
}
```

### Run Tests Before Committing

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "if echo \"$BASH_COMMAND\" | grep -q 'git commit'; then npm test; fi",
            "blocking": true
          }
        ]
      }
    ]
  }
}
```

### Completion Notification

```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "afplay /System/Library/Sounds/Glass.aiff"
          }
        ]
      }
    ]
  }
}
```

### Session Logging

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo \"Session started: $SESSION_ID\" >> ~/.gen/session.log"
          }
        ]
      }
    ]
  }
}
```

## Claude Code Compatibility

GenCode's hooks system is **fully compatible** with Claude Code's configuration format. This means:

### Configuration Fallback

1. GenCode loads hooks from both `.gen/settings.json` and `.claude/settings.json`
2. If `.gen` doesn't have hooks configuration, it automatically falls back to `.claude`
3. If both exist, they are merged together (arrays concatenated, objects deep-merged)
4. `.gen` settings always take priority over `.claude` at the same level

### Migration Example

**Existing `.claude/settings.json`:**
```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "eslint --fix $FILE_PATH"
          }
        ]
      }
    ]
  }
}
```

**Add GenCode-specific hooks in `.gen/settings.json`:**
```json
{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo 'GenCode session complete'"
          }
        ]
      }
    ]
  }
}
```

**Result**: Both hooks are active! The Edit hook from `.claude` and the Stop hook from `.gen`.

### Prioritization

When both `.claude` and `.gen` define hooks for the same event:

```json
// .claude/settings.json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [{ "type": "command", "command": "echo 'Claude hook'" }]
      }
    ]
  }
}

// .gen/settings.json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [{ "type": "command", "command": "echo 'GenCode hook'" }]
      }
    ]
  }
}
```

**Result**: Both hooks execute (arrays are merged), but they run in order: `.claude` first, then `.gen`.

## Configuration Levels

Hooks can be configured at multiple levels (following GenCode's configuration hierarchy):

1. **User level**: `~/.gen/settings.json` or `~/.claude/settings.json`
2. **Project level**: `<project>/.gen/settings.json` or `<project>/.claude/settings.json`
3. **Local level**: `<project>/.gen/settings.local.json` or `<project>/.claude/settings.local.json`

Higher levels override lower levels, and hooks from all levels are merged.

## Security Considerations

- Hooks run with your user's permissions (no privilege escalation)
- Commands are executed via `bash -c` with proper escaping
- Environment variables are sanitized to prevent injection
- Sensitive directories are blocked (/etc, /var, /sys, /proc, /dev)
- Output is truncated at 30KB to prevent memory issues
- Timeouts prevent runaway processes

## Troubleshooting

### Hook Not Executing

1. **Check configuration syntax**: Ensure JSON is valid
2. **Check matcher pattern**: Verify it matches the tool name
3. **Check event name**: Must be one of the supported events
4. **Check command**: Test the command in terminal first

### Hook Blocking Tool

If a PreToolUse hook is blocking tools unintentionally:

1. Check the hook's exit code (must be 2 to block)
2. Set `"blocking": false` to make it non-blocking
3. Review the matcher pattern (might be too broad)

### Performance Issues

If hooks slow down the agent:

1. Reduce timeout values
2. Make hooks non-blocking (`"blocking": false`)
3. Optimize hook commands (avoid slow operations)
4. Use `matcher` to limit when hooks run

## Examples Repository

For more examples, see:
- [tests/config/hooks-config.test.ts](../tests/config/hooks-config.test.ts) - Unit tests with various scenarios
- [tests/integration/hooks-integration.test.ts](../tests/integration/hooks-integration.test.ts) - Integration tests with Agent
