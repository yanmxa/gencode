# Proposal: LS Tool

- **Proposal ID**: 0015
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a dedicated LS (List) tool for directory listing that provides structured output with file metadata, sorting, filtering, and tree visualization. This replaces ad-hoc Bash `ls` commands with a purpose-built tool optimized for agent consumption.

## Motivation

Currently, mycode uses Bash with `ls` for directory listing. This causes issues:

1. **Parsing complexity**: Agent must parse varied `ls` output formats
2. **Platform differences**: `ls` flags differ between macOS, Linux, BSD
3. **No structured data**: Text output requires manual extraction
4. **Limited features**: Basic `ls` lacks tree view, size summaries
5. **Security concerns**: Bash command injection risks

A dedicated LS tool provides safe, structured, consistent directory listing.

## Claude Code Reference

Claude Code recommends using the Read tool for directory operations rather than Bash:

### From Claude Code Guidelines
```
- This tool can only read files, not directories.
  To read a directory, use an ls command via the Bash tool.
```

### Observed Behavior
Claude Code agents frequently use:
```bash
ls -la /path/to/dir
ls -1 src/
find . -type f -name "*.ts" | head -20
```

### Desired Improvement
A native LS tool would provide:
- Consistent JSON/structured output
- Cross-platform behavior
- Integrated filtering
- No command injection risk

## Detailed Design

### API Design

```typescript
// src/tools/ls/types.ts
interface LSInput {
  path?: string;           // Directory path (default: cwd)
  all?: boolean;           // Include hidden files (default: false)
  long?: boolean;          // Include metadata (default: false)
  recursive?: boolean;     // List recursively (default: false)
  depth?: number;          // Max recursion depth (default: 3)
  pattern?: string;        // Filter by glob pattern
  sort?: 'name' | 'size' | 'modified' | 'type';  // Sort order
  reverse?: boolean;       // Reverse sort order
  directories_only?: boolean;  // Only show directories
  files_only?: boolean;    // Only show files
  limit?: number;          // Max entries to return
}

interface FileEntry {
  name: string;
  path: string;            // Relative to input path
  type: 'file' | 'directory' | 'symlink' | 'other';
  size?: number;           // In bytes (when long=true)
  modified?: string;       // ISO timestamp (when long=true)
  permissions?: string;    // Unix permissions (when long=true)
  target?: string;         // Symlink target (when applicable)
}

interface LSOutput {
  success: boolean;
  path: string;            // Absolute path listed
  entries: FileEntry[];
  summary?: {
    total_files: number;
    total_directories: number;
    total_size: number;
  };
  truncated?: boolean;     // True if limit was applied
  error?: string;
}
```

```typescript
// src/tools/ls/ls-tool.ts
const lsTool: Tool<LSInput> = {
  name: 'LS',
  description: `List directory contents with structured output.

Parameters:
- path: Directory to list (default: current working directory)
- all: Include hidden files starting with . (default: false)
- long: Include file metadata (size, modified, permissions)
- recursive: List subdirectories recursively
- depth: Maximum recursion depth (default: 3)
- pattern: Glob pattern to filter results (e.g., "*.ts")
- sort: Sort by name, size, modified, or type
- directories_only: Only show directories
- files_only: Only show files
- limit: Maximum number of entries to return

Returns structured list of files and directories with optional metadata.
Use this instead of Bash ls commands for reliable, structured output.
`,
  parameters: z.object({
    path: z.string().optional(),
    all: z.boolean().optional().default(false),
    long: z.boolean().optional().default(false),
    recursive: z.boolean().optional().default(false),
    depth: z.number().int().positive().optional().default(3),
    pattern: z.string().optional(),
    sort: z.enum(['name', 'size', 'modified', 'type']).optional().default('name'),
    reverse: z.boolean().optional().default(false),
    directories_only: z.boolean().optional().default(false),
    files_only: z.boolean().optional().default(false),
    limit: z.number().int().positive().optional()
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **Path Resolution**: Resolve relative to cwd, validate existence
2. **Directory Reading**: Use fs.readdir with file types
3. **Filtering**: Apply pattern matching and type filters
4. **Metadata Collection**: Gather stats when long=true
5. **Sorting**: Apply requested sort order
6. **Recursion**: Process subdirectories with depth limit

```typescript
// Core implementation
async function listDirectory(
  input: LSInput,
  context: ToolContext
): Promise<LSOutput> {
  const targetPath = path.resolve(context.cwd, input.path || '.');

  // Validate path exists and is directory
  const stats = await fs.stat(targetPath);
  if (!stats.isDirectory()) {
    return { success: false, path: targetPath, entries: [], error: 'Not a directory' };
  }

  // Read directory entries
  let entries = await readDirectoryEntries(targetPath, {
    recursive: input.recursive,
    depth: input.depth,
    includeHidden: input.all
  });

  // Apply filters
  if (input.pattern) {
    entries = entries.filter(e => minimatch(e.name, input.pattern!));
  }
  if (input.directories_only) {
    entries = entries.filter(e => e.type === 'directory');
  }
  if (input.files_only) {
    entries = entries.filter(e => e.type === 'file');
  }

  // Gather metadata if requested
  if (input.long) {
    entries = await Promise.all(entries.map(async e => ({
      ...e,
      ...await getFileMetadata(path.join(targetPath, e.path))
    })));
  }

  // Sort entries
  entries = sortEntries(entries, input.sort, input.reverse);

  // Apply limit
  let truncated = false;
  if (input.limit && entries.length > input.limit) {
    entries = entries.slice(0, input.limit);
    truncated = true;
  }

  // Calculate summary
  const summary = input.long ? calculateSummary(entries) : undefined;

  return {
    success: true,
    path: targetPath,
    entries,
    summary,
    truncated
  };
}

