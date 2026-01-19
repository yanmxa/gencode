# Custom Commands

Custom commands are markdown-based slash commands with YAML frontmatter that provide reusable prompt templates. They support variable expansion, file inclusion, and pre-authorization of tools.

## Overview

Custom commands allow you to:
- Create reusable prompt templates with variables
- Pre-authorize specific tools for a command
- Include file contents dynamically
- Override the default model
- Share commands across projects or with your team

## Compatibility

GenCode's custom commands system is **compatible with Claude Code and OpenCode** command formats. Commands can be placed in either GenCode or Claude Code directories, with GenCode commands taking precedence when duplicates exist.

## Command Locations

Commands are loaded from multiple directories with the following precedence (highest priority last):

1. `~/.claude/commands/` - User-level Claude Code commands
2. `~/.gen/commands/` - User-level GenCode commands (overrides ~/.claude)
3. `.claude/commands/` - Project-level Claude Code commands
4. `.gen/commands/` - Project-level GenCode commands (overrides .claude)

**Merge Behavior:** If the same command name exists in multiple locations, the GenCode version takes precedence over Claude Code, and project-level takes precedence over user-level.

## Command Format

Commands are markdown files (`.md`) with YAML frontmatter:

```markdown
---
description: Brief description of what this command does
argument-hint: <arg1> <arg2>
allowed-tools:
  - Read
  - Write
  - Bash(git *)
model: gpt-4o
---
Command template body with $ARGUMENTS, $1, $2, and @file inclusions.
```

### Frontmatter Fields

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Brief description shown in `/commands` listing |
| `argument-hint` | string | Hint for expected arguments (e.g., `<file> <message>`) |
| `allowed-tools` | string or array | Tools pre-authorized for this command |
| `model` | string | Override the default model for this command |

All fields are optional.

## Variable Expansion

### $ARGUMENTS
Replaced with the full argument string as provided by the user.

**Example:**
```markdown
Process these arguments: $ARGUMENTS
```

**Usage:** `/mycommand hello world`
**Expanded:** `Process these arguments: hello world`

### $1, $2, $3, ...
Replaced with positional arguments (respects quoted strings).

**Example:**
```markdown
File: $1
Message: $2
```

**Usage:** `/mycommand README.md "Fix typo"`
**Expanded:**
```
File: README.md
Message: Fix typo
```

### @file
Replaced with file contents (with security checks).

**Example:**
```markdown
Summarize this file:

@README.md
```

**Security:**
- Path traversal (`../`) is rejected
- Only files within the project root are allowed
- Missing files show error message in template

## Pre-Authorization

The `allowed-tools` field pre-authorizes tools for the command, bypassing permission prompts.

### Simple Tools
```yaml
allowed-tools:
  - Read
  - Write
  - Edit
```

### Pattern-Based (Bash)
```yaml
allowed-tools:
  - Bash(git *)
  - Bash(npm *)
```

**Patterns:**
- `git *` - Allows all git commands
- `npm install*` - Allows npm install commands
- `gh:*` - Allows gh commands with any arguments

## Examples

### Example 1: Git Status Command

**File:** `.gen/commands/git-status.md`

```markdown
---
description: Show git status and recent commits
allowed-tools:
  - Bash(git *)
---
Please run the following:

1. Show git status
2. Show the last 5 commits

Provide a brief summary of the repository state.
```

**Usage:** `/git-status`

### Example 2: Commit with Message

**File:** `.gen/commands/commit.md`

```markdown
---
description: Commit changes with a message
argument-hint: <message>
allowed-tools:
  - Bash(git *)
---
Please commit all changes with this message: $1

Make sure to:
1. Stage all modified files (not untracked)
2. Create a commit with the provided message
3. Show the commit summary
```

**Usage:** `/commit "Fix authentication bug"`

### Example 3: Analyze File

**File:** `.gen/commands/analyze.md`

```markdown
---
description: Analyze a source file
argument-hint: <filepath>
allowed-tools: [Read]
---
Please analyze the following file and provide:
1. Purpose and responsibilities
2. Key functions/classes
3. Potential improvements

File: $1

@$1
```

