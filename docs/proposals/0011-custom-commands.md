# Proposal: Custom Commands (Slash Commands)

- **Proposal ID**: 0011
- **Author**: mycode team
- **Status**: Draft
- **Priority**: P2 (Enhanced Feature)
- **Created**: 2025-01-15
- **Updated**: 2025-01-17

## Summary

Implement a custom commands system that allows users to define reusable prompt templates as slash commands. Commands are stored as markdown files in `.mycode/commands/` and can accept arguments, making common workflows easily accessible.

## Motivation

Currently, users must re-type common prompts. This leads to:

1. **Repetition**: Same prompts typed repeatedly
2. **Inconsistency**: Variations in how tasks are requested
3. **No sharing**: Can't share workflows with team
4. **No organization**: Common tasks not documented
5. **Slow workflows**: Complex prompts take time to compose

Custom commands enable one-command access to common workflows.

## Claude Code Reference

Claude Code provides a powerful slash command system:

### Command Definition
```markdown
<!-- .claude/commands/fix-issue.md -->
---
description: Fix a GitHub issue
argument-hint: <issue-number>
allowed-tools: Bash(gh:*), Read, Edit, Write
---

# Fix GitHub Issue

Fetch issue $ARGUMENTS from GitHub and implement a fix:

1. Read the issue details using `gh issue view $ARGUMENTS`
2. Understand the problem and find relevant code
3. Implement the fix
4. Create a commit with descriptive message
5. Push the changes
```

### Usage
```
> /fix-issue 123

Agent: [Reads issue #123, implements fix, commits, pushes]
```

### Features
- Markdown files with YAML frontmatter
- `$ARGUMENTS` for user input
- `$1`, `$2` for positional arguments
- `@file` syntax for file inclusion
- `allowed-tools` for pre-authorized tools
- Project and user-level commands

### Command Locations
- `~/.mycode/commands/` - User commands (all projects)
- `.mycode/commands/` - Project commands

### Built-in Variables
- `$ARGUMENTS` - All arguments as string
- `$1`, `$2`, etc. - Positional arguments
- `@path/to/file` - Include file content

## Detailed Design

### API Design

```typescript
// src/commands/types.ts
interface CommandDefinition {
  name: string;              // Command name (from filename)
  description?: string;      // Short description
  argumentHint?: string;     // Usage hint for arguments
  allowedTools?: string[];   // Pre-authorized tools
  content: string;           // Prompt template
  filePath: string;          // Source file path
  scope: 'user' | 'project'; // Command scope
}

interface CommandInput {
  command: string;           // Command name
  arguments: string;         // Raw argument string
}

interface ParsedCommand {
  definition: CommandDefinition;
  expandedPrompt: string;    // Template with variables filled
  preAuthorizedTools: string[];
}
```

```typescript
// src/commands/command-manager.ts
class CommandManager {
  private commands: Map<string, CommandDefinition>;
  private userDir: string;
  private projectDir: string;

  constructor(projectDir: string);

  // Load commands from directories
  async loadCommands(): Promise<void>;

  // Get all available commands
  listCommands(): CommandDefinition[];

  // Check if command exists
  hasCommand(name: string): boolean;

  // Parse and expand a command
  parseCommand(input: CommandInput): ParsedCommand;

  // Get command by name
  getCommand(name: string): CommandDefinition | undefined;

  // Reload commands
  async refresh(): Promise<void>;
}
```

### Command Parsing

```typescript
// src/commands/parser.ts
interface CommandFrontmatter {
  description?: string;
  'argument-hint'?: string;
  'allowed-tools'?: string | string[];
}

function parseCommandFile(filePath: string, content: string): CommandDefinition {
  // Parse YAML frontmatter
  const { frontmatter, body } = parseFrontmatter(content);

  // Extract name from filename
  const name = path.basename(filePath, '.md');

  return {
    name,
    description: frontmatter.description,
    argumentHint: frontmatter['argument-hint'],
    allowedTools: normalizeAllowedTools(frontmatter['allowed-tools']),
    content: body,
    filePath,
    scope: filePath.includes('~/.mycode') ? 'user' : 'project'
  };
}

function expandTemplate(template: string, args: string): string {
  // Split arguments
  const argList = parseArguments(args);

  let result = template;

  // Replace $ARGUMENTS with full string
  result = result.replace(/\$ARGUMENTS/g, args);

  // Replace positional $1, $2, etc.
  for (let i = 0; i < argList.length; i++) {
    result = result.replace(new RegExp(`\\$${i + 1}`, 'g'), argList[i]);
  }

  // Handle @file includes
  result = expandFileIncludes(result);

  return result;
}
```

### Tool Pre-Authorization

