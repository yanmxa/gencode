/**
 * Checkpointing Module
 *
 * Provides automatic tracking of file changes with undo/rewind capabilities.
 *
 * Usage:
 *   import { getCheckpointManager, initCheckpointManager } from './checkpointing';
 *
 *   // Initialize at session start
 *   const manager = initCheckpointManager('session-123');
 *
 *   // Record changes (done automatically by ToolRegistry)
 *   manager.recordChange({
 *     path: '/path/to/file.ts',
 *     changeType: 'modify',
 *     previousContent: 'old content',
 *     newContent: 'new content',
 *     toolName: 'Edit'
 *   });
 *
 *   // List changes
 *   console.log(manager.formatCheckpointList());
 *
 *   // Rewind changes
 *   await manager.rewind({ all: true });
 */

// Type exports
export type {
  ChangeType,
  FileCheckpoint,
  CheckpointSession,
  RewindOptions,
  RewindResult,
  CheckpointSummary,
  RecordChangeInput,
} from './types.js';

// Manager exports
export {
  CheckpointManager,
  getCheckpointManager,
  initCheckpointManager,
  resetCheckpointManager,
} from './checkpoint-manager.js';
