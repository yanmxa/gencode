/**
 * Hook Utilities - Helper functions for hooks system
 *
 * Functions:
 * - Variable expansion in commands
 * - Input sanitization
 * - JSON payload formatting
 * - Hook validation
 */

import type { HookDefinition, HookContext, HookStdinPayload } from './types.js';
import { STATUS_SYMBOLS } from '../common/format-utils.js';

// =============================================================================
// Variable Expansion
// =============================================================================

/**
 * Expand variables in a command string
 *
 * Supported variables:
 * - $TOOL_NAME - Name of the tool being executed
 * - $FILE_PATH - File path from tool input (if available)
 * - $COMMAND - Command from tool input (for Bash tool)
 * - $SESSION_ID - Current session ID
 * - $CWD - Current working directory
 *
 * Note: This is a simple string replacement. For security, we rely on
 * bash's argument parsing when using spawn(['bash', '-c', command]).
 *
 * @param command - Command string with variables
 * @param context - Hook execution context
 * @returns Command with variables expanded
 */
export function expandVariables(command: string, context: HookContext): string {
  let expanded = command;

  // Replace tool name
  if (context.toolName) {
    expanded = expanded.replace(/\$TOOL_NAME/g, context.toolName);
  }

  // Replace file path if available
  if (context.toolInput?.file_path && typeof context.toolInput.file_path === 'string') {
    expanded = expanded.replace(/\$FILE_PATH/g, context.toolInput.file_path);
  }

  // Replace command if available (for Bash tool hooks)
  if (context.toolInput?.command && typeof context.toolInput.command === 'string') {
    expanded = expanded.replace(/\$COMMAND/g, context.toolInput.command);
  }

  // Replace session ID
  if (context.sessionId) {
    expanded = expanded.replace(/\$SESSION_ID/g, context.sessionId);
  }

  // Replace working directory
  expanded = expanded.replace(/\$CWD/g, context.cwd);

  return expanded;
}

// =============================================================================
// JSON Payload
// =============================================================================

/**
 * Build JSON payload for hook stdin
 *
 * @param context - Hook execution context
 * @returns JSON payload object
 */
export function buildStdinPayload(context: HookContext): HookStdinPayload {
  return {
    session_id: context.sessionId || '',
    cwd: context.cwd,
    hook_event_name: context.event,
    tool_name: context.toolName || '',
    tool_input: context.toolInput || {},
  };
}

/**
 * Serialize payload to JSON string for stdin
 *
 * @param payload - Payload object
 * @param pretty - If true, pretty-print JSON (default: true)
 * @returns JSON string
 */
export function serializePayload(payload: HookStdinPayload, pretty = true): string {
  return JSON.stringify(payload, null, pretty ? 2 : undefined);
}

// =============================================================================
// Hook Validation
// =============================================================================

/**
 * Validate a hook definition
 *
 * @param hook - Hook definition to validate
 * @returns Validation result
 */
export function validateHook(hook: HookDefinition): {
  valid: boolean;
  errors: string[];
} {
  const errors: string[] = [];

  // Check type
  if (!hook.type) {
    errors.push('Hook must have a type');
  } else if (hook.type !== 'command' && hook.type !== 'prompt') {
    errors.push(`Invalid hook type: ${hook.type}`);
  }

  // Validate command hooks
  if (hook.type === 'command') {
    if (!hook.command) {
      errors.push('Command hook must have a command field');
    } else if (typeof hook.command !== 'string') {
      errors.push('Command must be a string');
    } else if (hook.command.trim() === '') {
      errors.push('Command cannot be empty');
    }
  }

  // Validate prompt hooks
  if (hook.type === 'prompt') {
    if (!hook.prompt) {
      errors.push('Prompt hook must have a prompt field');
    } else if (typeof hook.prompt !== 'string') {
      errors.push('Prompt must be a string');
    }
  }

  // Validate timeout
  if (hook.timeout !== undefined) {
    if (typeof hook.timeout !== 'number') {
      errors.push('Timeout must be a number');
    } else if (hook.timeout <= 0) {
      errors.push('Timeout must be positive');
    } else if (hook.timeout > 600000) {
      errors.push('Timeout cannot exceed 10 minutes (600000ms)');
    }
  }

  return {
    valid: errors.length === 0,
    errors,
  };
}

// =============================================================================
// Result Formatting
// =============================================================================

/**
 * Format hook result for display
 *
 * @param result - Hook result
 * @returns Formatted string
 */
export function formatResult(result: {
  success: boolean;
  output?: string;
  error?: string;
  blocked?: boolean;
  exitCode?: number;
  durationMs?: number;
}): string {
  const parts: string[] = [];

  // Status
  if (result.blocked) {
    parts.push(STATUS_SYMBOLS.BLOCKED);
  } else if (result.success) {
    parts.push(STATUS_SYMBOLS.SUCCESS);
  } else {
    parts.push(STATUS_SYMBOLS.FAILED);
  }

  // Exit code
  if (result.exitCode !== undefined) {
    parts.push(`(exit ${result.exitCode})`);
  }

  // Duration
  if (result.durationMs !== undefined) {
    parts.push(`${result.durationMs}ms`);
  }

  // Output or error
  if (result.output && result.output.trim()) {
    parts.push(`\nOutput: ${result.output.trim()}`);
  }
  if (result.error && result.error.trim()) {
    parts.push(`\nError: ${result.error.trim()}`);
  }

  return parts.join(' ');
}

// =============================================================================
// Path Sanitization
// =============================================================================

/**
 * Sanitize a file path to prevent path traversal
 *
 * @param filePath - File path to sanitize
 * @returns Sanitized path or null if invalid
 */
export function sanitizePath(filePath: string): string | null {
  // Reject paths with ..
  if (filePath.includes('..')) {
    return null;
  }

  // Reject absolute paths to sensitive directories
  const sensitive = ['/etc/', '/var/', '/sys/', '/proc/', '/dev/'];
  for (const dir of sensitive) {
    if (filePath.startsWith(dir)) {
      return null;
    }
  }

  return filePath;
}
