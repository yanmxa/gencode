# Proposal: Memory System (AGENT.md) with /init Command

- **Proposal ID**: 0006
- **Author**: mycode team
- **Status**: Implemented
- **Created**: 2025-01-15
- **Updated**: 2025-01-16
- **Implemented**: 2025-01-16

## Summary

Implement a comprehensive memory system inspired by Claude Code, including:
1. **AGENT.md files** - Project-specific context that persists across sessions (with CLAUDE.md fallback for compatibility)
2. **Hierarchical memory loading** - User → Project → Local with rules directories
3. **/init command** - Automatic project analysis and AGENT.md generation
4. **# prefix** - Quick memory adds during sessions (`# note` for project, `## note` for user)
5. **/memory command** - View loaded memory files
6. **@import syntax** - Include other files into memory (max 5 levels)
7. **.gencode/rules/ directory** - Modular, path-scoped rules with frontmatter

## Motivation

Currently, mycode has no way to remember project-specific context. This leads to:

1. **Repetitive explanations**: Users re-explain project conventions every session
2. **Inconsistent behavior**: Agent doesn't follow project standards
3. **Poor onboarding**: New sessions start from scratch
4. **No learning**: Corrections aren't remembered
5. **No sharing**: Team conventions can't be documented for the agent

A memory system solves these by providing persistent, hierarchical project context.

## Claude Code Reference (Deep Research)

Based on comprehensive research of Claude Code's implementation:

### Memory File Hierarchy (Priority Order)

```
1. Enterprise Policy (highest priority)
   └── Organization-managed CLAUDE.md

2. User Memory
   └── ~/.claude/CLAUDE.md (personal preferences across all projects)

3. Project Memory
   └── ./CLAUDE.md or ./.claude/CLAUDE.md
   └── ./.claude/rules/*.md (path-scoped rules)
   └── ./.claude/CLAUDE.local.md (gitignored personal project notes)

4. Directory-Specific Memory (recursive up to root)
   └── ./src/CLAUDE.md
   └── ./src/components/CLAUDE.md
```

### /init Command

The /init command uses this exact prompt (extracted from Claude Code v2.1.8):

```
Please analyze this codebase and create a CLAUDE.md file, which will be given
to future instances of Claude Code to operate in this repository.

What to add:
1. Commands that will be commonly used, such as how to build, lint, and run tests.
   Include the necessary commands to develop in this codebase, such as how to run
   a single test.
2. High-level code architecture and structure so that future instances can be
   productive more quickly. Focus on the "big picture" architecture that requires
   reading multiple files to understand.

Usage notes:
- If there's already a CLAUDE.md, suggest improvements to it.
- When you make the initial CLAUDE.md, do not repeat yourself and do not include
  obvious instructions like "Provide helpful error messages to users", "Write
  unit tests for all new utilities", "Never include sensitive information (API
  keys, tokens) in code or commits".
- Avoid listing every component or file structure that can be easily discovered.
- Don't include generic development practices.
- If there are Cursor rules (in .cursor/rules/ or .cursorrules) or Copilot rules
  (in .github/copilot-instructions.md), make sure to include the important parts.
- If there is a README.md, make sure to include the important parts.
- Do not make up information such as "Common Development Tasks", "Tips for
  Development", "Support and Documentation" unless this is expressly included
  in other files that you read.
- Be sure to prefix the file with the following text:

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.
```

**Context Gathering**: Before running the prompt, Claude Code uses BatchTool with GlobTool to find:
- `package*.json`, `*.md`
- `.cursor/rules/**`, `.cursorrules/**`
- `.github/copilot-instructions.md`

### Memory Loading Mechanism

1. **Session-based loading**: All MYCODE.md files loaded at session start
2. **Appears as "claudeMd" context**: Injected into system prompt as labeled context
3. **Directory-aware**: Moving directories triggers loading of directory-specific rules
4. **Additive loading**: Context accumulates, doesn't replace
5. **@ imports resolved dynamically**: When reading files, not just at start

### .claude/rules/ Directory

