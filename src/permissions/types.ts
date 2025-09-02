/**
 * Permission System Types
 */

export type PermissionMode = 'auto' | 'confirm' | 'deny';

export interface PermissionRule {
  tool: string | RegExp;
  mode: PermissionMode;
}

export interface PermissionConfig {
  defaultMode: PermissionMode;
  rules: PermissionRule[];
}

export const DEFAULT_PERMISSION_CONFIG: PermissionConfig = {
  defaultMode: 'confirm',
  rules: [
    // Read-only tools are auto-approved
    { tool: 'Read', mode: 'auto' },
    { tool: 'Glob', mode: 'auto' },
    { tool: 'Grep', mode: 'auto' },
    // Write operations require confirmation
    { tool: 'Write', mode: 'confirm' },
    { tool: 'Edit', mode: 'confirm' },
    { tool: 'Bash', mode: 'confirm' },
  ],
};
