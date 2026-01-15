# Proposal: MultiEdit Tool

- **Proposal ID**: 0013
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a MultiEdit tool that performs batch string replacements across multiple files in a single atomic operation. This enables efficient refactoring, renaming, and coordinated multi-file changes without multiple individual Edit tool calls.

## Motivation

Currently, mycode's Edit tool operates on one file at a time. This leads to:

1. **Inefficiency**: Renaming a variable across 10 files requires 10 separate Edit calls
2. **Non-atomic changes**: Partial failures leave codebase in inconsistent state
3. **Context overhead**: Each Edit call consumes conversation context
4. **Poor UX**: Users see many individual tool calls for conceptually single operation
5. **Error-prone**: Easy to miss files when doing manual multi-file edits

A MultiEdit tool enables atomic, efficient, multi-file operations.

## Claude Code Reference

Claude Code provides batch editing through its Edit tool with `replace_all` flag and coordinated multi-call patterns:

### Current Edit Tool Pattern
```typescript
Edit({
  file_path: "/path/to/file.ts",
  old_string: "oldName",
  new_string: "newName",
  replace_all: true  // Replace all occurrences in file
})
```

### BatchTool Pattern (from /init)
Claude Code uses BatchTool for coordinated operations:
```typescript
BatchTool({
  description: "Rename variable across files",
  invocations: [
    { tool_name: "Edit", input: { file_path: "file1.ts", ... } },
    { tool_name: "Edit", input: { file_path: "file2.ts", ... } },
    { tool_name: "Edit", input: { file_path: "file3.ts", ... } }
  ]
})
```

### Key Characteristics
- Atomic: All or nothing execution
- Parallel execution where possible
- Rollback on failure
- Single result summary

## Detailed Design

### API Design

```typescript
// src/tools/multi-edit/types.ts
interface FileEdit {
  file_path: string;
  old_string: string;
  new_string: string;
  replace_all?: boolean;  // Default: false
}

interface MultiEditInput {
  description: string;     // What this batch edit does
  edits: FileEdit[];       // Array of file edits
  atomic?: boolean;        // Rollback all on any failure (default: true)
  dry_run?: boolean;       // Preview changes without applying
}

interface EditResult {
  file_path: string;
  success: boolean;
  replacements: number;    // Number of replacements made
  error?: string;
}

interface MultiEditOutput {
  success: boolean;
  results: EditResult[];
  summary: {
    total_files: number;
    successful_files: number;
    failed_files: number;
    total_replacements: number;
  };
  error?: string;
  rollback_performed?: boolean;
}
```

```typescript
// src/tools/multi-edit/multi-edit-tool.ts
const multiEditTool: Tool<MultiEditInput> = {
  name: 'MultiEdit',
  description: `Perform batch edits across multiple files atomically.

Features:
- Edit multiple files in a single operation
- Atomic mode: rollback all changes if any edit fails
- Dry run mode: preview changes without applying
- Replace all occurrences within each file

Usage:
- Provide array of edits with file paths and replacements
- Each edit specifies old_string and new_string
- Use replace_all: true for global replacement in file
- Use dry_run: true to preview changes

Best for:
- Variable/function renaming across codebase
- Import path updates
- Configuration value changes
- License header updates
`,
  parameters: z.object({
    description: z.string().min(1),
    edits: z.array(z.object({
      file_path: z.string(),
      old_string: z.string(),
      new_string: z.string(),
      replace_all: z.boolean().optional()
    })).min(1),
    atomic: z.boolean().optional().default(true),
    dry_run: z.boolean().optional().default(false)
  }),
  execute: async (input, context) => { ... }
};
```

### Implementation Approach

1. **Pre-validation**: Verify all files exist and are readable
2. **Backup creation**: Store original content for rollback
3. **Parallel execution**: Process independent files concurrently
4. **Atomic commit**: Apply all changes or rollback on failure
5. **Result aggregation**: Summarize all edit results

