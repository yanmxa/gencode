/**
 * Checkpoint Manager - Core checkpointing logic
 *
 * Tracks file changes and provides rewind capabilities.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type {
  FileCheckpoint,
  CheckpointSession,
  RewindOptions,
  RewindResult,
  CheckpointSummary,
  RecordChangeInput,
} from './types.js';

/**
 * Generates a unique ID for checkpoints
 */
function generateId(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

/**
 * CheckpointManager manages file change tracking and rewind operations.
 *
 * Usage:
 *   const manager = new CheckpointManager('session-123');
 *   manager.recordChange({ path: '/path/to/file', changeType: 'modify', ... });
 *   await manager.rewind({ all: true });
 */
export class CheckpointManager {
  private session: CheckpointSession;

  constructor(sessionId: string = 'default') {
    this.session = {
      sessionId,
      checkpoints: [],
      createdAt: new Date(),
    };
  }

  /**
   * Get the session ID
   */
  getSessionId(): string {
    return this.session.sessionId;
  }

  /**
   * Record a file change as a checkpoint
   */
  recordChange(input: RecordChangeInput): FileCheckpoint {
    const checkpoint: FileCheckpoint = {
      id: generateId(),
      path: input.path,
      changeType: input.changeType,
      timestamp: new Date(),
      previousContent: input.previousContent,
      newContent: input.newContent,
      toolName: input.toolName,
    };

    this.session.checkpoints.push(checkpoint);
    return checkpoint;
  }

  /**
   * Get all checkpoints
   */
  getCheckpoints(): FileCheckpoint[] {
    return [...this.session.checkpoints];
  }

  /**
   * Get checkpoints for a specific file
   */
  getFileHistory(filePath: string): FileCheckpoint[] {
    return this.session.checkpoints.filter((cp) => cp.path === filePath);
  }

  /**
   * Get a summary of all changes
   */
  getSummary(): CheckpointSummary {
    const summary: CheckpointSummary = {
      created: 0,
      modified: 0,
      deleted: 0,
      total: this.session.checkpoints.length,
    };

    for (const cp of this.session.checkpoints) {
      switch (cp.changeType) {
        case 'create':
          summary.created++;
          break;
        case 'modify':
          summary.modified++;
          break;
        case 'delete':
          summary.deleted++;
          break;
      }
    }

    return summary;
  }

  /**
   * Check if there are any checkpoints
   */
  hasCheckpoints(): boolean {
    return this.session.checkpoints.length > 0;
  }

  /**
   * Get the number of checkpoints
   */
  getCheckpointCount(): number {
    return this.session.checkpoints.length;
  }

  /**
   * Serialize checkpoints for JSON storage
   */
  serialize(): Array<{
    id: string;
    path: string;
    changeType: string;
    timestamp: string;
    previousContent: string | null;
    newContent: string | null;
    toolName: string;
  }> {
    return this.session.checkpoints.map((cp) => ({
      id: cp.id,
      path: cp.path,
      changeType: cp.changeType,
      timestamp: cp.timestamp.toISOString(),
      previousContent: cp.previousContent,
      newContent: cp.newContent,
      toolName: cp.toolName,
    }));
  }

  /**
   * Restore checkpoints from saved data
   */
  deserialize(
    data: Array<{
      id: string;
      path: string;
      changeType: string;
      timestamp: string;
      previousContent: string | null;
      newContent: string | null;
      toolName: string;
    }>
  ): void {
    this.session.checkpoints = data.map((item) => ({
      id: item.id,
      path: item.path,
      changeType: item.changeType as 'create' | 'modify' | 'delete',
      timestamp: new Date(item.timestamp),
      previousContent: item.previousContent,
      newContent: item.newContent,
      toolName: item.toolName,
    }));
  }

  /**
   * Rewind changes based on options
   */
  async rewind(options: RewindOptions): Promise<RewindResult> {
    const result: RewindResult = {
      success: true,
      revertedFiles: [],
      errors: [],
    };

    // Determine which checkpoints to rewind
    let checkpointsToRewind: FileCheckpoint[] = [];

    if (options.checkpointId) {
      // Rewind specific checkpoint
      const cp = this.session.checkpoints.find((c) => c.id === options.checkpointId);
      if (cp) {
        checkpointsToRewind = [cp];
      }
    } else if (options.path) {
      // Rewind all changes to a specific file (in reverse order)
      checkpointsToRewind = this.session.checkpoints
        .filter((c) => c.path === options.path)
        .reverse();
    } else if (options.count) {
      // Rewind last N changes (in reverse order)
      checkpointsToRewind = this.session.checkpoints.slice(-options.count).reverse();
    } else if (options.all) {
      // Rewind all changes (in reverse order)
      checkpointsToRewind = [...this.session.checkpoints].reverse();
    }

    // Apply reverts
    for (const checkpoint of checkpointsToRewind) {
      try {
        await this.revertCheckpoint(checkpoint);
        result.revertedFiles.push({
          path: checkpoint.path,
          action: this.getRevertAction(checkpoint),
        });

        // Remove the checkpoint from session
        const index = this.session.checkpoints.findIndex((c) => c.id === checkpoint.id);
        if (index !== -1) {
          this.session.checkpoints.splice(index, 1);
        }
      } catch (error) {
        result.success = false;
        result.errors.push({
          path: checkpoint.path,
          error: error instanceof Error ? error.message : String(error),
        });
      }
    }

    return result;
  }

  /**
   * Revert a single checkpoint
   */
  private async revertCheckpoint(checkpoint: FileCheckpoint): Promise<void> {
    switch (checkpoint.changeType) {
      case 'create':
        // File was created, delete it to revert
        await fs.unlink(checkpoint.path);
        break;

      case 'modify':
        // File was modified, restore previous content
        if (checkpoint.previousContent !== null) {
          await fs.writeFile(checkpoint.path, checkpoint.previousContent, 'utf-8');
        }
        break;

      case 'delete':
        // File was deleted, recreate it with previous content
        if (checkpoint.previousContent !== null) {
          await fs.mkdir(path.dirname(checkpoint.path), { recursive: true });
          await fs.writeFile(checkpoint.path, checkpoint.previousContent, 'utf-8');
        }
        break;
    }
  }

  /**
   * Get the action that will be taken to revert a checkpoint
   */
  private getRevertAction(checkpoint: FileCheckpoint): 'restored' | 'deleted' | 'recreated' {
    switch (checkpoint.changeType) {
      case 'create':
        return 'deleted';
      case 'modify':
        return 'restored';
      case 'delete':
        return 'recreated';
    }
  }

  /**
   * Clear all checkpoints
   */
  clearCheckpoints(): void {
    this.session.checkpoints = [];
  }

  /**
   * Format checkpoints for display
   */
  formatCheckpointList(includeUsage: boolean = false): string {
    if (this.session.checkpoints.length === 0) {
      return 'No file changes in this session.';
    }

    const lines: string[] = [];

    this.session.checkpoints.forEach((cp, index) => {
      const timeAgo = this.formatTimeAgo(cp.timestamp);
      const action = this.formatChangeType(cp.changeType);
      const fileName = cp.path.split('/').pop() || cp.path;
      lines.push(`  [${index + 1}] ${timeAgo.padEnd(8)} ${fileName.padEnd(30)} (${action})`);
    });

    const summary = this.getSummary();
    lines.push('');
    lines.push(
      `Total: ${summary.created} created, ${summary.modified} modified, ${summary.deleted} deleted`
    );

    if (includeUsage) {
      lines.push('');
      lines.push('Usage: /rewind [n] to revert change #n, /rewind all to revert all');
    }

    return lines.join('\n');
  }

  /**
   * Format a change type for display
   */
  private formatChangeType(changeType: string): string {
    switch (changeType) {
      case 'create':
        return 'created';
      case 'modify':
        return 'modified';
      case 'delete':
        return 'deleted';
      default:
        return changeType;
    }
  }

  /**
   * Format a timestamp as relative time
   */
  private formatTimeAgo(date: Date): string {
    const seconds = Math.floor((Date.now() - date.getTime()) / 1000);

    if (seconds < 60) {
      return `${seconds}s ago`;
    }

    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) {
      return `${minutes}m ago`;
    }

    const hours = Math.floor(minutes / 60);
    return `${hours}h ago`;
  }
}

// ============================================================================
// Singleton Instance
// ============================================================================

let globalCheckpointManager: CheckpointManager | null = null;

/**
 * Get the global checkpoint manager instance
 */
export function getCheckpointManager(): CheckpointManager {
  if (!globalCheckpointManager) {
    globalCheckpointManager = new CheckpointManager();
  }
  return globalCheckpointManager;
}

/**
 * Initialize checkpoint manager with a session ID
 */
export function initCheckpointManager(sessionId: string): CheckpointManager {
  globalCheckpointManager = new CheckpointManager(sessionId);
  return globalCheckpointManager;
}

/**
 * Reset the global checkpoint manager (for testing)
 */
export function resetCheckpointManager(): void {
  globalCheckpointManager = null;
}
