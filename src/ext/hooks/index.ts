/**
 * Hooks System - Event-driven hooks for tool execution and lifecycle events
 *
 * Main exports:
 * - HooksManager: Core orchestration class
 * - executeHook: Execute individual hooks
 * - matchesTool: Pattern matching for tool names
 * - Utility functions for validation, formatting, etc.
 *
 * @module hooks
 */

// Types
export type {
  HookEvent,
  HookType,
  HookDefinition,
  HookMatcher,
  HooksConfig,
  HookContext,
  HookResult,
  HookStdinPayload,
  TriggerOptions,
} from './types.js';

// Hooks Manager
export { HooksManager, hasBlockingResult, getFirstBlockingResult, allSucceeded, getTotalDuration } from './hooks-manager.js';

// Executor
export { executeHook } from './executor.js';

// Matcher
export { matchesTool, isValidRegex, testMatch } from './matcher.js';

// Utilities
export { expandVariables, buildStdinPayload, serializePayload, validateHook, formatResult, sanitizePath } from './utils.js';
