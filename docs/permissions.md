# Permission System

GenCode includes a comprehensive permission system compatible with Claude Code's permission model. It provides fine-grained control over tool execution with pattern matching, prompt-based approvals, and persistent allowlists.

## Overview

The permission system controls which tool operations require user approval:

- **Auto-approved**: Read-only operations (Read, Glob, Grep, LSP)
- **Require confirmation**: Write operations (Write, Edit, Bash, WebFetch, WebSearch)
- **Denied**: Explicitly blocked operations

Users can configure additional rules via settings files to auto-approve or block specific operations.

## Permission Check Flow

### Claude Code Official Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                      Tool Execution Request                      │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
                    ┌────────────────────────┐
                    │   Check DENY Rules     │
                    │   (settings.deny[])    │
                    └────────────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │                         │
               Match Found              No Match
                    │                         │
                    ▼                         ▼
             ┌──────────┐        ┌────────────────────────┐
             │  DENIED  │        │   Check ALLOW Rules    │
             │  (block) │        │   (settings.allow[])   │
             └──────────┘        └────────────────────────┘
                                              │
                                 ┌────────────┴────────────┐
                                 │                         │
                            Match Found              No Match
                                 │                         │
                                 ▼                         ▼
                          ┌──────────┐        ┌────────────────────────┐
                          │ ALLOWED  │        │    Check ASK Rules     │
                          │  (auto)  │        │    (settings.ask[])    │
                          └──────────┘        └────────────────────────┘
                                                           │
                                              ┌────────────┴────────────┐
                                              │                         │
                                         Match Found              No Match
                                              │                         │
                                              ▼                         ▼
                                       ┌──────────┐           ┌──────────────┐
                                       │  PROMPT  │           │   Default    │
                                       │   User   │           │   Behavior   │
                                       └──────────┘           └──────────────┘
                                                                     │
                                                    ┌────────────────┴────────────────┐
                                                    │                                 │
                                             Read-only Tool                    Write Tool
                                                    │                                 │
                                                    ▼                                 ▼
                                             ┌──────────┐                      ┌──────────┐
                                             │ ALLOWED  │                      │  PROMPT  │
                                             │  (auto)  │                      │   User   │
                                             └──────────┘                      └──────────┘
```

**Key Points (Claude Code)**:
1. **deny** rules are checked first - they block regardless of other rules
2. **allow** rules are checked second - they auto-approve if matched
3. **ask** rules always prompt for confirmation
4. Default behavior depends on tool type (read-only vs write)

### GenCode Implementation Flow

GenCode follows the same 4-step flow as Claude Code:

```
┌─────────────────────────────────────────────────────────────────┐
│                      Tool Execution Request                      │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
                    ┌────────────────────────┐
                    │   1. Check DENY Rules  │
                    │   (settings.deny[])    │
                    └────────────────────────┘
                                 │
                    ┌────────────┴────────────┐
                    │                         │
               Match Found              No Match
                    │                         │
                    ▼                         ▼
             ┌──────────┐        ┌────────────────────────┐
             │  DENIED  │        │   2. Check ALLOW Rules │
             │  (block) │        │   (settings.allow[])   │
             └──────────┘        └────────────────────────┘
                                              │
                                 ┌────────────┴────────────┐
                                 │                         │
                            Match Found              No Match
                                 │                         │
                                 ▼                         ▼
                          ┌──────────┐        ┌────────────────────────┐
                          │ ALLOWED  │        │    3. Check ASK Rules  │
                          │  (auto)  │        │    (settings.ask[])    │
                          └──────────┘        └────────────────────────┘
                                                           │
                                              ┌────────────┴────────────┐
                                              │                         │
                                         Match Found              No Match
                                              │                         │
                                              ▼                         ▼
                                       ┌──────────┐           ┌──────────────┐
                                       │  PROMPT  │           │   4. Default │
                                       │   User   │           │   Behavior   │
                                       └──────────┘           └──────────────┘
                                                                     │
                                                    ┌────────────────┴────────────────┐
                                                    │                                 │
                                             Read-only Tool                    Write Tool
                                                    │                                 │
                                                    ▼                                 ▼
                                             ┌──────────┐                      ┌──────────┐
                                             │ ALLOWED  │                      │  PROMPT  │
                                             │  (auto)  │                      │   User   │
                                             └──────────┘                      └──────────┘
```

**GenCode Implementation Details**:

| Step | Check | Source | Action on Match |
|------|-------|--------|-----------------|
| 1 | DENY Rules | `settings.deny[]` | Block immediately |
| 2 | ALLOW Rules | `settings.allow[]` + defaults + Plan mode + session cache | Allow immediately |
| 3 | ASK Rules | `settings.ask[]` | Force prompt (always) |
| 4 | Default | Tool type | Read-only → auto, Write → prompt |

**Step 2 (ALLOW) includes**:
- `settings.allow[]` rules
- Prompt-based permissions (Plan mode `allowedPrompts`)
- Session approval cache

**Default Auto-Approved Tools (read-only)**:
- Read, Glob, Grep, LSP

## Quick Reference

| Command | Description |
|---------|-------------|
| `/permissions` | View current permission rules |
| `/permissions audit` | View recent permission decisions |
| `/permissions stats` | View permission statistics |

## Configuration Hierarchy

GenCode uses a multi-level configuration system (Claude Code compatible):

| Level | Path | Description |
|-------|------|-------------|
| User (global) | `~/.claude/settings.json` | User-wide settings |
| Project | `.claude/settings.json` | Project settings (tracked in git) |
| Project local | `.claude/settings.local.json` | Project settings (gitignored) |

**Fallback**: If `.claude/` doesn't exist, GenCode falls back to `.gencode/` directories.

**Loading order**: User → Project → Project local (later overrides earlier for scalar values, arrays are concatenated)

### Configuration Merging

- **Scalar values** (e.g., `model`): Later values override earlier ones
- **Array values** (e.g., `permissions.allow`): Arrays are concatenated

```json
// ~/.claude/settings.json (user)
{ "permissions": { "allow": ["Bash(git:*)"] } }

