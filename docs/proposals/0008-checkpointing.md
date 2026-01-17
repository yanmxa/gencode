# Proposal: Checkpointing (Edit Undo/Rewind)

- **Proposal ID**: 0008
- **Author**: mycode team
- **Status**: Partially Implemented
- **Created**: 2025-01-15
- **Updated**: 2025-01-17

## Summary

Implement a checkpointing system that tracks all file changes made by the agent and enables users to rewind/undo unwanted modifications. This provides a safety net for experimentation and quick recovery from undesired changes.

## Motivation

Currently, mycode makes irreversible file changes. This leads to:

1. **Fear of damage**: Users hesitant to let agent modify files
2. **Manual recovery**: Must use git or backups to undo changes
3. **Lost work**: Accidental overwrites can't be recovered
4. **No visibility**: Can't see what files were changed
5. **No experimentation**: Can't easily try approaches and revert

A checkpointing system enables safe experimentation with easy rollback.

## Claude Code Reference

Claude Code provides automatic checkpointing:

### Key Features
- Automatic tracking of all file edits
- `/rewind` command to undo changes
- Change history visible in session
- Per-file and bulk rollback options
- Integration with git (commits as checkpoints)

### Example Usage
```
Agent: [Makes several file changes]
- Modified src/auth.ts
- Created src/middleware.ts
- Modified package.json

User: Actually, I don't like that approach. Undo those changes.

Agent: I'll rewind the file changes.
[Reverting 3 file changes...]
- Reverted src/auth.ts
- Deleted src/middleware.ts
- Reverted package.json

Changes have been undone.
```

### Change Tracking Display
```
Session Changes:
  [1] src/auth.ts (modified) - 10 minutes ago
  [2] src/middleware.ts (created) - 8 minutes ago
  [3] package.json (modified) - 5 minutes ago

Use /rewind [number] to undo specific changes
Use /rewind all to undo all changes
```

## Detailed Design

### API Design

```typescript
// src/checkpointing/types.ts
type ChangeType = 'create' | 'modify' | 'delete';

interface FileCheckpoint {
  id: string;
  path: string;
  changeType: ChangeType;
  timestamp: Date;
  previousContent: string | null;  // null for create
  newContent: string | null;       // null for delete
  toolName: string;                // Which tool made the change
}

interface CheckpointSession {
  id: string;
  sessionId: string;
  checkpoints: FileCheckpoint[];
  createdAt: Date;
}

interface RewindOptions {
  checkpointId?: string;    // Specific checkpoint
  path?: string;            // Specific file
  all?: boolean;            // All changes
  count?: number;           // Last N changes
}

interface RewindResult {
  success: boolean;
  revertedFiles: string[];
  errors: Array<{ path: string; error: string }>;
}
```

```typescript
// src/checkpointing/checkpoint-manager.ts
class CheckpointManager {
  private session: CheckpointSession;

  constructor(sessionId: string);

  // Record a file change
  recordChange(change: Omit<FileCheckpoint, 'id' | 'timestamp'>): void;

  // Get all checkpoints
  getCheckpoints(): FileCheckpoint[];

  // Get changes for a specific file
  getFileHistory(path: string): FileCheckpoint[];

  // Rewind changes
  async rewind(options: RewindOptions): Promise<RewindResult>;

  // Get summary of changes
  getSummary(): { created: number; modified: number; deleted: number };

  // Clear checkpoints (e.g., after git commit)
  clearCheckpoints(): void;
}
```

### Implementation Approach

1. **Tool Hooking**: Intercept Write and Edit tool executions
2. **Pre-Change Capture**: Store file content before modification
3. **Change Recording**: Log all changes with metadata
4. **Rewind Logic**: Apply inverse operations to restore state
5. **Persistence**: Store checkpoints in session data
6. **Git Integration**: Option to use git for versioning

```typescript
// Wrapping tools with checkpoint tracking
function withCheckpointing(tool: Tool, checkpointManager: CheckpointManager): Tool {
  return {
    ...tool,
    execute: async (input, context) => {
      // Capture pre-change state
      let previousContent: string | null = null;
      if (tool.name === 'Write' || tool.name === 'Edit') {
        previousContent = await safeReadFile(input.file_path);
      }

      // Execute original tool
      const result = await tool.execute(input, context);

      // Record checkpoint on success
      if (result.success) {
        const newContent = await safeReadFile(input.file_path);
        checkpointManager.recordChange({
          path: input.file_path,
          changeType: previousContent === null ? 'create' : 'modify',
          previousContent,
          newContent,
          toolName: tool.name
        });
      }

      return result;
    }
  };
}
```