```yaml
# .claude/rules/api-rules.md
---
paths:
  - "src/api/**/*.ts"
  - "src/routes/**/*.ts"
---

# API Development Rules

- Use async/await consistently
- Always validate request inputs with Zod
- Return consistent error response format
```

**Key behaviors**:
- All `.md` files automatically loaded as project memory
- Same priority as `.claude/CLAUDE.md`
- `paths` frontmatter scopes rules to specific files
- Only loads when working on matching files

### # Prefix Quick Adds

```
# Always use 2-space indentation for TypeScript
```

Instantly saved to appropriate MYCODE.md (user or project, with prompt to choose).

### /memory Command

Opens memory files in system editor for manual editing:
- Shows which memory files are currently loaded
- Allows selecting which file to edit

### @import Syntax

```markdown
# MYCODE.md

@docs/architecture.md
@docs/api-conventions.md

## Project Overview
...
```

**Behaviors**:
- Up to 5 levels of recursion
- Resolved dynamically when file is read
- Imported content injected via system reminders

## Detailed Design

### API Design

```typescript
// src/memory/types.ts
type MemoryLevel = 'enterprise' | 'user' | 'project' | 'local' | 'directory';

interface MemoryFile {
  path: string;
  content: string;
  level: MemoryLevel;
  loadedAt: Date;
  resolvedImports: string[];  // Paths of @imported files
}

interface MemoryRule {
  path: string;
  content: string;
  paths?: string[];  // Glob patterns for scoping
  isActive: boolean; // Whether current context matches paths
}

interface MemoryConfig {
  filename: string;           // Default: 'MYCODE.md'
  localFilename: string;      // Default: 'MYCODE.local.md'
  userPath: string;           // Default: '~/.mycode/MYCODE.md'
  rulesDir: string;           // Default: '.mycode/rules/'
  maxFileSize: number;        // Default: 100KB
  maxTotalSize: number;       // Default: 500KB
  maxImportDepth: number;     // Default: 5
}

interface LoadedMemory {
  files: MemoryFile[];
  rules: MemoryRule[];
  totalTokens: number;
  context: string;  // Combined context for system prompt
}
```

```typescript
// src/memory/memory-manager.ts
class MemoryManager {
  private config: MemoryConfig;
  private loadedMemory: LoadedMemory;
  private cwd: string;

  constructor(config?: Partial<MemoryConfig>);

  // Load all memory for current working directory
  async load(cwd: string): Promise<LoadedMemory>;

  // Get combined context string for system prompt
  getContext(): string;

  // Get context for specific file (includes path-scoped rules)
  getContextForFile(filePath: string): string;

  // Add content to memory via # prefix
  async quickAdd(content: string, level: 'user' | 'project'): Promise<void>;

  // Open memory file in editor
  async editMemory(level?: MemoryLevel): Promise<void>;

  // List loaded memory files
  getLoadedFiles(): MemoryFile[];

  // Resolve @imports in content
  private resolveImports(content: string, basePath: string, depth: number): string;

  // Parse frontmatter from rules files
  private parseRuleFrontmatter(content: string): { paths?: string[]; body: string };

  // Check if path matches glob patterns
  private matchesGlob(filePath: string, patterns: string[]): boolean;
}
```

### /init Command Implementation