// .claude/settings.local.json (project local)
{ "permissions": { "allow": ["WebSearch"] } }

// Result: allow = ["Bash(git:*)", "WebSearch"]
```

## Permission Rules

### Rule Format

Rules are configured in settings files using Claude Code format:

```json
{
  "permissions": {
    "allow": ["Bash(git add:*)", "WebSearch"],
    "ask": ["Bash(npm run:*)"],
    "deny": ["Bash(rm -rf:*)"]
  }
}
```

| Array | Mode | Description |
|-------|------|-------------|
| `allow` | auto | Auto-approve matching operations |
| `ask` | confirm | Always require confirmation |
| `deny` | deny | Block matching operations |

### Pattern Syntax

- `Tool` - Match all operations for a tool
- `Tool(pattern)` - Match operations where input matches pattern
- `*` - Wildcard matching any characters
- `:` - Treated as whitespace separator

### Examples

| Pattern | Matches |
|---------|---------|
| `Bash(git:*)` | Any git command |
| `Bash(npm install:*)` | npm install commands |
| `Bash(npm run test:*)` | npm test commands |
| `WebSearch` | All web searches |
| `WebFetch(domain:github.com)` | Fetches from github.com |

## Approval Options

When a tool requires confirmation, you can choose:

1. **Yes** - Allow this specific operation once
2. **Yes, and don't ask again** - Add a persistent rule to project settings
3. **No** - Block this operation

When you select "don't ask again", the rule is saved to `.claude/settings.local.json` (or `.gencode/settings.local.json` if using GenCode directories).

## Prompt-Based Permissions

Semantic prompts allow natural language permission grants (used by plan mode):

```typescript
// From plan mode (ExitPlanMode style)
agent.addAllowedPrompts([
  { tool: "Bash", prompt: "run tests" },
  { tool: "Bash", prompt: "install dependencies" },
  { tool: "Bash", prompt: "build the project" }
]);
```

### Built-in Prompts

The system recognizes these semantic prompts:

| Prompt | Matches |
|--------|---------|
| "run tests" | npm test, pytest, go test, jest, vitest |
| "install dependencies" | npm install, pip install, cargo build |
| "build the project" | npm run build, make, cargo build |
| "git operations" | Any git command |
| "lint code" | eslint, pylint, cargo clippy |
| "format code" | prettier, black, gofmt |
| "start dev server" | npm run dev, go run, cargo run |

## Permission Scopes

### Session Scope
- In-memory only
- Cleared when session ends
- Created by selecting "Allow once"

### Project Scope
- Stored in `.claude/settings.local.json`
- Applies to current project
- Created by selecting "Don't ask again"

### Global Scope
- Stored in `~/.claude/settings.json`
- Applies to all projects
- Configured manually in settings file

## Audit Logging

Permission decisions are logged for transparency:

```
/permissions audit

Recent Permission Decisions:
Time      Decision   Tool        Input
──────────────────────────────────────────────────────
10:42     allowed    Bash        git status
10:41     allowed    Read        src/index.ts
10:40     confirmed  Write       src/new.ts
10:38     denied     Bash        rm -rf /tmp/test
```

## Security Best Practices

1. **Start restrictive**: Default is to require confirmation for write operations
2. **Narrow patterns**: Use `git add:*` instead of `git:*`
3. **Review audit logs**: Check `/permissions audit` regularly
4. **Use project scope**: Limit auto-approvals to specific projects
5. **Avoid global allows for dangerous commands**: Configure per-project instead

## Default Behavior

| Tool | Default Mode | Description |
|------|--------------|-------------|
| Read | auto | File reading |
| Glob | auto | Pattern matching |
| Grep | auto | Content search |
| LSP | auto | Language server |
| Write | confirm | File creation |
| Edit | confirm | File modification |
| Bash | confirm | Shell execution |
| WebFetch | confirm | HTTP requests |
| WebSearch | confirm | Web search |

## API Reference

### PermissionManager

```typescript
import { PermissionManager } from 'gencode';

const manager = new PermissionManager({
  projectPath: '/path/to/project',
  enableAudit: true
});

// Initialize with settings
await manager.initialize({
  allow: ['Bash(git:*)'],
  ask: ['Bash(npm run:*)'],
  deny: ['Bash(rm -rf:*)']
});

// Check permission
const decision = await manager.checkPermission({
  tool: 'Bash',
  input: { command: 'git status' }
});

// Add prompt-based permission
manager.addAllowedPrompts([
  { tool: 'Bash', prompt: 'run tests' }
]);

// Set callback for saving rules to settings
manager.setSaveRuleCallback(async (tool, pattern) => {
  // Save to settings.local.json
});
```

### PromptMatcher

```typescript
import { PromptMatcher, parsePatternString } from 'gencode';

const matcher = new PromptMatcher();

// Check if input matches prompt
const matches = matcher.matches('run tests', { command: 'npm test' });
// true

// Parse pattern string
const parsed = parsePatternString('Bash(git add:*)');
// { tool: 'Bash', pattern: 'git add:*' }
```
