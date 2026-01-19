/**
 * Hook Executor - Executes individual hooks
 *
 * Handles:
 * - Shell command execution via spawn
 * - Timeout handling with SIGTERM
 * - Environment variable setup
 * - stdin JSON payload
 * - Exit code interpretation (0=success, 2=block, other=warn)
 */

import { spawn } from 'child_process';
import type { HookDefinition, HookContext, HookResult, HookStdinPayload } from './types.js';
import { isVerboseDebugEnabled } from '../common/debug.js';
import { logger } from '../common/logger.js';

// =============================================================================
// Constants
// =============================================================================

/** Default timeout for hook execution (60 seconds) */
const DEFAULT_TIMEOUT_MS = 60000;

/** Maximum output size (30KB, same as bash tool) */
const MAX_OUTPUT_SIZE = 30 * 1024;

// =============================================================================
// Main Executor
// =============================================================================

/**
 * Execute a hook based on its type
 *
 * @param hook - Hook definition to execute
 * @param context - Execution context
 * @returns Promise resolving to hook result
 */
export async function executeHook(hook: HookDefinition, context: HookContext): Promise<HookResult> {
  if (hook.type === 'command') {
    return executeCommandHook(hook, context);
  }

  if (hook.type === 'prompt') {
    // Prompt hooks not yet implemented
    return {
      success: false,
      error: 'Prompt hooks are not yet implemented',
      exitCode: 1,
      durationMs: 0,
    };
  }

  // Unknown hook type
  return {
    success: false,
    error: `Unknown hook type: ${(hook as HookDefinition).type}`,
    exitCode: 1,
    durationMs: 0,
  };
}

// =============================================================================
// Command Hook Execution
// =============================================================================

/**
 * Execute a shell command hook
 *
 * @param hook - Command hook definition
 * @param context - Execution context
 * @returns Promise resolving to hook result
 */
async function executeCommandHook(hook: HookDefinition, context: HookContext): Promise<HookResult> {
  const timeout = hook.timeout ?? DEFAULT_TIMEOUT_MS;
  const startTime = Date.now();

  // Verbose debug: Log hook execution start
  if (isVerboseDebugEnabled('hooks')) {
    logger.debug('Hook', `Executing hook`, {
      event: context.event,
      command: hook.command?.substring(0, 100) || 'none',
      blocking: hook.blocking,
      timeout,
    });
  }

  if (!hook.command) {
    return {
      success: false,
      error: 'Command hook missing command field',
      exitCode: 1,
      durationMs: 0,
    };
  }

  // Build stdin JSON payload
  const stdinPayload: HookStdinPayload = {
    session_id: context.sessionId || '',
    cwd: context.cwd,
    hook_event_name: context.event,
    tool_name: context.toolName || '',
    tool_input: context.toolInput || {},
  };

  const stdinData = JSON.stringify(stdinPayload, null, 2);

  // Verbose debug: Log stdin payload
  if (isVerboseDebugEnabled('hooks')) {
    logger.debug('Hook', `Sending stdin payload`, {
      payloadSize: stdinData.length,
      event: context.event,
      toolName: context.toolName || 'none',
    });
  }

  // Build environment variables
  const env = buildEnvironment(context);

  return new Promise((resolve) => {
    // Spawn bash process
    const proc = spawn('bash', ['-c', hook.command!], {
      cwd: context.cwd,
      env,
      stdio: ['pipe', 'pipe', 'pipe'],
    });

    let stdout = '';
    let stderr = '';
    let timedOut = false;
    let killed = false;

    // Timeout handling
    const timer = setTimeout(() => {
      timedOut = true;
      killed = true;
      proc.kill('SIGTERM');

      // Force kill after 1 second if still running
      setTimeout(() => {
        if (!proc.killed) {
          proc.kill('SIGKILL');
        }
      }, 1000);
    }, timeout);

    // Write JSON to stdin
    try {
      proc.stdin.write(stdinData);
      proc.stdin.end();
    } catch (error) {
      clearTimeout(timer);
      resolve({
        success: false,
        error: `Failed to write to stdin: ${error}`,
        exitCode: 1,
        durationMs: Date.now() - startTime,
      });
      return;
    }

    // Collect stdout
    proc.stdout.on('data', (data: Buffer) => {
      stdout += data.toString();
      // Truncate if too large
      if (stdout.length > MAX_OUTPUT_SIZE) {
        stdout = stdout.substring(0, MAX_OUTPUT_SIZE);
        if (!killed) {
          killed = true;
          proc.kill('SIGTERM');
        }
      }
    });

    // Collect stderr
    proc.stderr.on('data', (data: Buffer) => {
      stderr += data.toString();
      // Truncate if too large
      if (stderr.length > MAX_OUTPUT_SIZE) {
        stderr = stderr.substring(0, MAX_OUTPUT_SIZE);
        if (!killed) {
          killed = true;
          proc.kill('SIGTERM');
        }
      }
    });

    // Handle process completion
    proc.on('close', (code: number | null) => {
      clearTimeout(timer);
      const durationMs = Date.now() - startTime;

      // Determine success and blocking status
      const exitCode = code ?? 1;
      const success = exitCode === 0;
      const blocked = exitCode === 2;

      // Build error message
      let error: string | undefined;
      if (timedOut) {
        error = `Hook timed out after ${timeout}ms`;
      } else if (stderr) {
        error = stderr.trim();
      } else if (!success && !blocked) {
        error = `Hook failed with exit code ${exitCode}`;
      }

      // Verbose debug: Log hook execution complete
      if (isVerboseDebugEnabled('hooks')) {
        logger.debug('Hook', `Hook execution complete`, {
          event: context.event,
          exitCode,
          success,
          blocked,
          durationMs,
          stdoutLength: stdout.length,
          stderrLength: stderr.length,
          timedOut,
        });
      }

      resolve({
        success,
        output: stdout.trim(),
        error,
        blocked,
        exitCode,
        durationMs,
      });
    });

    // Handle process errors
    proc.on('error', (error: Error) => {
      clearTimeout(timer);
      resolve({
        success: false,
        error: `Failed to spawn hook process: ${error.message}`,
        exitCode: 1,
        durationMs: Date.now() - startTime,
      });
    });
  });
}

// =============================================================================
// Environment Setup
// =============================================================================

/**
 * Build environment variables for hook execution
 *
 * Includes:
 * - All parent process environment variables
 * - Hook-specific variables ($TOOL_NAME, $FILE_PATH, etc.)
 *
 * @param context - Hook execution context
 * @returns Environment object
 */
function buildEnvironment(context: HookContext): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {
    ...process.env,
  };

  // Add hook-specific variables
  env.TOOL_NAME = context.toolName || '';
  env.SESSION_ID = context.sessionId || '';
  env.CWD = context.cwd;
  env.HOOK_EVENT_NAME = context.event;

  // Add file path if available in tool input
  if (context.toolInput?.file_path && typeof context.toolInput.file_path === 'string') {
    env.FILE_PATH = context.toolInput.file_path;
  }

  // Add command if available in tool input (for Bash tool)
  if (context.toolInput?.command && typeof context.toolInput.command === 'string') {
    env.COMMAND = context.toolInput.command;
  }

  return env;
}
