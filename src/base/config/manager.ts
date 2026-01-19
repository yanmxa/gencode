/**
 * Configuration Manager - Unified multi-level configuration (Claude Code compatible)
 *
 * Configuration hierarchy (merged in order, later overrides earlier):
 * 1. User: ~/.claude/ + ~/.gencode/ (gencode wins within level)
 * 2. Extra: GENCODE_CONFIG_DIRS directories
 * 3. Project: .claude/ + .gencode/ (gencode wins within level)
 * 4. Local: *.local.* files (gencode wins within level)
 * 5. CLI: Command line arguments
 * 6. Managed: System-wide enforced settings (cannot be overridden)
 *
 * Within each level, both .claude and .gencode directories are loaded and merged,
 * with .gencode taking higher priority.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import type { Settings, MergedConfig, ConfigSource, SettingsManagerOptions } from './types.js';
import { SETTINGS_FILE_NAME, SETTINGS_LOCAL_FILE_NAME } from './types.js';
import { loadAllSources, getExistingConfigFiles } from './loader.js';
import { mergeAllSources, mergeWithCliArgs, deepMerge, createMergeSummary } from './merger.js';
import { findProjectRoot, getPrimarySettingsDir, getSettingsFilePath } from './levels.js';

/**
 * Configuration Manager
 *
 * Manages multi-level configuration loading, merging, and persistence.
 * Compatible with both GenCode (.gencode/) and Claude Code (.claude/) directories.
 */
export class ConfigManager {
  private cwd: string;
  private projectRoot: string | null = null;
  private mergedConfig: MergedConfig | null = null;
  private cliArgs: Partial<Settings> = {};

  constructor(options: { cwd?: string } = {}) {
    this.cwd = options.cwd ?? process.cwd();
  }

  /**
   * Load and merge all configuration sources
   */
  async load(): Promise<MergedConfig> {
    // Find project root
    this.projectRoot = await findProjectRoot(this.cwd);

    // Load all sources
    const sources = await loadAllSources(this.cwd);

    // Merge all sources
    let merged = mergeAllSources(sources);

    // Apply CLI arguments if any
    if (Object.keys(this.cliArgs).length > 0) {
      merged = mergeWithCliArgs(merged, this.cliArgs);
    }

    this.mergedConfig = merged;
    return merged;
  }

  /**
   * Set CLI argument overrides
   */
  setCliArgs(args: Partial<Settings>): void {
    this.cliArgs = args;
  }

  /**
   * Get the current merged settings
   */
  get(): Settings {
    if (!this.mergedConfig) {
      return {};
    }
    return { ...this.mergedConfig.settings };
  }

  /**
   * Get the full merged config with sources info
   */
  getMergedConfig(): MergedConfig | null {
    return this.mergedConfig;
  }

  /**
   * Get all loaded sources
   */
  getSources(): ConfigSource[] {
    return this.mergedConfig?.sources ?? [];
  }

  /**
   * Get managed deny rules
   */
  getManagedDeny(): string[] {
    return this.mergedConfig?.managedDeny ?? [];
  }

  /**
   * Save settings to a specific level
   */
  async saveToLevel(
    updates: Partial<Settings>,
    level: 'user' | 'project' | 'local'
  ): Promise<void> {
    if (!this.projectRoot) {
      this.projectRoot = await findProjectRoot(this.cwd);
    }

    const dir = getPrimarySettingsDir(level, this.projectRoot);
    const fileName = level === 'local' ? SETTINGS_LOCAL_FILE_NAME : SETTINGS_FILE_NAME;
    const filePath = path.join(dir, fileName);

    // Ensure directory exists
    await fs.mkdir(dir, { recursive: true });

    // Load existing settings for this level
    let existing: Settings = {};
    try {
      const content = await fs.readFile(filePath, 'utf-8');
      existing = JSON.parse(content);
    } catch {
      // File doesn't exist, start fresh
    }

    // Merge updates into existing
    const merged = deepMerge(existing, updates);

    // Write back
    await fs.writeFile(filePath, JSON.stringify(merged, null, 2), 'utf-8');

    // Reload configuration
    await this.load();
  }

  /**
   * Save to global user settings (default)
   */
  async save(updates: Partial<Settings>): Promise<void> {
    await this.saveToLevel(updates, 'user');
  }

  /**
   * Add a permission rule
   */
  async addPermissionRule(
    pattern: string,
    type: 'allow' | 'deny' | 'ask',
    level: 'user' | 'project' | 'local' = 'project'
  ): Promise<void> {
    const updates: Settings = {
      permissions: {
        [type]: [pattern],
      },
    };
    await this.saveToLevel(updates, level);
  }

  /**
   * Get existing config files
   */
  async getExistingFiles(): Promise<string[]> {
    return getExistingConfigFiles(this.cwd);
  }

  /**
   * Get debug summary of configuration
   */
  getDebugSummary(): string {
    if (!this.mergedConfig) {
      return 'Configuration not loaded';
    }
    return createMergeSummary(this.mergedConfig);
  }

