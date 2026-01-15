# Proposal: Enhanced Grep Tool

- **Proposal ID**: 0037
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Enhance the Grep tool with advanced regex features, context lines, multiline matching, output modes, and performance optimizations for powerful content searching.

## Motivation

The current Grep tool has limitations:

1. **Basic regex**: No multiline support
2. **No context**: Just matching lines
3. **Single mode**: Only line output
4. **No pagination**: All results at once
5. **Limited control**: Few options

Enhanced grep enables sophisticated content search.

## Claude Code Reference

Claude Code's Grep tool provides comprehensive search:

### From Tool Description
```
- Supports full regex syntax (e.g., "log.*Error", "function\\s+\\w+")
- Filter files with glob or type parameter
- Output modes: "content", "files_with_matches", "count"
- Context lines with -A/-B/-C
- Multiline matching with multiline: true
- Line numbers with -n
```

### Key Features
- Multiple output modes
- Context lines (before/after)
- File type filtering
- Multiline patterns
- Offset/limit pagination

## Detailed Design

### API Design

```typescript
// src/tools/grep/types.ts
type OutputMode = 'content' | 'files_with_matches' | 'count';

interface GrepInput {
  pattern: string;
  path?: string;
  glob?: string;              // File pattern filter
  type?: string;              // File type (js, py, etc.)
  output_mode?: OutputMode;
  '-A'?: number;              // Lines after match
  '-B'?: number;              // Lines before match
  '-C'?: number;              // Context lines (both)
  '-i'?: boolean;             // Case insensitive
  '-n'?: boolean;             // Show line numbers
  multiline?: boolean;        // Cross-line matching
  head_limit?: number;        // Limit results
  offset?: number;            // Skip results
}

interface GrepMatch {
  file: string;
  line: number;
  column: number;
  content: string;
  context?: {
    before: string[];
    after: string[];
  };
}

interface GrepOutput {
  success: boolean;
  mode: OutputMode;
  matches?: GrepMatch[];
  files?: string[];
  counts?: Record<string, number>;
  total: number;
  truncated: boolean;
  error?: string;
}
```

### Enhanced Grep Implementation

```typescript
// src/tools/grep/grep-tool.ts
const grepTool: Tool<GrepInput> = {
  name: 'Grep',
  description: `Search file contents with regex patterns.

Parameters:
- pattern: Regex pattern to search for
- path: Directory or file to search (default: cwd)
- glob: Filter files by glob pattern (e.g., "*.ts")
- type: Filter by file type (js, py, rust, go, etc.)
- output_mode: "content", "files_with_matches", or "count"
- -A: Lines to show after match
- -B: Lines to show before match
- -C: Lines to show before and after match
- -i: Case insensitive search
- -n: Show line numbers (default: true)
- multiline: Enable cross-line matching
- head_limit: Limit number of results
- offset: Skip first N results

