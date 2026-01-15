/**
 * Permission System
 *
 * Enhanced permission management with:
 * - Pattern-based rules (Claude Code style: "Bash(git add:*)")
 * - Prompt-based permissions (ExitPlanMode style)
 * - Session/project/global scopes
 * - Persistent allowlists
 * - Audit trail
 */

// Types
export * from './types.js';

// Core Manager
export { PermissionManager } from './manager.js';
export type { ConfirmCallback, SimpleConfirmCallback } from './manager.js';

// Prompt Matching
export { PromptMatcher, parsePatternString, matchesPatternString } from './prompt-matcher.js';

// Persistence
export { PermissionPersistence } from './persistence.js';

// Audit
export { PermissionAudit } from './audit.js';
