/**
 * Permission Manager - Enhanced permission control with pattern matching,
 * prompt-based approvals, persistence, and audit logging.
 *
 * Claude Code compatible design with:
 * - Pattern-based rules (e.g., "Bash(git add:*)")
 * - Prompt-based permissions (e.g., { tool: "Bash", prompt: "run tests" })
 * - Session/project/global scopes
 * - Persistent allowlists
 * - Audit trail
 */

import type {
  PermissionConfig,
  PermissionContext,
  PermissionDecision,
  PermissionMode,
  PermissionRule,
  PromptPermission,
  ApprovalAction,
  ApprovalSuggestion,
  ConfirmCallback,
  SimpleConfirmCallback,
  PermissionSettings,
  AuditDecision,
} from './types.js';
import {
  DEFAULT_PERMISSION_CONFIG,
  DEFAULT_SUGGESTIONS,
} from './types.js';
import { PromptMatcher, matchesPatternString } from './prompt-matcher.js';
import { PermissionPersistence } from './persistence.js';
import { PermissionAudit } from './audit.js';

/**
 * Session approval cache entry
 */
interface SessionApproval {
  tool: string;
  pattern?: string;
  approvedAt: Date;
}

/**
 * Enhanced Permission Manager
 */
export class PermissionManager {
  private config: PermissionConfig;
  private promptMatcher: PromptMatcher;
  private persistence: PermissionPersistence;
  private audit: PermissionAudit;

  // Session-scoped caches
  private sessionApprovals: Map<string, SessionApproval> = new Map();
  private sessionRejections: Set<string> = new Set();

  // Callbacks
  private confirmCallback?: ConfirmCallback;
  private simpleConfirmCallback?: SimpleConfirmCallback;
  private saveRuleCallback?: (tool: string, pattern?: string) => Promise<void>;

  // Current context
  private sessionId?: string;
  private projectPath?: string;

  constructor(options: {
    config?: Partial<PermissionConfig>;
    projectPath?: string;
    enableAudit?: boolean;
  } = {}) {
    this.config = {
      ...DEFAULT_PERMISSION_CONFIG,
      ...options.config,
      rules: [
        ...DEFAULT_PERMISSION_CONFIG.rules,
        ...(options.config?.rules ?? []),
      ],
    };

    this.projectPath = options.projectPath;
    this.promptMatcher = new PromptMatcher();
    this.persistence = new PermissionPersistence(options.projectPath);
    this.audit = new PermissionAudit({
      persistToFile: options.enableAudit ?? false,
    });
  }

  /**
   * Initialize - load persisted rules and settings
   */
  async initialize(settings?: PermissionSettings): Promise<void> {
    // Load persisted rules
    const persistedRules = await this.persistence.getAllRules();
    const runtimeRules = this.persistence.persistedToRuntime(persistedRules);
    this.config.rules.push(...runtimeRules);

    // Parse settings permissions
    if (settings) {
      const settingsRules = this.persistence.parseSettingsPermissions(settings);
      this.config.rules.push(...settingsRules);
    }
  }

  /**
   * Set session ID for tracking
   */
  setSessionId(sessionId: string): void {
    this.sessionId = sessionId;
  }

  /**
   * Set enhanced confirmation callback
   */
  setConfirmCallback(callback: ConfirmCallback): void {
    this.confirmCallback = callback;
  }

  /**
   * Set simple yes/no confirmation callback (backward compatible)
   */
  setSimpleConfirmCallback(callback: SimpleConfirmCallback): void {
    this.simpleConfirmCallback = callback;
  }

  /**
   * Set callback to save permission rules to settings
   * This allows integration with SettingsManager for settings.local.json persistence
   */
  setSaveRuleCallback(callback: (tool: string, pattern?: string) => Promise<void>): void {
    this.saveRuleCallback = callback;
  }

  /**
   * Add prompt-based permissions (Claude Code ExitPlanMode style)
   */
  addAllowedPrompts(prompts: PromptPermission[]): void {
    this.config.allowedPrompts.push(...prompts);
  }

  /**
   * Clear prompt-based permissions
   */
  clearAllowedPrompts(): void {
    this.config.allowedPrompts = [];
  }

  /**
   * Get current allowed prompts
   */
  getAllowedPrompts(): PromptPermission[] {
    return [...this.config.allowedPrompts];
  }

  /**
   * Get all rules
   */
  getRules(): PermissionRule[] {
    return [...this.config.rules];
  }