### Rewind Command

```typescript
// src/commands/rewind-command.ts
async function rewindCommand(args: string[], context: CommandContext): Promise<void> {
  const { checkpointManager, ui } = context;

  if (args[0] === 'all') {
    const result = await checkpointManager.rewind({ all: true });
    ui.showMessage(`Reverted ${result.revertedFiles.length} files`);
  } else if (args[0]) {
    const checkpointId = args[0];
    const result = await checkpointManager.rewind({ checkpointId });
    ui.showMessage(`Reverted: ${result.revertedFiles.join(', ')}`);
  } else {
    // Show change list
    const checkpoints = checkpointManager.getCheckpoints();
    ui.showChangeList(checkpoints);
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/checkpointing/types.ts` | Create | Checkpoint type definitions |
| `src/checkpointing/checkpoint-manager.ts` | Create | Core checkpoint management |
| `src/checkpointing/tool-wrapper.ts` | Create | Tool wrapping for tracking |
| `src/checkpointing/index.ts` | Create | Module exports |
| `src/commands/rewind-command.ts` | Create | /rewind command |
| `src/tools/write.ts` | Modify | Add checkpoint integration |
| `src/tools/edit.ts` | Modify | Add checkpoint integration |
| `src/session/types.ts` | Modify | Add checkpoint data |

## User Experience

### Change Tracking Display
```
┌─ File Changes This Session ───────────────────┐
│                                               │
│ [1] 5 min ago  src/auth.ts         (modified) │
│ [2] 4 min ago  src/middleware.ts   (created)  │
│ [3] 2 min ago  package.json        (modified) │
│                                               │
│ Total: 2 modified, 1 created, 0 deleted       │
└───────────────────────────────────────────────┘
```

### Rewind Command
```
> /rewind

Session has 3 file changes:
  [1] src/auth.ts (modified) - 5 min ago
  [2] src/middleware.ts (created) - 4 min ago
  [3] package.json (modified) - 2 min ago

Usage:
  /rewind 1        - Revert specific change
  /rewind all      - Revert all changes
  /rewind last 2   - Revert last 2 changes
```

### Rewind Confirmation
```
> /rewind all

Are you sure you want to revert 3 file changes?
  - src/auth.ts (restore previous)
  - src/middleware.ts (delete)
  - package.json (restore previous)

[Confirm] [Cancel]
```

### Rewind Success
```
✓ Reverted 3 files:
  • src/auth.ts - restored
  • src/middleware.ts - deleted
  • package.json - restored

Checkpoints cleared.
```

### Inline Change Indicator
After each file modification:
```
Agent: [Edit] Modified src/auth.ts
       (checkpoint saved - use /rewind to undo)
```

## Alternatives Considered

### Alternative 1: Git-Only Versioning
Rely solely on git for undo.

**Pros**: No extra storage, standard workflow
**Cons**: Requires git, must commit to checkpoint
**Decision**: Rejected as primary - git integration as optional enhancement

### Alternative 2: Full File Backups
Store complete file copies.

**Pros**: Simple, reliable
**Cons**: Storage intensive for large files
**Decision**: Partially adopted - store content for reasonable sizes

### Alternative 3: Diff-Based Storage
Store only diffs between versions.

**Pros**: Space efficient
**Cons**: Complex, harder to apply reverts
**Decision**: Deferred - start simple, optimize later

## Security Considerations

1. **Sensitive Data**: Checkpoints may contain sensitive content
2. **Storage Limits**: Limit total checkpoint size
3. **Cleanup**: Clear checkpoints on session end
4. **File Permissions**: Respect file permissions when reverting
5. **Concurrent Access**: Handle external file modifications

## Testing Strategy

1. **Unit Tests**:
   - Change recording
   - Rewind logic for each change type
   - Multiple file reverts

2. **Integration Tests**:
   - Tool wrapping
   - Session persistence
   - Command handling

3. **Manual Testing**:
   - Various file operations
   - Partial reverts
   - Edge cases (binary files, permissions)

## Migration Path

1. **Phase 1**: Basic Write/Edit tracking and rewind
2. **Phase 2**: /rewind command with UI
3. **Phase 3**: Git integration (optional)
4. **Phase 4**: Selective rewind (by file, by time range)
5. **Phase 5**: Checkpoint browsing and diff viewing

