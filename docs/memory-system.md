# Memory System

GenCode implements a hierarchical memory system compatible with Claude Code's CLAUDE.md mechanism. Memory files provide persistent context that survives across sessions.

## Memory File Hierarchy

| Level | Primary (GenCode) | Fallback (Claude Code) |
|-------|-------------------|------------------------|
| User | `~/.gencode/AGENT.md` | `~/.claude/CLAUDE.md` |
| User Rules | `~/.gencode/rules/*.md` | `~/.claude/rules/*.md` |
| Project | `./AGENT.md` or `./.gencode/AGENT.md` | `./CLAUDE.md` or `./.claude/CLAUDE.md` |
| Project Rules | `./.gencode/rules/*.md` | `./.claude/rules/*.md` |
| Local | `./.gencode/AGENT.local.md` | `./.claude/CLAUDE.local.md` |

**Loading Logic**: Primary path is checked first; if not found, fallback path is used.

## Memory Loading Flow

### Claude Code Memory Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Claude Code Memory Loading                        │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 1. User Level: ~/.claude/CLAUDE.md                                   │
│    └── ~/.claude/rules/*.md (User rules, lower priority)            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 2. Project Level: ./CLAUDE.md or ./.claude/CLAUDE.md                │
│    └── ./.claude/rules/*.md (Project rules, with paths: scoping)    │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 3. Local Level: ./.claude/CLAUDE.local.md (gitignored)              │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 4. Directory-specific: src/CLAUDE.md (on-demand when accessing src) │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Inject into <claudeMd> tag                        │
└─────────────────────────────────────────────────────────────────────┘
```

### GenCode Memory Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                    GenCode Memory Loading                            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 1. User Level                                                        │
│    Primary:  ~/.gencode/AGENT.md                                     │
│    Fallback: ~/.claude/CLAUDE.md                                     │
│    Rules:    ~/.gencode/rules/*.md → ~/.claude/rules/*.md            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 2. Project Level                                                     │
│    Primary:  ./AGENT.md or ./.gencode/AGENT.md                       │
│    Fallback: ./CLAUDE.md or ./.claude/CLAUDE.md                      │
│    Rules:    ./.gencode/rules/*.md → ./.claude/rules/*.md            │
│              (with paths: frontmatter for scoping)                   │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 3. Local Level (gitignored)                                          │
│    Primary:  ./.gencode/AGENT.local.md                               │
│    Fallback: ./.claude/CLAUDE.local.md                               │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 4. Resolve @imports (max 5 levels, circular detection)               │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 5. Activate path-scoped rules based on current file                  │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Inject into <claudeMd> tag                        │
└─────────────────────────────────────────────────────────────────────┘
```

## Comparison Table

| Feature | Claude Code | GenCode |
|---------|-------------|---------|
| Primary filename | CLAUDE.md | AGENT.md |
| Fallback support | No | Yes (→ CLAUDE.md) |
| User directory | ~/.claude/ | ~/.gencode/ → ~/.claude/ |
| Project directory | .claude/ | .gencode/ → .claude/ |
| Rules directory | .claude/rules/ | .gencode/rules/ → .claude/rules/ |
| Path scoping | Yes (paths: frontmatter) | Yes (paths: frontmatter) |
| @import syntax | Yes | Yes (max 5 levels) |
| Local files | CLAUDE.local.md | AGENT.local.md |
| Directory-specific | Yes (on-demand) | Not in v1 |

## Commands

### /init

Analyzes your codebase and generates an AGENT.md file:

```
> /init

Analyzing codebase...
Found 3 context file(s): package.json, README.md, tsconfig.json
Generating AGENT.md...
```

The command:
1. Scans for package.json, README.md, Makefile, etc.
2. Builds a prompt asking the AI to generate project instructions
3. Creates or updates AGENT.md via the Write tool

### /memory

Shows all loaded memory files:

```
> /memory

Loaded Memory Files:
  [1] ~/.gencode/AGENT.md (user, 1.2KB)
  [2] ./AGENT.md (project, 856B)

Loaded Rules:
  [1] .gencode/rules/api.md (project-rules, 234B)
```

### # Quick Add

Add notes to memory directly:

```
# Always use 2-space indentation
Added to project memory: ./AGENT.md

## Prefer async/await over callbacks
Added to user memory: ~/.gencode/AGENT.md
```

- `# note` → adds to project memory
- `## note` → adds to user memory

## Rules Directory

Create path-scoped rules in `.gencode/rules/`:

```
your-project/
├── .gencode/
│   ├── AGENT.md           # Main project instructions
│   └── rules/
│       ├── code-style.md  # Always loaded
│       ├── testing.md     # Always loaded
│       └── api.md         # Only when working with API files
```

### Path-Scoped Rules (Frontmatter)

```yaml
---
paths:
  - "src/api/**/*.ts"
  - "src/routes/**/*.ts"
---

# API Development Rules

- Use async/await consistently
- Always validate request inputs with Zod
```

Rules with `paths:` frontmatter only load when working with matching files.

## @import Syntax

Include other files in your memory files:

```markdown
# AGENT.md

@./docs/architecture.md
@./docs/conventions.md

## Project Overview
...
```

Features:
- Max 5 levels of nesting
- Circular import detection
- Path traversal protection

## System Prompt Integration

Memory content is injected into the system prompt:

```xml
<claudeMd>
Codebase and user instructions are shown below...

Contents of ~/.gencode/AGENT.md (user's private global instructions):
[user memory content]

Contents of ./AGENT.md (project instructions):
[project memory content]

Rule from .gencode/rules/api.md (applies to: src/api/**):
[rule content]
</claudeMd>
```

## Best Practices

1. **Keep memory concise** - Memory files are loaded every session
2. **Use rules for path-specific guidance** - Avoid cluttering main AGENT.md
3. **Use local files for personal notes** - .gitignore automatically includes them
4. **Leverage fallback for Claude Code users** - Existing CLAUDE.md files work automatically