  /**
   * Check permission (without prompting user)
   *
   * Flow matches Claude Code official design:
   * 1. DENY rules → block immediately
   * 2. ALLOW rules → auto-approve (includes prompt-based & session cache)
   * 3. ASK rules → force prompt
   * 4. Default behavior (read-only → auto, write → prompt)
   */
  async checkPermission(context: PermissionContext): Promise<PermissionDecision> {
    const { tool, input } = context;

    // ========================================================================
    // Step 1: Check DENY rules (highest priority - block immediately)
    // ========================================================================
    const denyRule = this.findMatchingRule(context, 'deny');
    if (denyRule) {
      await this.logAudit(context, 'denied', `Denied by rule: ${this.describeRule(denyRule)}`);
      return {
        allowed: false,
        reason: `Blocked: ${denyRule.description ?? denyRule.pattern ?? tool}`,
        matchedRule: denyRule,
        requiresConfirmation: false,
      };
    }

    // ========================================================================
    // Step 2: Check ALLOW rules (auto-approve if matched)
    // Includes: settings.allow[], prompt-based permissions, session cache
    // ========================================================================

    // 2a. Check settings.allow rules
    const autoRule = this.findMatchingRule(context, 'auto');
    if (autoRule) {
      await this.logAudit(context, 'allowed', `Auto-approved: ${this.describeRule(autoRule)}`);
      return {
        allowed: true,
        reason: 'Auto-approved',
        matchedRule: autoRule,
        requiresConfirmation: false,
      };
    }

    // 2b. Check prompt-based permissions (Plan mode allowedPrompts)
    const promptMatch = this.matchPrompt(tool, input);
    if (promptMatch) {
      await this.logAudit(context, 'allowed', `Prompt match: ${promptMatch.prompt}`);
      return {
        allowed: true,
        reason: `Approved: ${promptMatch.prompt}`,
        matchedRule: promptMatch,
        requiresConfirmation: false,
      };
    }

    // 2c. Check session approval cache
    const cacheKey = this.getCacheKey(tool, input);
    if (this.sessionApprovals.has(cacheKey)) {
      await this.logAudit(context, 'allowed', 'Session cache hit');
      return {
        allowed: true,
        reason: 'Previously approved',
        requiresConfirmation: false,
      };
    }

    // ========================================================================
    // Step 3: Check ASK rules (force confirmation)
    // ========================================================================
    const askRule = this.findMatchingRule(context, 'confirm');
    if (askRule) {
      await this.logAudit(context, 'confirmed', `ASK rule matched: ${this.describeRule(askRule)}`);
      return {
        allowed: false,
        reason: `Requires confirmation: ${askRule.description ?? askRule.pattern ?? tool}`,
        matchedRule: askRule,
        requiresConfirmation: true,
        suggestions: this.getSuggestions(context),
      };
    }

    // ========================================================================
    // Step 4: Default behavior (read-only → auto, write → prompt)
    // ========================================================================

    // Check session rejection cache
    if (this.sessionRejections.has(cacheKey)) {
      await this.logAudit(context, 'denied', 'Session rejection cache');
      return {
        allowed: false,
        reason: 'Previously denied',
        requiresConfirmation: false,
      };
    }

    // Default: requires confirmation for write operations
    return {
      allowed: false,
      reason: 'Requires approval',
      requiresConfirmation: true,
      suggestions: this.getSuggestions(context),
    };
  }

  /**
   * Request permission (prompts user if needed)
   */
  async requestPermission(tool: string, input: unknown): Promise<boolean> {
    const context: PermissionContext = {
      tool,
      input,
      sessionId: this.sessionId,
      projectPath: this.projectPath,
    };

    const decision = await this.checkPermission(context);

    if (decision.allowed) {
      return true;
    }

    if (!decision.requiresConfirmation) {
      return false;
    }

    // Prompt user for confirmation
    const action = await this.promptUser(tool, input, decision.suggestions);

    return this.handleApprovalAction(action, context);
  }

  /**
   * Backward-compatible check method
   */
  async check(tool: string, input: unknown): Promise<boolean> {
    return this.requestPermission(tool, input);
  }

  /**
   * Get permission mode for a tool (for simple queries)
   */
  getModeForTool(tool: string): PermissionMode {
    for (const rule of this.config.rules) {
      if (typeof rule.tool === 'string') {
        if (rule.tool === tool && !rule.pattern) {
          return rule.mode;
        }
      } else if (rule.tool.test(tool) && !rule.pattern) {
        return rule.mode;
      }
    }
    return this.config.defaultMode;
  }

