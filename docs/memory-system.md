# Memory System

GenCode implements a hierarchical memory system compatible with Claude Code's CLAUDE.md mechanism. Memory files provide persistent context that survives across sessions.

## Memory File Hierarchy

| Level | Primary (GenCode) | Fallback (Claude Code) |
|-------|-------------------|------------------------|
| User | `~/.gen/GEN.md` | `~/.claude/CLAUDE.md` |
| User Rules | `~/.gen/rules/*.md` | `~/.claude/rules/*.md` |
| Project | `./GEN.md` or `./.gen/GEN.md` | `./CLAUDE.md` or `./.claude/CLAUDE.md` |
| Project Rules | `./.gen/rules/*.md` | `./.claude/rules/*.md` |
| Local | `./.gen/GEN.local.md` | `./.claude/CLAUDE.local.md` |

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
│    Primary:  ~/.gen/GEN.md                                     │
│    Fallback: ~/.claude/CLAUDE.md                                     │
│    Rules:    ~/.gen/rules/*.md → ~/.claude/rules/*.md            │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 2. Project Level                                                     │
│    Primary:  ./GEN.md or ./.gen/GEN.md                       │
│    Fallback: ./CLAUDE.md or ./.claude/CLAUDE.md                      │
│    Rules:    ./.gen/rules/*.md → ./.claude/rules/*.md            │
│              (with paths: frontmatter for scoping)                   │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼
┌─────────────────────────────────────────────────────────────────────┐
│ 3. Local Level (gitignored)                                          │
│    Primary:  ./.gen/GEN.local.md                               │
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
| Primary filename | CLAUDE.md | GEN.md |
| Fallback support | No | Yes (→ CLAUDE.md) |
| User directory | ~/.claude/ | ~/.gen/ → ~/.claude/ |
| Project directory | .claude/ | .gen/ → .claude/ |
| Rules directory | .claude/rules/ | .gen/rules/ → .claude/rules/ |
| Path scoping | Yes (paths: frontmatter) | Yes (paths: frontmatter) |
| @import syntax | Yes | Yes (max 5 levels) |
| Local files | CLAUDE.local.md | GEN.local.md |
| Directory-specific | Yes (on-demand) | Not in v1 |

## Memory Merge Strategies

GenCode supports multiple strategies for loading memory files when both CLAUDE.md and GEN.md exist at the same level. This allows you to optimize context size and control which files are loaded.

### Available Strategies

| Strategy | Behavior | Use Case |
|----------|----------|----------|
| `fallback` (default) | Load GEN.md if exists, else CLAUDE.md | **Recommended**: Reduces context size while maintaining flexibility |
| `both` | Load both CLAUDE.md and GEN.md | Maximum context, useful when you need both file's content |
| `gencode-only` | Only load .gen/GEN.md files | Strict GenCode-only mode |
| `claude-only` | Only load .claude/CLAUDE.md files | Strict Claude Code compatibility mode |

### How Fallback Works

At each level (user, project, local), the system:
1. Checks for `.gen/GEN.md` (or `./GEN.md` at project root)
2. If found: Load GEN.md only, skip CLAUDE.md
3. If not found: Load `.claude/CLAUDE.md` (or `./CLAUDE.md`) as fallback

**Example:**
```
~/.claude/CLAUDE.md exists (global instructions)
~/.gen/GEN.md does NOT exist
./project1/.gen/GEN.md exists (project-specific)
./project2/ has no GEN.md

Loading for project1 (fallback mode):
- User level: ~/.claude/CLAUDE.md (no ~/.gen/GEN.md)
- Project level: ./project1/.gen/GEN.md (skip ./CLAUDE.md)
= Result: Global + Project configs, ~50% context reduction

Loading for project2 (fallback mode):
- User level: ~/.claude/CLAUDE.md (no ~/.gen/GEN.md)
- Project level: Nothing found
= Result: Only global config, maximum context reduction
```

### Configuring the Strategy

#### 1. Environment Variable (Highest Priority)

```bash
export GEN_MEMORY_STRATEGY=fallback  # or both, gencode-only, claude-only
gencode "help me with this code"
```

#### 2. Settings File (User or Project Level)

Add to `~/.gen/settings.json` or `./.gen/settings.json`:

```json
{
  "memoryMergeStrategy": "fallback"
}
```

#### 3. Default Behavior

If not specified, defaults to `fallback` mode.

### Verbose Mode

See which files were loaded and skipped:

```bash
# Set verbose in config
{
  "verbose": true,
  "memoryMergeStrategy": "fallback"
}

# Output shows:
[Memory] Strategy: fallback
[Memory] user: ~/.gen/GEN.md (0.1 KB)
[Memory] project: ./GEN.md (3.2 KB)
[Memory] Skipped: ~/.claude/CLAUDE.md
[Memory] Total: 3.3 KB (2 files loaded, 1 skipped)
```

### Context Size Comparison

Real-world example with both user and project memory:

| Strategy | Files Loaded | Total Size | Savings |
|----------|--------------|------------|---------|
| `both` (old default) | CLAUDE.md + GEN.md (both levels) | 8.9 KB | Baseline |
| `fallback` (new default) | GEN.md (user) + CLAUDE.md (project) | 3.4 KB | **61% reduction** |
| `gencode-only` | GEN.md only | 0.1 KB | **99% reduction** |
| `claude-only` | CLAUDE.md only | 8.7 KB | 2% reduction |

**Recommendation**: Use `fallback` mode (default) for best balance between context size and flexibility. Only use `both` if you specifically need content from both files at the same level.

## Commands

### /init

Analyzes your codebase and generates an GEN.md file:

```
> /init

Analyzing codebase...
Found 3 context file(s): package.json, README.md, tsconfig.json
Generating GEN.md...
```

The command:
1. Scans for package.json, README.md, Makefile, etc.
2. Builds a prompt asking the AI to generate project instructions
3. Creates or updates GEN.md via the Write tool

### /memory

Shows all loaded memory files:

```
> /memory

Loaded Memory Files:
  [1] ~/.gen/GEN.md (user, 1.2KB)
  [2] ./GEN.md (project, 856B)

Loaded Rules:
  [1] .gen/rules/api.md (project-rules, 234B)
```

### # Quick Add

Add notes to memory directly:

```
# Always use 2-space indentation
Added to project memory: ./GEN.md

## Prefer async/await over callbacks
Added to user memory: ~/.gen/GEN.md
```

- `# note` → adds to project memory
- `## note` → adds to user memory

## Rules Directory

Create path-scoped rules in `.gen/rules/`:

```
your-project/
├── .gen/
│   ├── GEN.md           # Main project instructions
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
# GEN.md

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

Contents of ~/.gen/GEN.md (user's private global instructions):
[user memory content]

Contents of ./GEN.md (project instructions):
[project memory content]

Rule from .gen/rules/api.md (applies to: src/api/**):
[rule content]
</claudeMd>
```

## Best Practices

1. **Keep memory concise** - Memory files are loaded every session
2. **Use rules for path-specific guidance** - Avoid cluttering main GEN.md
3. **Use local files for personal notes** - .gitignore automatically includes them
4. **Leverage fallback for Claude Code users** - Existing CLAUDE.md files work automatically