**Usage:** `/analyze src/agent/agent.ts`

### Example 4: Project Context

**File:** `.gen/commands/context.md`

```markdown
---
description: Load project context
allowed-tools: [Read]
---
Please read and summarize these key project files:

README:
@README.md

Package:
@package.json

Provide a brief overview of the project and its dependencies.
```

**Usage:** `/context`

### Example 5: Model Override

**File:** `.gen/commands/quick-fix.md`

```markdown
---
description: Quick fix with fast model
model: gemini-2.0-flash-exp
allowed-tools:
  - Read
  - Write
  - Edit
---
Please quickly fix this issue: $ARGUMENTS

Use the fastest approach possible.
```

**Usage:** `/quick-fix fix typo in header`

## Command Discovery

List all available custom commands:

```bash
/commands
# or
/cmd
```

This shows commands grouped by namespace (GenCode/Claude Code) with their descriptions and argument hints.

## Best Practices

1. **Keep Commands Focused:** Each command should do one thing well
2. **Use Descriptive Names:** Choose command names that clearly indicate their purpose
3. **Document Arguments:** Always provide `argument-hint` for commands that take arguments
4. **Minimal Pre-Auth:** Only pre-authorize tools that are essential for the command
5. **Test Locally First:** Create commands in `.gen/commands/` to test before moving to `~/.gen/commands/`
6. **Use File Includes Sparingly:** `@file` is powerful but can make templates hard to understand

## Security Considerations

### File Inclusion Security
- **Path Traversal:** Paths containing `../` are rejected
- **Scope:** Only files within the project root can be included
- **Error Handling:** Missing or inaccessible files show error messages in the template

### Pre-Authorization
- Pre-authorized tools bypass permission prompts
- Use patterns like `Bash(git *)` to limit scope
- Consider security implications before pre-authorizing `Bash` or `Write` tools

## Sharing Commands

### Team Commands (Project-Level)
Place in `.gen/commands/` or `.claude/commands/` and commit to version control:

```bash
mkdir -p .gen/commands
# Create command files
git add .gen/commands/
git commit -m "Add custom commands"
```

### Personal Commands (User-Level)
Place in `~/.gen/commands/` for all your projects:

```bash
mkdir -p ~/.gen/commands
# Create command files
```

## Troubleshooting

### Command Not Found
1. Check filename is `<command>.md` (e.g., `test.md` for `/test`)
2. Verify file is in a valid commands directory
3. Run `/commands` to see discovered commands
4. Check file permissions (must be readable)

### Variables Not Expanding
1. Ensure variable syntax is correct (`$ARGUMENTS`, `$1`, not `$arguments` or `$ARG1`)
2. Check YAML frontmatter is valid (use a YAML validator)
3. Quoted arguments are treated as single arguments

### File Inclusion Errors
1. Use project-relative paths (e.g., `@README.md`, `@src/index.ts`)
2. Avoid absolute paths or `../` traversal
3. Ensure file exists and is within project root

### Pre-Authorization Not Working
1. Check `allowed-tools` syntax (string or array)
2. Verify tool names match exactly (case-sensitive)
3. For patterns, use format: `Tool(pattern)` (e.g., `Bash(git *)`)

## Implementation Details

### Command Discovery
Commands are discovered at startup by scanning all command directories. The merge strategy ensures GenCode commands take precedence over Claude Code commands.

### Variable Expansion
Variables are expanded in this order:
1. `$ARGUMENTS` → full argument string
2. `$1`, `$2`, etc. → positional arguments (parsed with quote support)
3. `@file` → file contents (with security validation)

### Pre-Authorization
Pre-authorized tools are added to the permission manager as allowed prompts before the command executes. This bypasses the normal permission flow.

### Model Override
If a `model` field is specified, the model is changed for the duration of command execution. The previous model is not automatically restored.

## Related Documentation

- [Skills System](../docs/skills.md) - Domain expertise files
- [Permission System](../docs/permissions.md) - Permission rules and patterns
- [Hooks System](../docs/hooks.md) - Event-driven automation
