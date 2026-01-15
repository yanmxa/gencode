/**
 * Permission Persistence - Store and load permission rules
 *
 * Handles persistent storage of permission rules at:
 * - Global: ~/.gencode/permissions.json
 * - Project: .gencode/permissions.json
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type {
  PersistedPermissions,
  PersistedRule,
  PermissionRule,
  PermissionScope,
  PermissionMode,
  PermissionSettings,
} from './types.js';
import { parsePatternString } from './prompt-matcher.js';

const PERMISSIONS_VERSION = 1;
const PERMISSIONS_FILE = 'permissions.json';
const GLOBAL_DIR = path.join(os.homedir(), '.gencode');

/**
 * Permission Persistence Manager
 */
export class PermissionPersistence {
  private globalDir: string;
  private projectDir: string | null;

  constructor(projectPath?: string) {
    this.globalDir = GLOBAL_DIR;
    this.projectDir = projectPath ? path.join(projectPath, '.gencode') : null;
  }

  /**
   * Generate unique rule ID
   */
  private generateId(): string {
    return `rule_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`;
  }

  /**
   * Ensure directory exists
   */
  private async ensureDir(dir: string): Promise<void> {
    try {
      await fs.mkdir(dir, { recursive: true });
    } catch {
      // Directory may already exist
    }
  }

  /**
   * Get path for scope
   */
  private getPath(scope: PermissionScope): string {
    switch (scope) {
      case 'global':
        return path.join(this.globalDir, PERMISSIONS_FILE);
      case 'project':
        if (!this.projectDir) {
          throw new Error('Project path not set for project-scoped permissions');
        }
        return path.join(this.projectDir, PERMISSIONS_FILE);
      default:
        // Session scope is not persisted
        throw new Error('Session-scoped permissions are not persisted');
    }
  }

  /**
   * Load permissions from file
   */
  async load(scope: PermissionScope = 'global'): Promise<PersistedPermissions> {
    try {
      const filePath = this.getPath(scope);
      const content = await fs.readFile(filePath, 'utf-8');
      const data = JSON.parse(content) as PersistedPermissions;

      // Migration: add version if missing
      if (!data.version) {
        data.version = PERMISSIONS_VERSION;
      }

      return data;
    } catch {
      // Return empty permissions
      return { version: PERMISSIONS_VERSION, rules: [] };
    }
  }

  /**
   * Save permissions to file
   */
  async save(permissions: PersistedPermissions, scope: PermissionScope = 'global'): Promise<void> {
    const dir = scope === 'global' ? this.globalDir : this.projectDir;
    if (!dir) {
      throw new Error('Project path not set for project-scoped permissions');
    }

    await this.ensureDir(dir);
    const filePath = this.getPath(scope);

    await fs.writeFile(
      filePath,
      JSON.stringify(permissions, null, 2),
      'utf-8'
    );
  }

  /**
   * Add a new permission rule
   */
  async addRule(
    tool: string,
    mode: PermissionMode,
    options: {
      pattern?: string;
      scope?: PermissionScope;
      description?: string;
    } = {}
  ): Promise<PersistedRule> {
    const scope = options.scope ?? 'global';

    // Session rules are not persisted
    if (scope === 'session') {
      throw new Error('Session-scoped rules cannot be persisted');
    }

    const permissions = await this.load(scope);

    const rule: PersistedRule = {
      id: this.generateId(),
      tool,
      pattern: options.pattern,
      mode,
      scope,
      createdAt: new Date().toISOString(),
      description: options.description,
    };

    permissions.rules.push(rule);
    await this.save(permissions, scope);

    return rule;
  }

  /**
   * Remove a permission rule by ID
   */
  async removeRule(ruleId: string, scope: PermissionScope = 'global'): Promise<boolean> {
    if (scope === 'session') {
      return false;
    }

    const permissions = await this.load(scope);
    const initialLength = permissions.rules.length;
    permissions.rules = permissions.rules.filter((r) => r.id !== ruleId);

    if (permissions.rules.length < initialLength) {
      await this.save(permissions, scope);
      return true;
    }

    return false;
  }

  /**
   * Get all persisted rules for a scope
   */
  async getRules(scope: PermissionScope = 'global'): Promise<PersistedRule[]> {
    if (scope === 'session') {
      return [];
    }

    const permissions = await this.load(scope);
    return permissions.rules;
  }

  /**
   * Get all rules (global + project)
   */
  async getAllRules(): Promise<PersistedRule[]> {
    const globalRules = await this.getRules('global');
    let projectRules: PersistedRule[] = [];

    if (this.projectDir) {
      try {
        projectRules = await this.getRules('project');
      } catch {
        // Project permissions file may not exist
      }
    }

    return [...projectRules, ...globalRules];
  }

  /**
   * Convert persisted rules to runtime permission rules
   */
  persistedToRuntime(persisted: PersistedRule[]): PermissionRule[] {
    return persisted.map((p) => ({
      tool: p.tool,
      mode: p.mode,
      pattern: p.pattern,
      scope: p.scope,
      description: p.description,
    }));
  }

  /**
   * Parse settings-style permissions (from settings.json)
   * Claude Code format: { allow: ["Bash(git add:*)"], ask: ["Bash(npm run:*)"], deny: ["Bash(rm -rf:*)"] }
   */
  parseSettingsPermissions(settings: PermissionSettings): PermissionRule[] {
    const rules: PermissionRule[] = [];

    // Parse allow rules (auto-approve)
    if (settings.allow) {
      for (const pattern of settings.allow) {
        const parsed = parsePatternString(pattern);
        if (parsed) {
          rules.push({
            tool: parsed.tool,
            mode: 'auto',
            pattern: parsed.pattern,
            scope: 'global',
            description: `Settings: ${pattern}`,
          });
        }
      }
    }

    // Parse ask rules (require confirmation)
    if (settings.ask) {
      for (const pattern of settings.ask) {
        const parsed = parsePatternString(pattern);
        if (parsed) {
          rules.push({
            tool: parsed.tool,
            mode: 'confirm',
            pattern: parsed.pattern,
            scope: 'global',
            description: `Settings: ${pattern}`,
          });
        }
      }
    }

    // Parse deny rules (block)
    if (settings.deny) {
      for (const pattern of settings.deny) {
        const parsed = parsePatternString(pattern);
        if (parsed) {
          rules.push({
            tool: parsed.tool,
            mode: 'deny',
            pattern: parsed.pattern,
            scope: 'global',
            description: `Settings: ${pattern}`,
          });
        }
      }
    }

    return rules;
  }

  /**
   * Clear all rules for a scope
   */
  async clearRules(scope: PermissionScope = 'global'): Promise<void> {
    if (scope === 'session') {
      return;
    }

    const permissions = await this.load(scope);
    permissions.rules = [];
    await this.save(permissions, scope);
  }

  /**
   * Check if permissions file exists
   */
  async exists(scope: PermissionScope = 'global'): Promise<boolean> {
    if (scope === 'session') {
      return false;
    }

    try {
      const filePath = this.getPath(scope);
      await fs.access(filePath);
      return true;
    } catch {
      return false;
    }
  }
}
