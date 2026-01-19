/**
 * Permission System Types
 * Enhanced permission management with pattern matching, prompt-based approvals,
 * persistence, and audit logging - Claude Code compatible design.
 */

// ============================================================================
// Core Types
// ============================================================================

export type PermissionMode = 'auto' | 'confirm' | 'deny';
export type PermissionScope = 'session' | 'project' | 'global';
export type ApprovalAction = 'allow_once' | 'allow_session' | 'allow_always' | 'deny';

// ============================================================================
// Permission Rules
// ============================================================================

/**
 * Permission rule for matching tool operations
 * Claude Code style: "Bash(git add:*)"
 */
export interface PermissionRule {
  /** Tool name or regex pattern */
  tool: string | RegExp;
  /** Permission mode for matched operations */
  mode: PermissionMode;
  /** Optional input pattern for matching (glob-style, e.g., "git add:*") */
  pattern?: string | RegExp;
  /** Semantic description for prompt-based matching */
  prompt?: string;
  /** Rule scope - where this rule applies */
  scope?: PermissionScope;
  /** Expiration timestamp for time-limited rules */
  expiresAt?: Date;
  /** Human-readable description of the rule */
  description?: string;
}

/**
 * Prompt-based permission (Claude Code ExitPlanMode style)
 * { tool: "Bash", prompt: "run tests" }
 */
export interface PromptPermission {
  /** Tool this permission applies to */
  tool: string;
  /** Semantic description of allowed action */
  prompt: string;
}

// ============================================================================
// Permission Config
// ============================================================================

/**
 * Complete permission configuration
 */
export interface PermissionConfig {
  /** Default mode for unmatched tools */
  defaultMode: PermissionMode;
  /** Built-in and user-defined rules */
  rules: PermissionRule[];
  /** Prompt-based permissions (from plan approval) */
  allowedPrompts: PromptPermission[];
}

/**
 * Serializable permission settings (for settings.json)
 * Claude Code format: { allow: ["Bash(git add:*)"], ask: ["Bash(npm run:*)"], deny: ["Bash(rm -rf:*)"] }
 */
export interface PermissionSettings {
  /** Allow patterns - auto-approve matching operations */
  allow?: string[];
  /** Ask patterns - require confirmation for matching operations */
  ask?: string[];
  /** Deny patterns - block matching operations */
  deny?: string[];
}

// ============================================================================
// Permission Context
// ============================================================================

/**
 * Context for permission check
 */
export interface PermissionContext {
  /** Tool being executed */
  tool: string;
  /** Tool input/parameters */
  input: unknown;
  /** Current session ID */
  sessionId?: string;
  /** Project path */
  projectPath?: string;
}

// ============================================================================
// Permission Decision
// ============================================================================

/**
 * Result of a permission check
 */
export interface PermissionDecision {
  /** Whether operation is allowed */
  allowed: boolean;
  /** Reason for the decision */
  reason: string;
  /** Rule that matched (if any) */
  matchedRule?: PermissionRule | PromptPermission;
  /** Whether user confirmation is required */
  requiresConfirmation: boolean;
  /** Suggested approval options */
  suggestions?: ApprovalSuggestion[];
}

/**
 * Approval option presented to user
 */
export interface ApprovalSuggestion {
  /** Action identifier */
  action: ApprovalAction;
  /** User-facing label */
  label: string;
  /** Description of what this option does */
  description?: string;
  /** Keyboard shortcut (e.g., "1", "y") */
  shortcut?: string;
}

// ============================================================================
// Audit Types
// ============================================================================

export type AuditDecision = 'allowed' | 'denied' | 'confirmed' | 'rejected';

/**
 * Audit log entry for permission decisions
 */
export interface PermissionAuditEntry {
  /** When the decision was made */
  timestamp: Date;
  /** Tool that was checked */
  tool: string;
  /** Summarized input (not full payload for privacy) */
  inputSummary: string;
  /** Final decision */
  decision: AuditDecision;
  /** Reason for decision */
  reason: string;
  /** Rule that matched (if any) */
  matchedRule?: string;
  /** Session ID */
  sessionId?: string;
}

// ============================================================================
// Persistence Types
// ============================================================================

/**
 * Persisted rule for permanent storage
 */
export interface PersistedRule {
  /** Unique rule ID */
  id: string;
  /** Tool pattern string */
  tool: string;
  /** Input pattern string */
  pattern?: string;
  /** Permission mode */
  mode: PermissionMode;
  /** Rule scope */
  scope: PermissionScope;
  /** When rule was created */
  createdAt: string;
  /** Description */
  description?: string;
}

/**
 * Persisted permissions file structure
 */
export interface PersistedPermissions {
  /** Version for migration */
  version: number;
  /** Global rules */
  rules: PersistedRule[];
}

// ============================================================================
// Callback Types
// ============================================================================

/**
 * Confirmation callback with approval options
 */
export type ConfirmCallback = (
  tool: string,
  input: unknown,
  suggestions: ApprovalSuggestion[]
) => Promise<ApprovalAction>;

/**
 * Simple yes/no confirmation callback (for backward compatibility)
 */
export type SimpleConfirmCallback = (tool: string, input: unknown) => Promise<boolean>;

// ============================================================================
// Default Configuration
// ============================================================================

/**
 * Default permission configuration (Claude Code style)
 * - Read-only tools auto-approved
 * - Write operations require confirmation
 * - Users can configure additional rules in settings.json
 */
export const DEFAULT_PERMISSION_CONFIG: PermissionConfig = {
  defaultMode: 'confirm',
  rules: [
    // Read-only tools - auto-approve (Claude Code behavior)
    { tool: 'Read', mode: 'auto', description: 'File reading' },
    { tool: 'Glob', mode: 'auto', description: 'Pattern matching' },
    { tool: 'Grep', mode: 'auto', description: 'Content search' },
    { tool: 'LSP', mode: 'auto', description: 'Language server' },
    // Internal state management - auto-approve (no side effects)
    { tool: 'TodoWrite', mode: 'auto', description: 'Task tracking' },
    // User interaction - auto-approve (asking user questions, not dangerous)
    { tool: 'AskUserQuestion', mode: 'auto', description: 'User questioning' },
  ],
  allowedPrompts: [],
};

/**
 * Default approval suggestions (Claude Code style)
 * The second option dynamically shows the path context
 */
export const DEFAULT_SUGGESTIONS: ApprovalSuggestion[] = [
  {
    action: 'allow_once',
    label: 'Yes',
    description: 'Allow this operation',
    shortcut: '1',
  },
  {
    action: 'allow_always',
    label: "Yes, and don't ask again",
    description: 'Add to project allowlist',
    shortcut: '2',
  },
  {
    action: 'deny',
    label: 'No',
    description: 'Block this operation',
    shortcut: '3',
  },
];