  /**
   * Approve a tool for this session
   */
  approveForSession(tool: string, pattern?: string): void {
    const key = pattern ? `${tool}:${pattern}` : tool;
    this.sessionApprovals.set(key, {
      tool,
      pattern,
      approvedAt: new Date(),
    });
  }

  /**
   * Add a persistent allow rule
   */
  async addAllowRule(
    tool: string,
    pattern?: string,
    scope: 'project' | 'global' = 'global'
  ): Promise<void> {
    await this.persistence.addRule(tool, 'auto', {
      pattern,
      scope,
      description: `User approved: ${tool}${pattern ? `(${pattern})` : ''}`,
    });

    // Also add to runtime config
    this.config.rules.push({
      tool,
      mode: 'auto',
      pattern,
      scope,
    });
  }

  /**
   * Add a persistent deny rule
   */
  async addDenyRule(
    tool: string,
    pattern?: string,
    scope: 'project' | 'global' = 'global'
  ): Promise<void> {
    await this.persistence.addRule(tool, 'deny', {
      pattern,
      scope,
      description: `User blocked: ${tool}${pattern ? `(${pattern})` : ''}`,
    });

    // Also add to runtime config
    this.config.rules.push({
      tool,
      mode: 'deny',
      pattern,
      scope,
    });
  }

  /**
   * Clear session approvals
   */
  clearSessionApprovals(): void {
    this.sessionApprovals.clear();
    this.sessionRejections.clear();
  }

  /**
   * Get audit log
   */
  getAuditLog(count?: number): ReturnType<PermissionAudit['getRecent']> {
    return this.audit.getRecent(count);
  }

  /**
   * Get audit statistics
   */
  getAuditStats(): ReturnType<PermissionAudit['getStats']> {
    return this.audit.getStats();
  }

  /**
   * Get persistence manager (for direct access)
   */
  getPersistence(): PermissionPersistence {
    return this.persistence;
  }

  // ============================================================================
  // Private Methods
  // ============================================================================

  /**
   * Find a matching rule for the given context and mode
   */
  private findMatchingRule(
    context: PermissionContext,
    mode: PermissionMode
  ): PermissionRule | undefined {
    for (const rule of this.config.rules) {
      if (rule.mode !== mode) continue;
      if (!this.matchesTool(rule.tool, context.tool)) continue;

      // If rule has pattern, check it (pass tool name for path matching)
      if (rule.pattern) {
        if (!this.matchesPattern(rule.pattern, context.input, context.tool)) {
          continue;
        }
      }

      // Check expiration
      if (rule.expiresAt && new Date() > rule.expiresAt) {
        continue;
      }

      return rule;
    }

    return undefined;
  }

  /**
   * Check if tool matches rule
   */
  private matchesTool(rulePattern: string | RegExp, tool: string): boolean {
    if (typeof rulePattern === 'string') {
      return rulePattern === tool || rulePattern === '*';
    }
    return rulePattern.test(tool);
  }

  /**
   * Check if input matches pattern
   */
  private matchesPattern(pattern: string | RegExp, input: unknown, tool?: string): boolean {
    if (typeof pattern === 'string') {
      return matchesPatternString(pattern, input, tool);
    }

    const inputStr = typeof input === 'string'
      ? input
      : JSON.stringify(input);

    return pattern.test(inputStr);
  }

  /**
   * Match against prompt-based permissions
   */
  private matchPrompt(tool: string, input: unknown): PromptPermission | null {
    for (const permission of this.config.allowedPrompts) {
      if (permission.tool !== tool) continue;

      if (this.promptMatcher.matches(permission.prompt, input)) {
        return permission;
      }
    }
    return null;
  }

  /**
   * Generate cache key for session approvals
   */
  private getCacheKey(tool: string, input: unknown): string {
    // For Bash, use the command as the key
    if (tool === 'Bash' && input && typeof input === 'object') {
      const cmd = (input as { command?: string }).command;
      if (cmd) {
        // Use first few tokens as pattern
        const tokens = cmd.split(/\s+/).slice(0, 3).join(' ');
        return `${tool}:${tokens}`;
      }
    }

    // For file operations, use the path
    if (['Read', 'Write', 'Edit', 'Glob'].includes(tool)) {
      if (input && typeof input === 'object') {
        const path = (input as { file_path?: string; path?: string }).file_path
          ?? (input as { file_path?: string; path?: string }).path;
        if (path) {
          return `${tool}:${path}`;
        }
      }
    }

    // Default: use full input JSON
    return `${tool}:${JSON.stringify(input)}`;
  }

