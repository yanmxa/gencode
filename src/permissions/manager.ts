/**
 * Permission Manager - Controls tool execution permissions
 */

import type { PermissionConfig, PermissionMode } from './types.js';
import { DEFAULT_PERMISSION_CONFIG } from './types.js';

export type ConfirmCallback = (
  tool: string,
  input: unknown
) => Promise<boolean>;

export class PermissionManager {
  private config: PermissionConfig;
  private confirmCallback?: ConfirmCallback;
  private approvedTools: Set<string> = new Set();

  constructor(config?: Partial<PermissionConfig>) {
    this.config = {
      ...DEFAULT_PERMISSION_CONFIG,
      ...config,
    };
  }

  /**
   * Set the confirmation callback
   */
  setConfirmCallback(callback: ConfirmCallback): void {
    this.confirmCallback = callback;
  }

  /**
   * Get the permission mode for a tool
   */
  getModeForTool(tool: string): PermissionMode {
    for (const rule of this.config.rules) {
      if (typeof rule.tool === 'string') {
        if (rule.tool === tool) {
          return rule.mode;
        }
      } else if (rule.tool.test(tool)) {
        return rule.mode;
      }
    }
    return this.config.defaultMode;
  }

  /**
   * Check if a tool can be executed
   */
  async checkPermission(tool: string, input: unknown): Promise<boolean> {
    const mode = this.getModeForTool(tool);

    switch (mode) {
      case 'auto':
        return true;

      case 'deny':
        return false;

      case 'confirm':
        // Check if already approved this session
        const key = `${tool}:${JSON.stringify(input)}`;
        if (this.approvedTools.has(key)) {
          return true;
        }

        if (!this.confirmCallback) {
          // No callback, auto-approve in non-interactive mode
          return true;
        }

        const approved = await this.confirmCallback(tool, input);
        if (approved) {
          this.approvedTools.add(key);
        }
        return approved;

      default:
        return false;
    }
  }

  /**
   * Approve a tool for this session
   */
  approveToolForSession(tool: string): void {
    this.approvedTools.add(tool);
  }

  /**
   * Clear all session approvals
   */
  clearApprovals(): void {
    this.approvedTools.clear();
  }
}