```typescript
// Parse allowed-tools patterns
function normalizeAllowedTools(tools: string | string[] | undefined): string[] {
  if (!tools) return [];
  if (typeof tools === 'string') {
    return tools.split(',').map(t => t.trim());
  }
  return tools;
}

// Examples:
// "Bash(npm:*)" - Allow npm commands
// "Bash(gh:*)" - Allow GitHub CLI
// "Read, Write" - Allow Read and Write tools
// "Bash(mkdir:*), Bash(tee:*)" - Multiple Bash patterns
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/commands/types.ts` | Create | Command type definitions |
| `src/commands/command-manager.ts` | Create | Command loading and management |
| `src/commands/parser.ts` | Create | Markdown/frontmatter parsing |
| `src/commands/expander.ts` | Create | Template expansion |
| `src/commands/index.ts` | Create | Module exports |
| `src/cli/input-handler.ts` | Modify | Handle slash command input |
| `src/permissions/permission-manager.ts` | Modify | Pre-authorize tools |

## User Experience

### Creating a Command
Create `.mycode/commands/review-pr.md`:

```markdown
---
description: Review a GitHub pull request
argument-hint: <pr-number>
allowed-tools: Bash(gh:*), Read, Grep
---

# Review Pull Request

Review PR #$ARGUMENTS:

1. Fetch PR details: `gh pr view $ARGUMENTS`
2. Get the diff: `gh pr diff $ARGUMENTS`
3. Review code changes for:
   - Code quality issues
   - Potential bugs
   - Style violations
4. Provide a summary with actionable feedback
```

### Using Commands
```
> /review-pr 42

Agent: I'll review PR #42 for you.
[Fetches PR, analyzes diff, provides feedback]
```

### Listing Commands
```
> /help

Available Commands:
┌───────────────┬────────────────────────────────────────┐
│ Command       │ Description                            │
├───────────────┼────────────────────────────────────────┤
│ /fix-issue    │ Fix a GitHub issue                     │
│ /review-pr    │ Review a GitHub pull request           │
│ /create-test  │ Create unit tests for a file           │
│ /summarize    │ Summarize a file or directory          │
└───────────────┴────────────────────────────────────────┘

Project commands: .mycode/commands/
User commands: ~/.mycode/commands/
```

### Command with Multiple Arguments
```markdown
---
description: Create a component
argument-hint: <name> <type>
---

Create a $2 component named $1:
- File: src/components/$1.$2
- Include tests
- Follow project conventions
```

```
> /create-component Button tsx

Agent: Creating a tsx component named Button...
```

### File Inclusion
```markdown
---
description: Apply coding standards
---

Apply these coding standards to the codebase:

@.mycode/standards.md

Focus on the most critical violations first.
```

## Alternatives Considered

### Alternative 1: JSON Command Format
Define commands in JSON files.

**Pros**: Structured, easier to parse
**Cons**: Less readable, harder to write prompts
**Decision**: Rejected - Markdown is more natural

### Alternative 2: Inline Command Definition
Define commands within settings.json.

**Pros**: Single config file
**Cons**: Hard to manage many commands, no syntax highlighting
**Decision**: Rejected - Separate files are more manageable

### Alternative 3: JavaScript Commands
Write commands as JS/TS modules.

**Pros**: Full programmability
**Cons**: Security concerns, higher barrier to entry
**Decision**: Deferred - Start with templates, add later

## Security Considerations

1. **Tool Restrictions**: Pre-authorized tools respect permission system
2. **File Inclusion**: Only allow including files within project
3. **Argument Sanitization**: Sanitize arguments before expansion
4. **Path Traversal**: Prevent `@../../../etc/passwd`
5. **Execution Scope**: Commands run with normal permissions

```typescript
// Safe file inclusion
function expandFileIncludes(content: string, projectRoot: string): string {
  return content.replace(/@([^\s]+)/g, (match, filePath) => {
    const absolutePath = path.resolve(projectRoot, filePath);

    // Prevent path traversal
    if (!absolutePath.startsWith(projectRoot)) {
      return `[Error: Cannot include files outside project]`;
    }

    if (fs.existsSync(absolutePath)) {
      return fs.readFileSync(absolutePath, 'utf-8');
    }
    return `[File not found: ${filePath}]`;
  });
}
```

## Testing Strategy

1. **Unit Tests**:
   - Frontmatter parsing
   - Argument expansion
   - File inclusion

2. **Integration Tests**:
   - Command loading
   - Command execution
   - Pre-authorization

3. **Manual Testing**:
   - Various command formats
   - Argument edge cases
   - File inclusion

## Migration Path

1. **Phase 1**: Basic command loading and execution
2. **Phase 2**: Argument expansion ($ARGUMENTS, $1, $2)
3. **Phase 3**: File inclusion (@file)
4. **Phase 4**: Tool pre-authorization
5. **Phase 5**: /help command and discoverability

No breaking changes to existing functionality.

## References

- [Claude Code Commands Documentation](https://code.claude.com/docs/en/commands)
- [Claude Code Plugins Reference](https://code.claude.com/docs/en/plugins-reference)
- [YAML Frontmatter Specification](https://jekyllrb.com/docs/front-matter/)