  /**
   * Get the project root directory
   */
  getProjectRoot(): string {
    return this.projectRoot ?? this.cwd;
  }

  /**
   * Get the current working directory
   */
  getCwd(): string {
    return this.cwd;
  }

  /**
   * Get the primary settings file path for a level
   */
  getSettingsPath(level: 'user' | 'project' | 'local' = 'user'): string {
    const projectRoot = this.projectRoot ?? this.cwd;
    return getSettingsFilePath(level, projectRoot);
  }

  /**
   * Get effective permissions (merged from all sources)
   */
  getEffectivePermissions(): {
    allow: string[];
    ask: string[];
    deny: string[];
    managedDeny: string[];
  } {
    const settings = this.get();
    const permissions = settings.permissions ?? {};

    return {
      allow: permissions.allow ?? [],
      ask: permissions.ask ?? [],
      deny: permissions.deny ?? [],
      managedDeny: this.getManagedDeny(),
    };
  }

  /**
   * Check if a permission pattern is allowed
   */
  isAllowed(pattern: string): boolean {
    const { allow, deny, managedDeny } = this.getEffectivePermissions();

    // Managed deny always wins
    if (managedDeny.some((p) => this.matchPattern(pattern, p))) {
      return false;
    }

    // Check deny list
    if (deny.some((p) => this.matchPattern(pattern, p))) {
      return false;
    }

    // Check allow list
    return allow.some((p) => this.matchPattern(pattern, p));
  }

  /**
   * Check if a permission pattern requires asking
   */
  shouldAsk(pattern: string): boolean {
    const { ask, allow, deny, managedDeny } = this.getEffectivePermissions();

    // Managed deny always wins
    if (managedDeny.some((p) => this.matchPattern(pattern, p))) {
      return false; // Don't ask, just deny
    }

    // If explicitly allowed, don't ask
    if (allow.some((p) => this.matchPattern(pattern, p))) {
      return false;
    }

    // If explicitly denied, don't ask
    if (deny.some((p) => this.matchPattern(pattern, p))) {
      return false;
    }

    // Check ask list
    if (ask.some((p) => this.matchPattern(pattern, p))) {
      return true;
    }

    // Default: ask for unknown patterns
    return true;
  }

  /**
   * Simple pattern matching (supports * and :* wildcards)
   */
  private matchPattern(value: string, pattern: string): boolean {
    // Exact match
    if (value === pattern) return true;

    // Handle :* suffix (e.g., "Bash(git:*)" matches "Bash(git:status)")
    if (pattern.endsWith(':*)')) {
      const prefix = pattern.slice(0, -3); // Remove ":*)"
      const valuePrefix = value.slice(0, value.lastIndexOf(':'));
      return valuePrefix.startsWith(prefix);
    }

    // Handle * wildcards
    if (pattern.includes('*')) {
      const regex = new RegExp('^' + pattern.replace(/\*/g, '.*') + '$');
      return regex.test(value);
    }

    return false;
  }
}

// =============================================================================
// Legacy SettingsManager (backward compatibility)
// =============================================================================

/**
 * Legacy SettingsManager for backward compatibility
 *
 * @deprecated Use ConfigManager instead
 */
export class SettingsManager {
  private configManager: ConfigManager;
  private globalDir: string;
  private projectDir: string;

  constructor(options: SettingsManagerOptions & { cwd?: string } = {}) {
    this.configManager = new ConfigManager({ cwd: options.cwd });

    // For legacy compatibility
    const cwd = options.cwd ?? process.cwd();
    this.globalDir = options.settingsDir ?? getPrimarySettingsDir('user', cwd);
    this.projectDir = getPrimarySettingsDir('project', cwd);
  }

  async load(): Promise<Settings> {
    const merged = await this.configManager.load();
    return merged.settings;
  }

  async save(updates: Partial<Settings>): Promise<void> {
    await this.configManager.save(updates);
  }

  async saveToLevel(
    updates: Partial<Settings>,
    level: 'global' | 'project' | 'local'
  ): Promise<void> {
    const mappedLevel = level === 'global' ? 'user' : level;
    await this.configManager.saveToLevel(updates, mappedLevel);
  }

  async addPermissionRule(
    pattern: string,
    type: 'allow' | 'deny',
    level: 'global' | 'project' | 'local' = 'project'
  ): Promise<void> {
    const mappedLevel = level === 'global' ? 'user' : level;
    await this.configManager.addPermissionRule(pattern, type, mappedLevel);
  }

  get(): Settings {
    return this.configManager.get();
  }

  getPath(): string {
    return this.configManager.getSettingsPath('user');
  }

  getProjectPath(): string {
    return this.configManager.getSettingsPath('project');
  }

  getLocalPath(): string {
    return this.configManager.getSettingsPath('local');
  }

  getCwd(): string {
    return this.configManager.getCwd();
  }

  getProjectDir(): string {
    return this.projectDir;
  }
}
