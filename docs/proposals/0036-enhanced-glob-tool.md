# Proposal: Enhanced Glob Tool

- **Proposal ID**: 0036
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Enhance the Glob tool with advanced pattern matching, file metadata, sorting options, and result formatting for more powerful file discovery.

## Motivation

The current Glob tool is basic:

1. **Pattern only**: No metadata in results
2. **No sorting**: Results in arbitrary order
3. **Limited patterns**: Basic glob only
4. **No exclusions**: Can't ignore patterns
5. **Flat output**: Just file paths

Enhanced glob enables sophisticated file discovery.

## Claude Code Reference

Claude Code's Glob tool provides fast pattern matching:

### From Tool Description
```
- Fast file pattern matching for any codebase size
- Supports glob patterns like "**/*.js" or "src/**/*.ts"
- Returns matching file paths sorted by modification time
```

### Key Features
- Sort by modification time
- Size-based filtering
- Exclusion patterns

## Detailed Design

### API Design

```typescript
// src/tools/glob/types.ts
interface GlobInput {
  pattern: string;
  path?: string;              // Base directory
  ignore?: string[];          // Exclusion patterns
  type?: 'file' | 'directory' | 'all';
  sort?: 'name' | 'modified' | 'size' | 'type';
  reverse?: boolean;
  limit?: number;
  includeMetadata?: boolean;  // Include file stats
  dot?: boolean;              // Include dotfiles
}

interface GlobMatch {
  path: string;               // Relative path
  absolutePath: string;
  type: 'file' | 'directory' | 'symlink';
  // Metadata (when includeMetadata=true)
  size?: number;
  modified?: string;
  created?: string;
  permissions?: string;
}

interface GlobOutput {
  success: boolean;
  matches: GlobMatch[];
  pattern: string;
  total: number;
  truncated: boolean;
  error?: string;
}
```

### Enhanced Glob Implementation

```typescript
// src/tools/glob/glob-tool.ts
const globTool: Tool<GlobInput> = {
  name: 'Glob',
  description: `Fast file pattern matching.

Parameters:
- pattern: Glob pattern (e.g., "**/*.ts", "src/**/*.{js,jsx}")
- path: Base directory (default: cwd)
- ignore: Patterns to exclude (e.g., ["node_modules/**"])
- type: Match files, directories, or all
- sort: Sort by name, modified, size, or type (default: modified)
- reverse: Reverse sort order
- limit: Maximum results
- includeMetadata: Include file stats (size, modified, etc.)
- dot: Include dotfiles (default: false)

Returns matching file paths sorted by modification time.
`,
  parameters: z.object({
    pattern: z.string(),
    path: z.string().optional(),
    ignore: z.array(z.string()).optional(),
    type: z.enum(['file', 'directory', 'all']).optional(),
    sort: z.enum(['name', 'modified', 'size', 'type']).optional(),
    reverse: z.boolean().optional(),
    limit: z.number().int().positive().optional(),
    includeMetadata: z.boolean().optional(),
    dot: z.boolean().optional()
  }),
  execute: async (input, context) => {
    const basePath = input.path
      ? path.resolve(context.cwd, input.path)
      : context.cwd;

    const options: GlobOptions = {
      cwd: basePath,
      ignore: input.ignore || ['**/node_modules/**', '**/.git/**'],
      dot: input.dot || false,
      onlyFiles: input.type === 'file',
      onlyDirectories: input.type === 'directory'
    };

    try {
      const paths = await glob(input.pattern, options);

      // Build matches with optional metadata
      let matches: GlobMatch[] = await Promise.all(
        paths.map(async (p) => {
          const absolutePath = path.join(basePath, p);
          const match: GlobMatch = {
            path: p,
            absolutePath,
            type: 'file'
          };

          if (input.includeMetadata) {
            const stats = await fs.stat(absolutePath);
            match.size = stats.size;
            match.modified = stats.mtime.toISOString();
            match.created = stats.birthtime.toISOString();
            match.type = stats.isDirectory() ? 'directory'
              : stats.isSymbolicLink() ? 'symlink' : 'file';
          }

          return match;
        })
      );

      // Sort
      matches = sortMatches(matches, input.sort || 'modified', input.reverse);

      // Limit
      const truncated = input.limit ? matches.length > input.limit : false;
      if (input.limit) {
        matches = matches.slice(0, input.limit);
      }

      return {
        success: true,
        matches,
        pattern: input.pattern,
        total: paths.length,
        truncated
      };
    } catch (error) {
      return {
        success: false,
        matches: [],
        pattern: input.pattern,
        total: 0,
        truncated: false,
        error: error instanceof Error ? error.message : 'Unknown error'
      };
    }
  }
};

function sortMatches(
  matches: GlobMatch[],
  sortBy: string,
  reverse?: boolean
): GlobMatch[] {
  const sorted = [...matches];

  sorted.sort((a, b) => {
    switch (sortBy) {
      case 'name':
        return a.path.localeCompare(b.path);
      case 'modified':
        return (b.modified || '').localeCompare(a.modified || '');
      case 'size':
        return (b.size || 0) - (a.size || 0);
      case 'type':
        return a.type.localeCompare(b.type);
      default:
        return 0;
    }
  });

  return reverse ? sorted.reverse() : sorted;
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/glob/types.ts` | Create | Type definitions |
| `src/tools/glob/glob-tool.ts` | Modify | Enhanced implementation |
| `src/tools/glob/utils.ts` | Create | Sorting utilities |

## User Experience

### Basic Pattern
```
Agent: [Glob: pattern="src/**/*.ts"]

Found 24 TypeScript files:
  src/index.ts
  src/cli/index.ts
  src/agent/agent.ts
  ...
```

### With Metadata
```
Agent: [Glob: pattern="*.md", includeMetadata=true, sort="modified"]

┌─ Markdown Files ──────────────────────────────────┐
│ File                Size     Modified            │
├───────────────────────────────────────────────────┤
│ README.md           4.2 KB   2025-01-15 10:30   │
│ CHANGELOG.md        12.8 KB  2025-01-14 16:45   │
│ CONTRIBUTING.md     2.1 KB   2025-01-10 09:15   │
└───────────────────────────────────────────────────┘
```

### With Exclusions
```
Agent: [Glob: pattern="**/*.js", ignore=["node_modules/**", "dist/**"]]

Found 15 JavaScript files (excluding node_modules, dist):
  scripts/build.js
  scripts/deploy.js
  ...
```

## Security Considerations

1. Path traversal prevention
2. Symlink loop detection
3. Result size limits
4. Performance limits

## Migration Path

1. **Phase 1**: Sorting and limits
2. **Phase 2**: Metadata support
3. **Phase 3**: Advanced patterns
4. **Phase 4**: Performance optimization

## References

- [fast-glob](https://github.com/mrmlnc/fast-glob)
- [Glob Patterns](https://en.wikipedia.org/wiki/Glob_(programming))
