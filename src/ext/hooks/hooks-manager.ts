/**
 * Hooks Manager - Core orchestration for hook execution
 *
 * Responsibilities:
 * - Load hooks configuration
 * - Filter hooks by event and matcher
 * - Execute matching hooks (parallel or sequential)
 * - Aggregate and return results
 */

import type {
  HooksConfig,
  HookEvent,
  HookContext,
  HookResult,
  HookDefinition,
  TriggerOptions,
} from './types.js';
import { executeHook } from './executor.js';
import { matchesTool } from './matcher.js';

// =============================================================================
// Hooks Manager Class
// =============================================================================

export class HooksManager {
  private config: HooksConfig;

  /**
   * Create a new hooks manager
   *
   * @param config - Hooks configuration from settings.json
   */
  constructor(config: HooksConfig = {}) {
    this.config = config;
  }

  // ===========================================================================
  // Public API
  // ===========================================================================

  /**
   * Trigger hooks for a specific event
   *
   * @param event - Hook event to trigger
   * @param context - Execution context
   * @param options - Trigger options
   * @returns Promise resolving to array of hook results
   */
  async trigger(
    event: HookEvent,
    context: HookContext,
    options: TriggerOptions = {}
  ): Promise<HookResult[]> {
    const {
      parallel = true,
      stopOnBlock = true,
      maxHooks = Number.POSITIVE_INFINITY,
    } = options;

    // Get matching hooks for this event
    const matchingHooks = this.getMatchingHooks(event, context.toolName);

    // Limit number of hooks if maxHooks is set
    const hooksToExecute = matchingHooks.slice(0, maxHooks);

    if (hooksToExecute.length === 0) {
      return [];
    }

    // Execute hooks
    if (parallel) {
      return this.executeParallel(hooksToExecute, context);
    } else {
      return this.executeSequential(hooksToExecute, context, stopOnBlock);
    }
  }

  /**
   * Check if any hooks are configured for an event
   *
   * @param event - Event to check
   * @returns true if hooks exist for this event
   */
  hasHooks(event: HookEvent): boolean {
    const matchers = this.config[event];
    return !!matchers && matchers.length > 0;
  }

  /**
   * Get all hook matchers for an event
   *
   * @param event - Event to get matchers for
   * @returns Array of hook matchers
   */
  getMatchers(event: HookEvent) {
    return this.config[event] || [];
  }

  /**
   * Update hooks configuration
   *
   * @param config - New hooks configuration
   */
  setConfig(config: HooksConfig): void {
    this.config = config;
  }

  /**
   * Get current hooks configuration
   *
   * @returns Current hooks configuration
   */
  getConfig(): HooksConfig {
    return this.config;
  }

  // ===========================================================================
  // Private Methods
  // ===========================================================================

  /**
   * Get all hooks that match an event and tool name
   *
   * @param event - Hook event
   * @param toolName - Tool name (optional)
   * @returns Array of matching hook definitions
   */
  private getMatchingHooks(event: HookEvent, toolName?: string): HookDefinition[] {
    const matchers = this.config[event] || [];
    const matchingHooks: HookDefinition[] = [];

    for (const matcher of matchers) {
      // Check if matcher pattern matches the tool name
      if (matchesTool(matcher.matcher, toolName)) {
        matchingHooks.push(...matcher.hooks);
      }
    }

    return matchingHooks;
  }

  /**
   * Execute hooks in parallel
   *
   * All hooks run simultaneously, results collected when all complete.
   *
   * @param hooks - Hooks to execute
   * @param context - Execution context
   * @returns Promise resolving to array of results
   */
  private async executeParallel(hooks: HookDefinition[], context: HookContext): Promise<HookResult[]> {
    const promises = hooks.map((hook) => executeHook(hook, context));
    return Promise.all(promises);
  }

  /**
   * Execute hooks sequentially
   *
   * Hooks run one at a time, optionally stopping on first blocking hook.
   *
   * @param hooks - Hooks to execute
   * @param context - Execution context
   * @param stopOnBlock - If true, stop on first blocking hook
   * @returns Promise resolving to array of results
   */
  private async executeSequential(
    hooks: HookDefinition[],
    context: HookContext,
    stopOnBlock: boolean
  ): Promise<HookResult[]> {
    const results: HookResult[] = [];

    for (const hook of hooks) {
      const result = await executeHook(hook, context);
      results.push(result);

      // Stop if hook blocked and stopOnBlock is true
      if (result.blocked && stopOnBlock) {
        break;
      }
    }

    return results;
  }
}

// =============================================================================
// Utility Functions
// =============================================================================

/**
 * Check if any hook results indicate blocking
 *
 * @param results - Array of hook results
 * @returns true if any hook blocked
 */
export function hasBlockingResult(results: HookResult[]): boolean {
  return results.some((result) => result.blocked);
}

/**
 * Get first blocking result
 *
 * @param results - Array of hook results
 * @returns First blocking result, or undefined if none
 */
export function getFirstBlockingResult(results: HookResult[]): HookResult | undefined {
  return results.find((result) => result.blocked);
}

/**
 * Check if all hooks succeeded
 *
 * @param results - Array of hook results
 * @returns true if all hooks succeeded
 */
export function allSucceeded(results: HookResult[]): boolean {
  return results.every((result) => result.success);
}

/**
 * Get total execution time for all hooks
 *
 * @param results - Array of hook results
 * @returns Total duration in milliseconds
 */
export function getTotalDuration(results: HookResult[]): number {
  return results.reduce((total, result) => total + (result.durationMs || 0), 0);
}