  /**
   * Get approval suggestions for a context
   */
  private getSuggestions(context: PermissionContext): ApprovalSuggestion[] {
    // Start with default suggestions
    const suggestions = [...DEFAULT_SUGGESTIONS];

    // Customize "Always allow" label based on tool/input
    const alwaysIndex = suggestions.findIndex((s) => s.action === 'allow_always');
    if (alwaysIndex >= 0 && context.tool === 'Bash') {
      const input = context.input as { command?: string };
      if (input?.command) {
        const prefix = input.command.split(/\s+/).slice(0, 2).join(' ');
        suggestions[alwaysIndex] = {
          ...suggestions[alwaysIndex],
          label: `Always allow "${prefix}*"`,
        };
      }
    }

    return suggestions;
  }

  /**
   * Prompt user for approval
   */
  private async promptUser(
    tool: string,
    input: unknown,
    suggestions?: ApprovalSuggestion[]
  ): Promise<ApprovalAction> {
    const effectiveSuggestions = suggestions ?? DEFAULT_SUGGESTIONS;

    // Use enhanced callback if available
    if (this.confirmCallback) {
      return this.confirmCallback(tool, input, effectiveSuggestions);
    }

    // Fall back to simple callback
    if (this.simpleConfirmCallback) {
      const confirmed = await this.simpleConfirmCallback(tool, input);
      return confirmed ? 'allow_session' : 'deny';
    }

    // No callback - auto-approve (non-interactive mode)
    return 'allow_once';
  }

  /**
   * Handle the user's approval action
   */
  private async handleApprovalAction(
    action: ApprovalAction,
    context: PermissionContext
  ): Promise<boolean> {
    const { tool, input } = context;
    const cacheKey = this.getCacheKey(tool, input);

    switch (action) {
      case 'allow_once':
        await this.logAudit(context, 'confirmed', 'User approved once');
        return true;

      case 'allow_session':
        this.sessionApprovals.set(cacheKey, {
          tool,
          approvedAt: new Date(),
        });
        await this.logAudit(context, 'confirmed', 'User approved for session');
        return true;

      case 'allow_always':
        // Extract pattern for persistent rule
        const pattern = this.extractPattern(tool, input);
        // Use saveRuleCallback if available (saves to settings.local.json)
        // Otherwise fallback to addAllowRule (saves to permissions.json)
        if (this.saveRuleCallback) {
          await this.saveRuleCallback(tool, pattern);
        } else {
          await this.addAllowRule(tool, pattern, 'project');
        }
        // Also add to runtime config for immediate effect
        this.config.rules.push({
          tool,
          mode: 'auto',
          pattern,
          scope: 'project',
        });
        await this.logAudit(context, 'confirmed', 'User approved for project');
        return true;

      case 'deny':
        this.sessionRejections.add(cacheKey);
        await this.logAudit(context, 'rejected', 'User denied');
        return false;

      default:
        return false;
    }
  }

  /**
   * Extract a pattern from tool input for persistent rules
   */
  private extractPattern(tool: string, input: unknown): string | undefined {
    if (tool === 'Bash' && input && typeof input === 'object') {
      const cmd = (input as { command?: string }).command;
      if (cmd) {
        // Use first two tokens + wildcard
        const tokens = cmd.split(/\s+/).slice(0, 2);
        return tokens.join(' ') + ':*';
      }
    }

    return undefined;
  }

  /**
   * Describe a rule for logging
   */
  private describeRule(rule: PermissionRule): string {
    const parts = [rule.tool.toString()];
    if (rule.pattern) {
      parts.push(`(${rule.pattern})`);
    }
    if (rule.description) {
      parts.push(`- ${rule.description}`);
    }
    return parts.join('');
  }

  /**
   * Log audit entry
   */
  private async logAudit(
    context: PermissionContext,
    decision: AuditDecision,
    reason: string
  ): Promise<void> {
    await this.audit.log(context.tool, context.input, decision, reason, {
      sessionId: context.sessionId,
    });
  }
}

// Re-export types and utilities
export type { ConfirmCallback, SimpleConfirmCallback };