```typescript
// Core implementation
async function executeMultiEdit(
  input: MultiEditInput,
  context: ToolContext
): Promise<MultiEditOutput> {
  const { edits, atomic, dry_run } = input;
  const backups: Map<string, string> = new Map();
  const results: EditResult[] = [];

  // Phase 1: Validate and backup
  for (const edit of edits) {
    const fullPath = resolvePath(edit.file_path, context.cwd);
    if (!await fileExists(fullPath)) {
      if (atomic) {
        return { success: false, error: `File not found: ${edit.file_path}` };
      }
      results.push({ file_path: edit.file_path, success: false, error: 'File not found' });
      continue;
    }
    const content = await readFile(fullPath);
    backups.set(fullPath, content);
  }

  // Phase 2: Apply edits
  for (const edit of edits) {
    const fullPath = resolvePath(edit.file_path, context.cwd);
    const original = backups.get(fullPath);
    if (!original) continue;

    // Check if old_string exists
    if (!original.includes(edit.old_string)) {
      if (atomic) {
        await rollbackAll(backups);
        return { success: false, error: `String not found in ${edit.file_path}` };
      }
      results.push({ file_path: edit.file_path, success: false, error: 'String not found' });
      continue;
    }

    // Perform replacement
    const newContent = edit.replace_all
      ? original.replaceAll(edit.old_string, edit.new_string)
      : original.replace(edit.old_string, edit.new_string);

    const replacements = countReplacements(original, newContent, edit.old_string);

    if (!dry_run) {
      await writeFile(fullPath, newContent);
    }

    results.push({
      file_path: edit.file_path,
      success: true,
      replacements
    });
  }

  // Phase 3: Generate summary
  return {
    success: results.every(r => r.success),
    results,
    summary: {
      total_files: edits.length,
      successful_files: results.filter(r => r.success).length,
      failed_files: results.filter(r => !r.success).length,
      total_replacements: results.reduce((sum, r) => sum + (r.replacements || 0), 0)
    }
  };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/tools/multi-edit/types.ts` | Create | Type definitions |
| `src/tools/multi-edit/multi-edit-tool.ts` | Create | Tool implementation |
| `src/tools/multi-edit/index.ts` | Create | Module exports |
| `src/tools/index.ts` | Modify | Register MultiEdit tool |

## User Experience

### Basic Usage - Variable Rename
```
User: Rename the function 'getUserData' to 'fetchUserProfile' across the codebase

Agent: I'll search for all occurrences and perform a batch rename.
[Grep: getUserData]

Found in 5 files. Performing batch edit:

┌─ MultiEdit ───────────────────────────────────────┐
│ Description: Rename getUserData to fetchUserProfile │
│ Files: 5                                           │
│ Mode: Atomic                                       │
└───────────────────────────────────────────────────┘

Results:
✓ src/api/users.ts        (3 replacements)
✓ src/services/auth.ts    (2 replacements)
✓ src/hooks/useUser.ts    (1 replacement)
✓ src/tests/users.test.ts (4 replacements)
✓ src/types/api.ts        (1 replacement)

Summary: 5/5 files updated, 11 total replacements
```

### Dry Run Preview
```
Agent: Let me preview the changes first:

[MultiEdit: dry_run=true]

Preview (no changes made):
• src/api/users.ts: Would make 3 replacements
• src/services/auth.ts: Would make 2 replacements
...

Proceed with these changes?
```

### Atomic Rollback
```
Agent: [MultiEdit: atomic=true]

Error in src/services/auth.ts: String not found
Rolling back changes to 3 files...
Rollback complete. No files were modified.
```

## Alternatives Considered

### Alternative 1: Enhanced Single Edit
Add multi-file support to existing Edit tool.

**Pros**: No new tool, simpler API
**Cons**: Overloads Edit semantics, breaks single-responsibility
**Decision**: Rejected - Separate tool is cleaner

### Alternative 2: Parallel Edit Calls
Let agent make multiple Edit calls in parallel.

**Pros**: No new implementation needed
**Cons**: No atomicity, no rollback, context overhead
**Decision**: Rejected - Atomicity is key feature

### Alternative 3: File Glob Pattern
Support glob patterns for target files.

**Pros**: Concise syntax for many files
**Cons**: Less precise, risk of unintended edits
**Decision**: Deferred - Can add later

## Security Considerations

1. **Path Validation**: Ensure all paths are within allowed directories
2. **Backup Storage**: Store backups securely in temp directory
3. **Size Limits**: Limit number of files and total content size
4. **Permission Check**: Verify write permission before starting
5. **No Secret Exposure**: Don't log file contents in error messages

```typescript
const MAX_FILES = 100;
const MAX_TOTAL_SIZE = 50 * 1024 * 1024;  // 50MB
```

## Testing Strategy

1. **Unit Tests**:
   - Single file edit
   - Multi-file edit
   - Atomic rollback
   - Dry run mode
   - Error handling

2. **Integration Tests**:
   - Large batch edits
   - Permission scenarios
   - Concurrent access

3. **Manual Testing**:
   - Variable renaming workflow
   - Import path updates
   - Cross-platform paths

## Migration Path

1. **Phase 1**: Basic multi-edit with atomic mode
2. **Phase 2**: Dry run preview
3. **Phase 3**: Rollback functionality
4. **Phase 4**: Performance optimization for large batches

No breaking changes to existing Edit tool.

## References

- [Claude Code Edit Tool Documentation](https://code.claude.com/docs/en/tools)
- [Atomic File Operations Best Practices](https://en.wikipedia.org/wiki/Atomicity_(database_systems))
- [Existing Edit Tool Implementation](../../../src/tools/builtin/edit.ts)
