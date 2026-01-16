/**
 * Plan Mode State Management
 *
 * Singleton state manager for plan mode. Tracks whether plan mode
 * is active, current phase, and manages tool filtering.
 */

import type {
  PlanModeState,
  PlanPhase,
  AllowedPrompt,
  ModeType,
  PlanModeEvent,
} from './types.js';
import { PLAN_MODE_ALLOWED_TOOLS, PLAN_MODE_BLOCKED_TOOLS } from './types.js';

// ============================================================================
// State Manager
// ============================================================================

/**
 * Plan Mode State Manager
 *
 * Manages the plan mode state and provides methods for:
 * - Entering/exiting plan mode
 * - Phase transitions
 * - Tool filtering
 * - Event notifications
 */
export class PlanModeManager {
  private state: PlanModeState;
  private eventListeners: Set<(event: PlanModeEvent) => void>;

  constructor() {
    this.state = this.getInitialState();
    this.eventListeners = new Set();
  }

  /**
   * Get initial state
   */
  private getInitialState(): PlanModeState {
    return {
      active: false,
      phase: 'understanding',
      planFilePath: null,
      originalRequest: null,
      requestedPermissions: [],
      enteredAt: null,
    };
  }

  /**
   * Get current state (readonly)
   */
  getState(): Readonly<PlanModeState> {
    return { ...this.state };
  }

  /**
   * Check if plan mode is active
   */
  isActive(): boolean {
    return this.state.active;
  }

  /**
   * Get current mode type for UI
   * Note: Returns 'plan' or 'normal'. 'accept' mode is managed at the App level.
   */
  getCurrentMode(): ModeType {
    return this.state.active ? 'plan' : 'normal';
  }

  /**
   * Enter plan mode
   */
  enter(planFilePath: string, originalRequest?: string): void {
    this.state = {
      active: true,
      phase: 'understanding',
      planFilePath,
      originalRequest: originalRequest ?? null,
      requestedPermissions: [],
      enteredAt: new Date(),
    };

    this.emit({ type: 'enter', planFilePath });
  }

  /**
   * Exit plan mode
   */
  exit(approved: boolean = false): void {
    const wasActive = this.state.active;
    this.state = this.getInitialState();

    if (wasActive) {
      this.emit({ type: 'exit', approved });
    }
  }

  /**
   * Toggle plan mode (for Shift+Tab)
   */
  toggle(planFilePath?: string): void {
    if (this.state.active) {
      this.exit(false);
    } else if (planFilePath) {
      this.enter(planFilePath);
    }
    this.emit({ type: 'toggle' });
  }

  /**
   * Update phase
   */
  setPhase(phase: PlanPhase): void {
    if (this.state.active) {
      this.state.phase = phase;
      this.emit({ type: 'phase_change', phase });
    }
  }

  /**
   * Get current phase
   */
  getPhase(): PlanPhase {
    return this.state.phase;
  }

  /**
   * Set requested permissions (from ExitPlanMode)
   */
  setRequestedPermissions(permissions: AllowedPrompt[]): void {
    this.state.requestedPermissions = permissions;
  }

  /**
   * Get requested permissions
   */
  getRequestedPermissions(): AllowedPrompt[] {
    return [...this.state.requestedPermissions];
  }

  /**
   * Get plan file path
   */
  getPlanFilePath(): string | null {
    return this.state.planFilePath;
  }

  // ============================================================================
  // Tool Filtering
  // ============================================================================

  /**
   * Check if a tool is allowed in the current mode
   */
  isToolAllowed(toolName: string): boolean {
    if (!this.state.active) {
      return true; // All tools allowed in build mode
    }

    // In plan mode, only allow read-only tools
    return (PLAN_MODE_ALLOWED_TOOLS as readonly string[]).includes(toolName);
  }

  /**
   * Check if a tool is blocked in the current mode
   */
  isToolBlocked(toolName: string): boolean {
    if (!this.state.active) {
      return false; // No tools blocked in build mode
    }

    return (PLAN_MODE_BLOCKED_TOOLS as readonly string[]).includes(toolName);
  }

  /**
   * Get list of allowed tools for current mode
   */
  getAllowedTools(): string[] {
    if (!this.state.active) {
      return []; // Empty means all allowed
    }
    return [...PLAN_MODE_ALLOWED_TOOLS];
  }

  /**
   * Get list of blocked tools for current mode
   */
  getBlockedTools(): string[] {
    if (!this.state.active) {
      return [];
    }
    return [...PLAN_MODE_BLOCKED_TOOLS];
  }

  /**
   * Filter tool list based on current mode
   */
  filterTools(toolNames: string[]): string[] {
    if (!this.state.active) {
      return toolNames;
    }

    return toolNames.filter((name) => this.isToolAllowed(name));
  }

  // ============================================================================
  // Event System
  // ============================================================================

  /**
   * Subscribe to plan mode events
   */
  subscribe(listener: (event: PlanModeEvent) => void): () => void {
    this.eventListeners.add(listener);
    return () => {
      this.eventListeners.delete(listener);
    };
  }

  /**
   * Emit an event to all listeners
   */
  private emit(event: PlanModeEvent): void {
    for (const listener of this.eventListeners) {
      try {
        listener(event);
      } catch (error) {
        console.error('Error in plan mode event listener:', error);
      }
    }
  }
}

// ============================================================================
// Singleton Instance
// ============================================================================

/**
 * Global plan mode manager instance
 */
let globalPlanModeManager: PlanModeManager | null = null;

/**
 * Get the global plan mode manager
 */
export function getPlanModeManager(): PlanModeManager {
  if (!globalPlanModeManager) {
    globalPlanModeManager = new PlanModeManager();
  }
  return globalPlanModeManager;
}

/**
 * Reset the global plan mode manager (for testing)
 */
export function resetPlanModeManager(): void {
  if (globalPlanModeManager) {
    globalPlanModeManager.exit(false);
  }
  globalPlanModeManager = null;
}

// ============================================================================
// Convenience Functions
// ============================================================================

/**
 * Check if plan mode is currently active
 */
export function isPlanModeActive(): boolean {
  return getPlanModeManager().isActive();
}

/**
 * Get the current mode type
 */
export function getCurrentMode(): ModeType {
  return getPlanModeManager().getCurrentMode();
}

/**
 * Enter plan mode
 */
export function enterPlanMode(planFilePath: string, originalRequest?: string): void {
  getPlanModeManager().enter(planFilePath, originalRequest);
}

/**
 * Exit plan mode
 */
export function exitPlanMode(approved: boolean = false): void {
  getPlanModeManager().exit(approved);
}

/**
 * Toggle plan mode
 */
export function togglePlanMode(planFilePath?: string): void {
  getPlanModeManager().toggle(planFilePath);
}
