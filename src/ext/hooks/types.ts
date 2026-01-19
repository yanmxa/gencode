/**
 * Hooks System Type Definitions
 *
 * Event-driven hooks system for executing shell commands or scripts
 * in response to specific events (tool execution, prompts, sessions).
 *
 * Based on Claude Code's hooks architecture.
 */

import type { ToolResult } from '../../core/tools/types.js';

// =============================================================================
// Hook Event Types
// =============================================================================

/**
 * Hook events that can trigger hook execution
 *
 * Tool Events:
 * - PreToolUse: Before tool execution
 * - PostToolUse: After successful tool execution
 * - PostToolUseFailure: After failed tool execution
 *
 * User Events:
 * - UserPromptSubmit: Before processing user prompt
 *
 * Session Events:
 * - SessionStart: Session begins
 * - SessionEnd: Session ends
 * - Stop: Main agent finishes
 */
export type HookEvent =
  | 'PreToolUse'
  | 'PostToolUse'
  | 'PostToolUseFailure'
  | 'UserPromptSubmit'
  | 'SessionStart'
  | 'SessionEnd'
  | 'Stop';

// =============================================================================
// Hook Configuration Types
// =============================================================================

/**
 * Hook type defines the execution strategy
 * - command: Execute shell command
 * - prompt: Use LLM to evaluate (future)
 */
export type HookType = 'command' | 'prompt';

/**
 * Definition for a single hook
 */
export interface HookDefinition {
  /** Hook execution type */
  type: HookType;

  /** Shell command to execute (for command type) */
  command?: string;

  /** LLM prompt to evaluate (for prompt type - future) */
  prompt?: string;

  /** Maximum execution time in milliseconds (default: 60000) */
  timeout?: number;

  /** Display message while hook is running */
  statusMessage?: string;

  /** If true, wait for hook completion before proceeding */
  blocking?: boolean;
}

/**
 * Matcher configuration for filtering hooks by tool name
 */
export interface HookMatcher {
  /**
   * Pattern to match tool names
   * - undefined or "*" or "": Match all tools
   * - "ToolName": Exact match
   * - "Tool1|Tool2": Regex alternation
   */
  matcher?: string;

  /** Hooks to execute when matcher succeeds */
  hooks: HookDefinition[];
}

/**
 * Complete hooks configuration (settings.json format)
 *
 * Example:
 * ```json
 * {
 *   "PostToolUse": [
 *     {
 *       "matcher": "Write|Edit",
 *       "hooks": [
 *         {
 *           "type": "command",
 *           "command": "npm run lint:fix $FILE_PATH",
 *           "timeout": 5000
 *         }
 *       ]
 *     }
 *   ]
 * }
 * ```
 */
export interface HooksConfig {
  [event: string]: HookMatcher[];
}

// =============================================================================
// Hook Execution Types
// =============================================================================

/**
 * Context available to hooks during execution
 */
export interface HookContext {
  /** Event that triggered the hook */
  event: HookEvent;

  /** Current session ID (if available) */
  sessionId?: string;

  /** Current working directory */
  cwd: string;

  /** Tool name (for tool-related events) */
  toolName?: string;

  /** Tool input parameters (for tool-related events) */
  toolInput?: Record<string, unknown>;

  /** Tool result (for PostToolUse events) */
  toolResult?: ToolResult;

  /** Timestamp when hook was triggered */
  timestamp: Date;
}

/**
 * Result from hook execution
 */
export interface HookResult {
  /** Whether hook executed successfully (exit code 0) */
  success: boolean;

  /** stdout from hook command */
  output?: string;

  /** stderr from hook command or error message */
  error?: string;

  /** If true, hook blocked the action (exit code 2) */
  blocked?: boolean;

  /** Process exit code */
  exitCode?: number;

  /** Execution duration in milliseconds */
  durationMs?: number;
}

// =============================================================================
// Hook Stdin Payload
// =============================================================================

/**
 * JSON payload sent to hooks via stdin
 *
 * Hooks receive this data on stdin and can use it to make decisions.
 */
export interface HookStdinPayload {
  /** Session ID */
  session_id: string;

  /** Current working directory */
  cwd: string;

  /** Hook event name */
  hook_event_name: string;

  /** Tool name (for tool events) */
  tool_name: string;

  /** Tool input (for tool events) */
  tool_input: Record<string, unknown>;
}

// =============================================================================
// Hook Manager Types
// =============================================================================

/**
 * Options for triggering hooks
 */
export interface TriggerOptions {
  /** If true, run hooks in parallel (default: true) */
  parallel?: boolean;

  /** If true, stop on first blocking hook (default: true) */
  stopOnBlock?: boolean;

  /** Maximum number of hooks to execute (default: unlimited) */
  maxHooks?: number;
}
