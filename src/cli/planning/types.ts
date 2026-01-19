/**
 * Plan Mode Type Definitions
 *
 * Types for Plan Mode - a read-only exploration mode that allows
 * the LLM to plan implementations before executing changes.
 */

// ============================================================================
// Plan Mode State Types
// ============================================================================

/**
 * Plan mode phases following Claude Code's workflow
 */
export type PlanPhase =
  | 'understanding' // Initial exploration
  | 'design' // Planning approach
  | 'review' // Clarifying with user
  | 'final' // Writing plan file
  | 'approval'; // Waiting for user approval

/**
 * User approval options for plan mode
 */
export type PlanApprovalOption =
  | 'approve' // Accept plan and execute with auto-accept edits
  | 'approve_clear' // Accept plan, clear context, auto-accept edits
  | 'approve_manual' // Accept plan but manually approve each edit
  | 'approve_manual_keep' // Accept plan, keep context, manually approve each edit
  | 'modify' // Go back to modify the plan
  | 'cancel'; // Cancel plan mode entirely

/**
 * Pre-approved permission request (Claude Code ExitPlanMode style)
 */
export interface AllowedPrompt {
  tool: 'Bash';
  prompt: string; // Semantic description, e.g., "run tests", "install dependencies"
}

/**
 * Plan mode state
 */
export interface PlanModeState {
  /** Whether plan mode is currently active */
  active: boolean;

  /** Current phase of planning */
  phase: PlanPhase;

  /** Path to the plan file */
  planFilePath: string | null;

  /** User's original request that triggered plan mode */
  originalRequest: string | null;

  /** Requested permissions for execution phase */
  requestedPermissions: AllowedPrompt[];

  /** Timestamp when plan mode was entered */
  enteredAt: Date | null;
}

/**
 * Plan file structure
 */
export interface PlanFile {
  /** Plan file path */
  path: string;

  /** Plan content in markdown */
  content: string;

  /** Creation timestamp */
  createdAt: Date;

  /** Last update timestamp */
  updatedAt: Date;
}

// ============================================================================
// Tool Filtering Types
// ============================================================================

/**
 * Tools allowed in plan mode (read-only + planning tools)
 * Note: Write is allowed but restricted to plan file only (enforced at execution time)
 */
export const PLAN_MODE_ALLOWED_TOOLS = [
  'Read',
  'Glob',
  'Grep',
  'WebFetch',
  'WebSearch',
  'TodoWrite',
  'AskUserQuestion',
  'EnterPlanMode', // Can re-enter if needed
  'ExitPlanMode', // To exit plan mode
  'Write', // Allowed but restricted to plan file only (enforced in write.ts)
] as const;

/**
 * Tools blocked in plan mode (write/execute operations)
 * Note: Write is NOT blocked because it's needed to write the plan file
 */
export const PLAN_MODE_BLOCKED_TOOLS = ['Edit', 'Bash'] as const;

export type PlanModeAllowedTool = (typeof PLAN_MODE_ALLOWED_TOOLS)[number];
export type PlanModeBlockedTool = (typeof PLAN_MODE_BLOCKED_TOOLS)[number];

// ============================================================================
// UI Types
// ============================================================================

/**
 * Operating mode (cycle with Shift+Tab: normal → plan → accept → normal)
 * - normal: Default mode, edits require confirmation
 * - plan: Read-only exploration mode, edits blocked
 * - accept: Auto-accept mode, edits approved without confirmation
 */
export type ModeType = 'normal' | 'plan' | 'accept';

/**
 * Plan approval UI state
 */
export interface PlanApprovalState {
  /** Plan content for display */
  planContent: string;

  /** Summary of files to change */
  filesToChange: Array<{
    path: string;
    action: 'create' | 'modify' | 'delete';
  }>;

  /** Requested permissions */
  requestedPermissions: AllowedPrompt[];

  /** Callback when user makes decision */
  onDecision: (option: PlanApprovalOption) => void;
}

// ============================================================================
// Event Types
// ============================================================================

/**
 * Plan mode events for UI updates
 */
export type PlanModeEvent =
  | { type: 'enter'; planFilePath: string }
  | { type: 'phase_change'; phase: PlanPhase }
  | { type: 'exit'; approved: boolean }
  | { type: 'toggle' }; // Shift+Tab toggle