Output modes:
- content: Show matching lines with context
- files_with_matches: Show only file paths
- count: Show match counts per file
`,
  parameters: z.object({
    pattern: z.string(),
    path: z.string().optional(),
    glob: z.string().optional(),
    type: z.string().optional(),
    output_mode: z.enum(['content', 'files_with_matches', 'count']).optional(),
    '-A': z.number().int().nonnegative().optional(),
    '-B': z.number().int().nonnegative().optional(),
    '-C': z.number().int().nonnegative().optional(),
    '-i': z.boolean().optional(),
    '-n': z.boolean().optional().default(true),
    multiline: z.boolean().optional(),
    head_limit: z.number().int().positive().optional(),
    offset: z.number().int().nonnegative().optional()
  }),
  execute: async (input, context) => {
    return grepExecutor.search(input, context);
  }
};

class GrepExecutor {
  async search(input: GrepInput, context: ToolContext): Promise<GrepOutput> {
    const basePath = input.path
      ? path.resolve(context.cwd, input.path)
      : context.cwd;

    // Build regex
    let flags = 'g';
    if (input['-i']) flags += 'i';
    if (input.multiline) flags += 'ms';

    const regex = new RegExp(input.pattern, flags);

    // Find files to search
    const files = await this.findFiles(basePath, input.glob, input.type);

    // Search based on output mode
    const mode = input.output_mode || 'files_with_matches';

    switch (mode) {
      case 'content':
        return this.searchContent(files, regex, input);
      case 'files_with_matches':
        return this.searchFilesOnly(files, regex, input);
      case 'count':
        return this.searchCount(files, regex, input);
    }
  }

  private async searchContent(
    files: string[],
    regex: RegExp,
    input: GrepInput
  ): Promise<GrepOutput> {
    const matches: GrepMatch[] = [];
    const contextBefore = input['-B'] || input['-C'] || 0;
    const contextAfter = input['-A'] || input['-C'] || 0;

    for (const file of files) {
      const content = await fs.readFile(file, 'utf-8');
      const lines = content.split('\n');

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        let match = regex.exec(line);

        while (match) {
          const grepMatch: GrepMatch = {
            file: path.relative(process.cwd(), file),
            line: i + 1,
            column: match.index + 1,
            content: line
          };

          if (contextBefore > 0 || contextAfter > 0) {
            grepMatch.context = {
              before: lines.slice(Math.max(0, i - contextBefore), i),
              after: lines.slice(i + 1, i + 1 + contextAfter)
            };
          }

          matches.push(grepMatch);
          match = regex.exec(line);
        }

        // Reset regex for next line
        regex.lastIndex = 0;
      }
    }

    // Apply offset and limit
    const offset = input.offset || 0;
    const limit = input.head_limit;
    const truncated = limit ? matches.length > offset + limit : false;
    const paginatedMatches = limit
      ? matches.slice(offset, offset + limit)
      : matches.slice(offset);

    return {
      success: true,
      mode: 'content',
      matches: paginatedMatches,
      total: matches.length,
      truncated
    };
  }

  private async findFiles(
    basePath: string,
    glob?: string,
    type?: string
  ): Promise<string[]> {
    let pattern = '**/*';

    if (glob) {
      pattern = glob;
    } else if (type) {
      const extensions = this.getExtensionsForType(type);
      pattern = `**/*.{${extensions.join(',')}}`;
    }

    return globby(pattern, {
      cwd: basePath,
      absolute: true,
      ignore: ['**/node_modules/**', '**/.git/**']
    });
  }

  private getExtensionsForType(type: string): string[] {
    const typeMap: Record<string, string[]> = {
      js: ['js', 'mjs', 'cjs'],
      ts: ['ts', 'tsx', 'mts', 'cts'],
      py: ['py', 'pyi'],
      go: ['go'],
      rust: ['rs'],
      java: ['java'],
      c: ['c', 'h'],
      cpp: ['cpp', 'cxx', 'cc', 'hpp', 'hxx'],
      md: ['md', 'markdown'],
      json: ['json'],
      yaml: ['yaml', 'yml']
    };
    return typeMap[type] || [type];
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/grep/types.ts` | Create | Type definitions |
| `src/tools/grep/grep-tool.ts` | Modify | Enhanced implementation |
| `src/tools/grep/executor.ts` | Create | Search logic |
| `src/tools/grep/utils.ts` | Create | Utilities |

## User Experience

### Content with Context
```
Agent: [Grep: pattern="TODO", -C=2, output_mode="content"]

Found 5 matches:

src/agent/agent.ts:45
  43: async function processMessage() {
  44:   // Setup connection
> 45:   // TODO: Add retry logic
  46:   const response = await fetch(url);
  47: }

src/cli/ui.ts:112
  110: function renderOutput() {
  111:   // Format the response
> 112:   // TODO: Add syntax highlighting
  113:   console.log(output);
  114: }
...
```

### Files Only Mode
```
Agent: [Grep: pattern="import.*React", output_mode="files_with_matches", type="ts"]

Files containing pattern:
  src/components/Button.tsx
  src/components/Modal.tsx
  src/pages/Home.tsx
  src/pages/Settings.tsx
  src/App.tsx

5 files matched
```

### Count Mode
```
Agent: [Grep: pattern="console\\.log", output_mode="count"]

Match counts:
  src/cli/index.ts       12
  src/tools/bash.ts       8
  src/agent/agent.ts      5
  src/utils/debug.ts     23

Total: 48 matches in 4 files
```

### Multiline Match
```
Agent: [Grep: pattern="function\\s+\\w+\\([^)]*\\)\\s*\\{[\\s\\S]*?return", multiline=true]

Multiline matches found:
  src/utils/helpers.ts:15-22
    function calculateTotal(items) {
      let sum = 0;
      for (const item of items) {
        sum += item.price;
      }
      return sum;
    }
```

## Security Considerations

1. Regex DoS prevention (ReDoS)
2. File access restrictions
3. Output size limits
4. Search timeout

## Migration Path

1. **Phase 1**: Output modes
2. **Phase 2**: Context lines
3. **Phase 3**: Multiline support
4. **Phase 4**: Performance optimization

## References

- [ripgrep](https://github.com/BurntSushi/ripgrep)
- [ReDoS Prevention](https://owasp.org/www-community/attacks/Regular_expression_Denial_of_Service_-_ReDoS)