```typescript
// src/commands/init-command.ts
interface InitCommandOptions {
  improve?: boolean;  // Improve existing MYCODE.md
  force?: boolean;    // Overwrite without confirmation
}

async function initCommand(context: CommandContext): Promise<void> {
  const { agent, ui, cwd } = context;

  ui.showMessage('Analyzing codebase...');

  // Step 1: Gather context files
  const contextFiles = await gatherContextFiles(cwd);

  // Step 2: Check for existing MYCODE.md
  const existingMycode = await findExistingMycode(cwd);

  // Step 3: Build the init prompt
  const prompt = buildInitPrompt(contextFiles, existingMycode);

  // Step 4: Run agent with the prompt
  ui.showMessage('Generating MYCODE.md...');
  const result = await agent.run(prompt);

  // Step 5: Write or confirm changes
  const mycodeContent = extractMycodeContent(result);
  await confirmAndWrite(cwd, mycodeContent, existingMycode, ui);
}

async function gatherContextFiles(cwd: string): Promise<Record<string, string>> {
  const patterns = [
    'package*.json',
    'README.md',
    'CONTRIBUTING.md',
    '.cursor/rules/**/*.md',
    '.cursorrules',
    '.github/copilot-instructions.md',
    'Cargo.toml',
    'go.mod',
    'pyproject.toml',
    'requirements.txt',
  ];

  const files: Record<string, string> = {};
  for (const pattern of patterns) {
    const matches = await glob(pattern, { cwd });
    for (const match of matches.slice(0, 5)) {  // Limit per pattern
      const content = await readFile(path.join(cwd, match), 'utf-8');
      if (content.length < 50000) {  // Size limit
        files[match] = content;
      }
    }
  }
  return files;
}

function buildInitPrompt(
  contextFiles: Record<string, string>,
  existingMycode?: string
): string {
  const contextParts = Object.entries(contextFiles)
    .map(([path, content]) => `### ${path}\n\`\`\`\n${content}\n\`\`\``)
    .join('\n\n');

  return `
Please analyze this codebase and create a MYCODE.md file, which will be given
to future instances of mycode to operate in this repository.

## Context Files

${contextParts}

${existingMycode ? `## Existing MYCODE.md\n\`\`\`\n${existingMycode}\n\`\`\`\n` : ''}

## Instructions

What to add:
1. Commands that will be commonly used, such as how to build, lint, and run tests.
   Include the necessary commands to develop in this codebase, such as how to run
   a single test.
2. High-level code architecture and structure so that future instances can be
   productive more quickly. Focus on the "big picture" architecture that requires
   reading multiple files to understand.

Usage notes:
- ${existingMycode ? 'Suggest improvements to the existing MYCODE.md.' : 'Create a new MYCODE.md.'}
- Do not repeat yourself and do not include obvious instructions like "Provide
  helpful error messages" or "Write tests for new code".
- Avoid listing every component or file structure that can be easily discovered.
- Don't include generic development practices.
- Do not make up information unless it's expressly included in the files above.
- Keep it concise - aim for ~20-30 lines.
- Prefix the file with:

# MYCODE.md

This file provides guidance to mycode when working with code in this repository.
`;
}
```

### Memory Loading Integration

```typescript
// src/agent/agent.ts (modified)
class Agent {
  private memoryManager: MemoryManager;

  async run(prompt: string): Promise<AgentEvent[]> {
    // Load memory at start of each run
    const memory = await this.memoryManager.load(this.cwd);

    // Build system prompt with memory context
    const systemPrompt = this.buildSystemPrompt(memory);

    // Continue with normal agent loop...
  }

  private buildSystemPrompt(memory: LoadedMemory): string {
    const basePrompt = this.getBaseSystemPrompt();

    if (memory.context) {
      return `${basePrompt}

<claudeMd>
${memory.context}
</claudeMd>

IMPORTANT: Follow the instructions above from MYCODE.md files. These are project-specific guidelines that take precedence over general behavior.
`;
    }

    return basePrompt;
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/memory/types.ts` | Create | Memory system types |
| `src/memory/memory-manager.ts` | Create | Core memory management |
| `src/memory/import-resolver.ts` | Create | @import syntax handling |
| `src/memory/rules-parser.ts` | Create | .mycode/rules/ parsing |
| `src/memory/index.ts` | Create | Module exports |
| `src/commands/init-command.ts` | Create | /init command |
| `src/commands/memory-command.ts` | Create | /memory command |
| `src/agent/agent.ts` | Modify | Integrate memory loading |
| `src/cli/input-handler.ts` | Modify | Handle # prefix |

## User Experience

### Running /init

