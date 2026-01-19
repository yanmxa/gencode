/**
 * TaskOutput Tool - Retrieve and manage background task outputs
 *
 * Allows the agent to:
 * - Check status of background tasks
 * - List all tasks with filters
 * - Retrieve results from completed tasks
 * - Wait for task completion
 * - Cancel running tasks
 */

import { z } from 'zod';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import type { TaskOutputInput, BackgroundTask } from '../../session/tasks/types.js';
import { TaskManager } from '../../session/tasks/task-manager.js';

/**
 * Format task status for display
 */
function formatTaskStatus(task: BackgroundTask): string {
  const lines: string[] = [];

  lines.push(`Task ID: ${task.id}`);
  lines.push(`Type: ${task.subagentType}`);
  lines.push(`Description: ${task.description}`);
  lines.push(`Status: ${task.status}`);

  if (task.status === 'running' && task.progress) {
    const { currentTurn, maxTurns } = task.progress;
    const percent = Math.round((currentTurn / maxTurns) * 100);
    lines.push(`Progress: ${currentTurn}/${maxTurns} turns (${percent}%)`);
  }

  const elapsed = task.completedAt
    ? task.completedAt.getTime() - task.startedAt.getTime()
    : Date.now() - task.startedAt.getTime();
  const elapsedSec = Math.round(elapsed / 1000);
  lines.push(`Duration: ${elapsedSec}s`);

  if (task.completedAt) {
    lines.push(`Completed: ${task.completedAt.toISOString()}`);
  }

  if (task.tokenUsage) {
    lines.push(
      `Tokens: ${task.tokenUsage.input} in / ${task.tokenUsage.output} out`
    );
  }

  if (task.error) {
    lines.push(`Error: ${task.error}`);
  }

  if (task.result) {
    lines.push(`\nResult:\n${task.result}`);
  }

  return lines.join('\n');
}

/**
 * Format task list for display
 */
function formatTasksList(tasks: BackgroundTask[]): string {
  if (tasks.length === 0) {
    return 'No background tasks found.';
  }

  const lines: string[] = [];
  lines.push(`Found ${tasks.length} task(s):\n`);

  for (const task of tasks) {
    const statusIcon =
      task.status === 'running'
        ? '⏳'
        : task.status === 'completed'
          ? '✅'
          : task.status === 'error'
            ? '❌'
            : task.status === 'cancelled'
              ? '🚫'
              : '⏸️';

    const elapsed = task.completedAt
      ? task.completedAt.getTime() - task.startedAt.getTime()
      : Date.now() - task.startedAt.getTime();
    const elapsedStr = formatDuration(elapsed);

    let statusStr: string = task.status;
    if (task.status === 'running' && task.progress) {
      const { currentTurn, maxTurns } = task.progress;
      statusStr = `running (${currentTurn}/${maxTurns})`;
    }

    lines.push(
      `${statusIcon} ${task.id} - ${task.description} [${statusStr}] (${elapsedStr})`
    );
  }

  return lines.join('\n');
}

/**
 * Format duration in human-readable format
 */
function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);

  if (hours > 0) return `${hours}h ${minutes % 60}m`;
  if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
  return `${seconds}s`;
}

/**
 * TaskOutput tool implementation
 */
export const taskoutputTool: Tool<TaskOutputInput> = {
  name: 'TaskOutput',

  description: `Retrieve output from background tasks.

Actions:
- status: Get current status of a specific task
- list: List all tasks (optional filter: running/completed/error)
- result: Get final result from a task (optionally block until complete)
- wait: Wait for task completion with timeout
- cancel: Cancel a running task

Examples:
  TaskOutput({ action: 'status', taskId: 'bg-explore-123' })
  TaskOutput({ action: 'list', filter: 'running' })
  TaskOutput({ action: 'result', taskId: 'bg-explore-123', block: true, timeout: 30000 })
  TaskOutput({ action: 'wait', taskId: 'bg-explore-123', timeout: 60000 })
  TaskOutput({ action: 'cancel', taskId: 'bg-explore-123' })`,

  parameters: z.object({
    action: z
      .enum(['status', 'list', 'result', 'wait', 'cancel'])
      .describe('Action to perform on background tasks'),
    taskId: z
      .string()
      .optional()
      .describe('Task ID (required for status/result/wait/cancel)'),
    filter: z
      .enum(['all', 'running', 'completed', 'error'])
      .optional()
      .describe('Filter for list action'),
    timeout: z
      .number()
      .min(0)
      .max(600000)
      .optional()
      .describe('Timeout in milliseconds (max 10 minutes)'),
    block: z
      .boolean()
      .optional()
      .describe('Block until task completes (for result action)'),
  }),

  async execute(input: TaskOutputInput, context: ToolContext): Promise<ToolResult> {
    const taskManager = new TaskManager();

    try {
      switch (input.action) {
        case 'status': {
          if (!input.taskId) {
            return {
              success: false,
              output: '',
              error: 'taskId is required for status action',
            };
          }

          const task = taskManager.getTask(input.taskId);
          if (!task) {
            return {
              success: false,
              output: '',
              error: `Task not found: ${input.taskId}`,
            };
          }

          return {
            success: true,
            output: formatTaskStatus(task),
          };
        }

        case 'list': {
          const filter = input.filter ?? 'all';
          const tasks = taskManager.listTasks(filter);
          return {
            success: true,
            output: formatTasksList(tasks),
          };
        }

        case 'result': {
          if (!input.taskId) {
            return {
              success: false,
              output: '',
              error: 'taskId is required for result action',
            };
          }

          // Wait for completion if block=true
          if (input.block) {
            const timeout = input.timeout ?? 30000;
            try {
              await taskManager.waitForTask(input.taskId, timeout);
            } catch (error) {
              return {
                success: false,
                output: '',
                error: error instanceof Error ? error.message : String(error),
              };
            }
          }

          const task = taskManager.getTask(input.taskId);
          if (!task) {
            return {
              success: false,
              output: '',
              error: `Task not found: ${input.taskId}`,
            };
          }

          if (task.status === 'running' || task.status === 'pending') {
            return {
              success: true,
              output: `Task is still ${task.status}. Use block=true to wait for completion.`,
            };
          }

          if (task.error) {
            return {
              success: false,
              output: '',
              error: `Task failed: ${task.error}`,
            };
          }

          return {
            success: true,
            output: task.result ?? '(no result available)',
          };
        }

        case 'wait': {
          if (!input.taskId) {
            return {
              success: false,
              output: '',
              error: 'taskId is required for wait action',
            };
          }

          const timeout = input.timeout ?? 60000;
          try {
            const task = await taskManager.waitForTask(input.taskId, timeout);
            return {
              success: true,
              output: `Task completed with status: ${task.status}`,
            };
          } catch (error) {
            return {
              success: false,
              output: '',
              error: error instanceof Error ? error.message : String(error),
            };
          }
        }

        case 'cancel': {
          if (!input.taskId) {
            return {
              success: false,
              output: '',
              error: 'taskId is required for cancel action',
            };
          }

          const cancelled = await taskManager.cancelTask(input.taskId);
          if (!cancelled) {
            return {
              success: false,
              output: '',
              error: 'Task is not running or not found',
            };
          }

          return {
            success: true,
            output: `Task ${input.taskId} has been cancelled`,
          };
        }

        default:
          return {
            success: false,
            output: '',
            error: `Unknown action: ${input.action}`,
          };
      }
    } catch (error) {
      return {
        success: false,
        output: '',
        error: error instanceof Error ? error.message : String(error),
      };
    }
  },
};