async function readDirectoryEntries(
  dir: string,
  options: { recursive: boolean; depth: number; includeHidden: boolean },
  currentDepth = 0,
  basePath = ''
): Promise<FileEntry[]> {
  const dirents = await fs.readdir(dir, { withFileTypes: true });
  const entries: FileEntry[] = [];

  for (const dirent of dirents) {
    // Skip hidden unless requested
    if (!options.includeHidden && dirent.name.startsWith('.')) continue;

    const relativePath = path.join(basePath, dirent.name);
    const entry: FileEntry = {
      name: dirent.name,
      path: relativePath,
      type: getEntryType(dirent)
    };

    entries.push(entry);

    // Recurse into directories
    if (options.recursive && dirent.isDirectory() && currentDepth < options.depth) {
      const subEntries = await readDirectoryEntries(
        path.join(dir, dirent.name),
        options,
        currentDepth + 1,
        relativePath
      );
      entries.push(...subEntries);
    }
  }

  return entries;
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/ls/types.ts` | Create | Type definitions |
| `src/tools/ls/ls-tool.ts` | Create | Tool implementation |
| `src/tools/ls/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register LS tool |

## User Experience

### Basic Directory Listing
```
User: What files are in the src directory?

Agent: [LS: path="src"]

Contents of src/:
├── index.ts
├── cli/
├── agent/
├── providers/
├── session/
└── tools/

6 items (1 file, 5 directories)
```

### Detailed Listing with Metadata
```
Agent: [LS: path="src/tools", long=true]

┌─ src/tools/ ──────────────────────────────────────┐
│ Name            Type   Size     Modified         │
├───────────────────────────────────────────────────┤
│ index.ts        file   1.2 KB   2024-01-15 10:30 │
│ types.ts        file   2.8 KB   2024-01-14 15:45 │
│ registry.ts     file   4.1 KB   2024-01-15 09:20 │
│ builtin/        dir    -        2024-01-15 11:00 │
└───────────────────────────────────────────────────┘

Summary: 3 files, 1 directory, 8.1 KB total
```

### Recursive with Pattern Filter
```
Agent: [LS: path="src", recursive=true, pattern="*.ts", limit=10]

TypeScript files in src/:
  src/index.ts
  src/cli/index.ts
  src/cli/ui.ts
  src/agent/agent.ts
  src/agent/types.ts
  src/providers/index.ts
  src/providers/types.ts
  src/providers/openai.ts
  src/providers/anthropic.ts
  src/providers/gemini.ts

Showing 10 of 24 files (truncated)
```

### Tree View for Recursive Listing
```
Agent: [LS: path="src", recursive=true, depth=2]

src/
├── index.ts
├── cli/
│   ├── index.ts
│   ├── ui.ts
│   └── session-picker.ts
├── agent/
│   ├── agent.ts
│   ├── types.ts
│   └── index.ts
├── providers/
│   ├── index.ts
│   ├── types.ts
│   ├── openai.ts
│   ├── anthropic.ts
│   └── gemini.ts
└── tools/
    ├── index.ts
    ├── types.ts
    └── builtin/
```

## Alternatives Considered

### Alternative 1: Extend Read Tool
Add directory listing to Read tool.

**Pros**: Fewer tools to manage
**Cons**: Overloads Read semantics, complex parameters
**Decision**: Rejected - Separate concern

### Alternative 2: Use Bash with Parsing
Parse Bash ls output programmatically.

**Pros**: No new tool needed
**Cons**: Platform differences, parsing fragility
**Decision**: Rejected - Not reliable cross-platform

### Alternative 3: Return JSON Only
No formatted tree output, just JSON.

**Pros**: Simpler implementation
**Cons**: Harder for agent to present to user
**Decision**: Rejected - Tree view is valuable UX

## Security Considerations

1. **Path Traversal**: Prevent `../` escaping workspace
2. **Symlink Loops**: Detect and prevent infinite recursion
3. **Large Directories**: Enforce entry limits
4. **Permission Errors**: Handle gracefully
5. **No Command Injection**: Pure filesystem operations

```typescript
const LIMITS = {
  maxEntries: 1000,
  maxDepth: 10,
  maxSymlinkDepth: 5
};

function validatePath(target: string, cwd: string): boolean {
  const resolved = path.resolve(cwd, target);
  return resolved.startsWith(cwd) || !target.includes('..');
}
```

## Testing Strategy

1. **Unit Tests**:
   - Basic listing
   - Hidden file filtering
   - Pattern matching
   - Sorting options
   - Depth limiting

2. **Integration Tests**:
   - Large directories
   - Symlinks
   - Permission denied scenarios
   - Cross-platform paths

3. **Manual Testing**:
   - Various project structures
   - Edge cases (empty dirs, special characters)

## Migration Path

1. **Phase 1**: Basic listing with filtering
2. **Phase 2**: Long format with metadata
3. **Phase 3**: Recursive listing
4. **Phase 4**: Tree visualization
5. **Phase 5**: Performance optimization for large dirs

The Bash tool remains available for special cases.

## References

- [Node.js fs.readdir](https://nodejs.org/api/fs.html#fsreaddirpath-options-callback)
- [Minimatch - Glob Matching](https://github.com/isaacs/minimatch)
- [eza - Modern ls replacement](https://github.com/eza-community/eza)