No breaking changes to existing functionality.

## References

- [Claude Code Checkpointing](https://code.claude.com/docs/en/checkpointing)
- [Git Reset and Revert](https://git-scm.com/docs/git-reset)

## Implementation Status

### ✅ Implemented (Phase 1-2)

**Core Checkpointing System**:
- ✅ `CheckpointManager` class with full API
  - `recordChange()` - Record file changes
  - `getCheckpoints()` - Get all checkpoints
  - `getFileHistory()` - Get file-specific history
  - `rewind()` - Rewind with multiple options (checkpointId, path, all, count)
  - `getSummary()` - Get change summary
  - `clearCheckpoints()` - Clear checkpoints
  - `formatCheckpointList()` - Format for display

**Tool Integration**:
- ✅ Automatic tracking in `ToolRegistry`
  - Pre-execution file state capture
  - Post-execution checkpoint recording
  - Support for Write and Edit tools
  - Automatic detection of create/modify/delete operations

**CLI Commands**:
- ✅ `/changes` - List all file changes in session
- ✅ `/rewind [n]` - Rewind specific checkpoint by index
- ✅ `/rewind all` - Rewind all changes
- ✅ `/rewind` - Show changes list with usage info

**Type System**:
- ✅ Complete type definitions (`src/checkpointing/types.ts`)
- ✅ All interfaces from the proposal

**Testing**:
- ✅ Test example (`examples/test-checkpointing.ts`)

### ❌ Not Implemented (Phase 3-5)

**Session Persistence**:
- ❌ Checkpoints not saved to session files
- ❌ No checkpoint restoration when resuming sessions
- ❌ Session type doesn't include checkpoint data
- **Impact**: Checkpoints are lost when the session ends

**User Experience Enhancements**:
- ❌ Confirmation prompt for `/rewind all`
  - Currently executes immediately without confirmation
  - Proposal shows interactive confirmation UI
- ❌ Inline change indicators
  - Proposal shows checkpoint saved message after each file modification
  - Currently no visual feedback when checkpoint is created
- ❌ Formatted change list UI with boxes/borders
  - Current implementation uses simple text list
  - Proposal shows fancy bordered display

**Git Integration** (Phase 3):
- ❌ Optional git-based versioning
- ❌ Git commits as checkpoints
- ❌ Integration with git workflow

**Advanced Features** (Phase 4-5):
- ❌ Selective rewind by time range
- ❌ Checkpoint browsing UI
- ❌ Diff viewing between checkpoints
- ❌ Storage limits and cleanup policies
- ❌ Large file optimization

### 📋 Remaining Work

To complete this proposal, the following tasks are needed:

1. **Session Persistence** (High Priority):
   - Add `checkpoints` field to `SessionMetadata` or `Session` type
   - Save/load checkpoints in `SessionManager`
   - Restore checkpoint manager state when resuming sessions

2. **Confirmation UI** (Medium Priority):
   - Add confirmation prompt for `/rewind all` command
   - Show list of files that will be affected
   - Allow user to confirm or cancel

3. **Visual Feedback** (Medium Priority):
   - Show checkpoint saved message after Write/Edit operations
   - Improve `/changes` display with better formatting
   - Add color coding for different change types

4. **Git Integration** (Low Priority):
   - Optional: Use git for checkpoint storage
   - Optional: Create git commits as checkpoints
   - Optional: Integrate with existing git workflow

5. **Advanced Features** (Future):
   - Time-based rewind
   - Diff viewing
   - Storage optimization

### 📁 Implementation Files

| File | Status | Notes |
|------|--------|-------|
| `src/checkpointing/types.ts` | ✅ Complete | All types defined |
| `src/checkpointing/checkpoint-manager.ts` | ✅ Complete | Core logic implemented |
| `src/checkpointing/index.ts` | ✅ Complete | Module exports |
| `src/tools/registry.ts` | ✅ Modified | Checkpoint tracking added |
| `src/cli/components/App.tsx` | ✅ Modified | `/changes` and `/rewind` commands |
| `examples/test-checkpointing.ts` | ✅ Complete | Test coverage |
| `src/session/types.ts` | ❌ Not Modified | Missing checkpoint fields |
| `src/session/manager.ts` | ❌ Not Modified | No persistence logic |
