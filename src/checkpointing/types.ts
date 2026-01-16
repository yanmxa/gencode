/**
 * Checkpointing System Type Definitions
 *
 * Provides automatic tracking of file changes with undo/rewind capabilities.
 */

/**
 * Type of file change
 */
export type ChangeType = 'create' | 'modify' | 'delete';

/**
 * A single file checkpoint recording a change
 */
export interface FileCheckpoint {
  /** Unique identifier for this checkpoint */
  id: string;
  /** Path to the file that was changed */
  path: string;
  /** Type of change made */
  changeType: ChangeType;
  /** When the change was made */
  timestamp: Date;
  /** File content before the change (null for create) */
  previousContent: string | null;
  /** File content after the change (null for delete) */
  newContent: string | null;
  /** Which tool made the change */
  toolName: string;
}

/**
 * A checkpoint session containing all checkpoints for a session
 */
export interface CheckpointSession {
  /** Session ID this checkpoint session belongs to */
  sessionId: string;
  /** All checkpoints in order */
  checkpoints: FileCheckpoint[];
  /** When this checkpoint session was created */
  createdAt: Date;
}

/**
 * Options for rewinding changes
 */
export interface RewindOptions {
  /** Rewind a specific checkpoint by ID */
  checkpointId?: string;
  /** Rewind changes to a specific file path */
  path?: string;
  /** Rewind all changes */
  all?: boolean;
  /** Rewind the last N changes */
  count?: number;
}

/**
 * Result of a rewind operation
 */
export interface RewindResult {
  /** Whether the rewind was successful */
  success: boolean;
  /** Files that were successfully reverted */
  revertedFiles: Array<{
    path: string;
    action: 'restored' | 'deleted' | 'recreated';
  }>;
  /** Any errors that occurred during rewind */
  errors: Array<{
    path: string;
    error: string;
  }>;
}

/**
 * Summary of changes in a checkpoint session
 */
export interface CheckpointSummary {
  /** Number of files created */
  created: number;
  /** Number of files modified */
  modified: number;
  /** Number of files deleted */
  deleted: number;
  /** Total number of checkpoints */
  total: number;
}

/**
 * Input for recording a file change
 */
export interface RecordChangeInput {
  /** Path to the file */
  path: string;
  /** Type of change */
  changeType: ChangeType;
  /** Content before change (null for create) */
  previousContent: string | null;
  /** Content after change (null for delete) */
  newContent: string | null;
  /** Tool that made the change */
  toolName: string;
}
