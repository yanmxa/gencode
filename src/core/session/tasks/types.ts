/**
 * Background Task System - Type Definitions
 *
 * Enables subagents to run in the background without blocking the main conversation.
 */

import type { SubagentType } from '../../../extensions/subagents/types.js';

/**
 * Status of a background task
 */
export type TaskStatus = 'pending' | 'running' | 'completed' | 'error' | 'cancelled';

/**
 * Background task metadata and state
 */
export interface BackgroundTask {
  /** Unique task ID (format: bg-{type}-{timestamp}-{random}) */
  id: string;

  /** Linked subagent ID */
  subagentId: string;

  /** Type of subagent running this task */
  subagentType: SubagentType;

  /** Short task description (3-5 words) */
  description: string;

  /** Current task status */
  status: TaskStatus;

  /** Path to NDJSON output log file */
  outputFile: string;

  /** Path to task metadata JSON file */
  metadataFile: string;

  /** Task start timestamp */
  startedAt: Date;

  /** Task completion timestamp (if completed/error/cancelled) */
  completedAt?: Date;

  /** Final result summary (if completed) */
  result?: string;

  /** Error message (if status is 'error') */
  error?: string;

  /** Token usage statistics */
  tokenUsage?: {
    input: number;
    output: number;
  };

  /** Total execution duration in milliseconds */
  durationMs?: number;

  /** Number of turns executed */
  turns?: number;

  /** Model used for execution */
  model?: string;

  /** Current progress (for running tasks) */
  progress?: {
    currentTurn: number;
    maxTurns: number;
  };
}

/**
 * Action types for TaskOutput tool
 */
export type TaskOutputAction = 'status' | 'list' | 'result' | 'wait' | 'cancel';

/**
 * Filter for listing tasks
 */
export type TaskListFilter = 'all' | 'running' | 'completed' | 'error';

/**
 * Input parameters for TaskOutput tool
 */
export interface TaskOutputInput {
  /** Action to perform */
  action: TaskOutputAction;

  /** Task ID (required for status/result/wait/cancel) */
  taskId?: string;

  /** Filter for list action */
  filter?: TaskListFilter;

  /** Timeout in milliseconds for wait action (max: 600000 = 10 min) */
  timeout?: number;

  /** Whether to block until task completes (for result action) */
  block?: boolean;
}

/**
 * Serializable version of BackgroundTask for JSON storage
 */
export interface BackgroundTaskJson {
  id: string;
  subagentId: string;
  subagentType: SubagentType;
  description: string;
  status: TaskStatus;
  outputFile: string;
  metadataFile: string;
  startedAt: string; // ISO 8601
  completedAt?: string; // ISO 8601
  result?: string;
  error?: string;
  tokenUsage?: {
    input: number;
    output: number;
  };
  durationMs?: number;
  turns?: number;
  model?: string;
  progress?: {
    currentTurn: number;
    maxTurns: number;
  };
}

/**
 * Task registry structure (persisted to disk)
 */
export interface TaskRegistry {
  /** Map of task ID to task metadata */
  tasks: Record<string, BackgroundTaskJson>;

  /** Last updated timestamp */
  lastUpdated: string; // ISO 8601
}
