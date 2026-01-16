/**
 * TodoWrite Tool - Manage task list for tracking progress
 */

import type { Tool, ToolResult, TodoItem } from '../types.js';
import { TodoWriteInputSchema, type TodoWriteInput } from '../types.js';
import { loadToolDescription } from '../../prompts/index.js';

// Global todo state - shared across tool invocations
let currentTodos: TodoItem[] = [];

/**
 * Get the current todo list
 */
export function getTodos(): TodoItem[] {
  return [...currentTodos];
}

/**
 * Clear all todos
 */
export function clearTodos(): void {
  currentTodos = [];
}

/**
 * Format todos for display
 */
function formatTodos(todos: TodoItem[]): string {
  if (todos.length === 0) {
    return 'Todo list is empty.';
  }

  const statusIcons: Record<string, string> = {
    pending: '[ ]',
    in_progress: '[>]',
    completed: '[x]',
  };

  const lines = todos.map((todo, index) => {
    const icon = statusIcons[todo.status] || '[ ]';
    return `${index + 1}. ${icon} ${todo.content}`;
  });

  return lines.join('\n');
}

/**
 * Validate todo list rules
 */
function validateTodos(todos: TodoItem[]): string | null {
  const inProgress = todos.filter((t) => t.status === 'in_progress');
  if (inProgress.length > 1) {
    return `Only one task should be in_progress at a time. Found ${inProgress.length} tasks in progress.`;
  }
  return null;
}

export const todowriteTool: Tool<TodoWriteInput> = {
  name: 'TodoWrite',
  description: loadToolDescription('todowrite'),
  parameters: TodoWriteInputSchema,

  async execute(input): Promise<ToolResult> {
    try {
      // Validate the todo list
      const validationError = validateTodos(input.todos);
      if (validationError) {
        return {
          success: false,
          output: '',
          error: validationError,
        };
      }

      // Update the global todo state (auto-generate id if missing)
      currentTodos = input.todos.map((todo, index) => ({
        ...todo,
        id: todo.id || `todo-${index + 1}`,
      }));

      // Count statistics
      const pending = currentTodos.filter((t) => t.status === 'pending').length;
      const inProgress = currentTodos.filter((t) => t.status === 'in_progress').length;
      const completed = currentTodos.filter((t) => t.status === 'completed').length;

      const summary = `Todos updated: ${completed} completed, ${inProgress} in progress, ${pending} pending`;
      const formatted = formatTodos(currentTodos);

      return {
        success: true,
        output: `${summary}\n\n${formatted}`,
      };
    } catch (error) {
      return {
        success: false,
        output: '',
        error: `Failed to update todos: ${error instanceof Error ? error.message : String(error)}`,
      };
    }
  },
};