```
$ mycode
> /init

Analyzing codebase...
  Reading package.json
  Reading README.md
  Reading tsconfig.json
  Found .cursorrules

Generating MYCODE.md...

┌─ Generated MYCODE.md ─────────────────────────┐
│                                               │
│ # MYCODE.md                                   │
│                                               │
│ This file provides guidance to mycode when    │
│ working with code in this repository.         │
│                                               │
│ ## Build Commands                             │
│ - `npm run build` - Compile TypeScript        │
│ - `npm test` - Run all tests                  │
│ - `npm test -- path/to/test` - Single test    │
│                                               │
│ ## Architecture                               │
│ - Provider abstraction in src/providers/      │
│ - Tool system in src/tools/                   │
│ - Agent loop in src/agent/                    │
│                                               │
└───────────────────────────────────────────────┘

Create MYCODE.md? [Yes] [Edit First] [Cancel]
```

### Quick Memory Add with #

```
> # Always run npm run lint before committing

✓ Added to project memory (MYCODE.md)
```

### /memory Command

```
> /memory

Loaded Memory Files:
  [1] ~/.mycode/MYCODE.md (user)
  [2] ./MYCODE.md (project)
  [3] ./.mycode/rules/api.md (rule, paths: src/api/**)

Select file to edit (1-3) or 'new' to create:
```

### Automatic Context Display

At session start:
```
$ mycode
Loading context from:
  ~/.mycode/MYCODE.md (user preferences)
  ./MYCODE.md (project)
  ./.mycode/rules/typescript.md (active)

mycode v0.2.0 | gpt-4o
>
```

## Alternatives Considered

### Alternative 1: Database Storage
Store memory in SQLite.

**Pros**: Efficient queries, structured data
**Cons**: Not version-controllable, harder to share
**Decision**: Rejected - File-based is git-friendly

### Alternative 2: Single Global File
Only support ~/.mycode/MYCODE.md.

**Pros**: Simple
**Cons**: No project-specific context
**Decision**: Rejected - Hierarchy is essential

### Alternative 3: JSON Configuration
Use JSON instead of Markdown.

**Pros**: Structured, parseable
**Cons**: Less readable, harder for humans to write
**Decision**: Rejected - Markdown is more natural

### Alternative 4: Skip /init
Let users create MYCODE.md manually.

**Pros**: Simpler
**Cons**: Poor onboarding, users don't know what to include
**Decision**: Rejected - /init provides great starting point

## Security Considerations

1. **Path Traversal**: Validate @import paths within project
2. **File Size Limits**: Prevent memory exhaustion
3. **Sensitive Data Warning**: Remind users not to include secrets
4. **Import Depth Limit**: Prevent infinite recursion
5. **Local Files**: MYCODE.local.md auto-added to .gitignore

## Testing Strategy

1. **Unit Tests**:
   - Memory loading hierarchy
   - @import resolution with depth limits
   - Path glob matching for rules
   - Frontmatter parsing

2. **Integration Tests**:
   - /init command end-to-end
   - Memory injection into agent
   - Directory-aware loading

3. **Manual Testing**:
   - Real projects with various structures
   - Cursor/Copilot rule migration
   - Team workflows

## Migration Path

1. **Phase 1**: Basic MYCODE.md loading (user + project)
2. **Phase 2**: /init command implementation
3. **Phase 3**: # prefix and /memory command
4. **Phase 4**: @import syntax
5. **Phase 5**: .mycode/rules/ with path scoping
6. **Phase 6**: Directory-aware dynamic loading

## References

- [Claude Code Memory Documentation](https://code.claude.com/docs/en/memory)
- [Piebald-AI/claude-code-system-prompts](https://github.com/Piebald-AI/claude-code-system-prompts) - Extracted prompts
- [How CLAUDE.md Works in Claude Code](https://gist.github.com/radleta/301666cfd7551b78d1378e18c82ba661)
- [Build your own /init command](https://kau.sh/blog/build-ai-init-command/)
- [Claude Code Best Practices: Memory Management](https://cuong.io/blog/2025/06/15-claude-code-best-practices-memory-management)
- [Modular Rules in Claude Code](https://claude-blog.setec.rs/blog/claude-code-rules-directory)
- [The Complete Guide to CLAUDE.md](https://www.builder.io/blog/claude-md-guide)
