/**
 * Settings Manager - Multi-level configuration loading (Claude Code style)
 *
 * Configuration hierarchy (later overrides earlier):
 * 1. ~/.gencode/settings.json (global) - fallback: ~/.claude/settings.json
 * 2. .gencode/settings.json (project - tracked in git) - fallback: .claude/settings.json
 * 3. .gencode/settings.local.json (project local - gitignored) - fallback: .claude/settings.local.json
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { Settings, SettingsManagerOptions } from './types.js';
import {
  DEFAULT_SETTINGS_DIR,
  PROJECT_SETTINGS_DIR,
  FALLBACK_SETTINGS_DIR,
  FALLBACK_PROJECT_DIR,
  SETTINGS_FILE_NAME,
  SETTINGS_LOCAL_FILE_NAME,
} from './types.js';

/**
 * Deep merge two objects
 */
function deepMerge<T extends object>(base: T, override: Partial<T>): T {
  const result = { ...base };

  for (const key in override) {
    const baseValue = result[key];
    const overrideValue = override[key];

    if (
      baseValue &&
      overrideValue &&
      typeof baseValue === 'object' &&
      typeof overrideValue === 'object' &&
      !Array.isArray(baseValue) &&
      !Array.isArray(overrideValue)
    ) {
      // Recursively merge objects
      result[key] = deepMerge(
        baseValue as Record<string, unknown>,
        overrideValue as Record<string, unknown>
      ) as T[Extract<keyof T, string>];
    } else if (Array.isArray(baseValue) && Array.isArray(overrideValue)) {
      // Concatenate arrays (permissions.allow, permissions.deny)
      result[key] = [...baseValue, ...overrideValue] as T[Extract<keyof T, string>];
    } else if (overrideValue !== undefined) {
      result[key] = overrideValue as T[Extract<keyof T, string>];
    }
  }

  return result;
}

/**
 * Check if a directory exists synchronously
 */
function directoryExistsSync(dirPath: string): boolean {
  try {
    const resolvedPath = dirPath.replace('~', os.homedir());
    const stats = require('fs').statSync(resolvedPath);
    return stats.isDirectory();
  } catch {
    return false;
  }
}

export class SettingsManager {
  private globalDir: string;
  private projectDir: string;
  private cwd: string;
  private settings: Settings = {};

  constructor(options: SettingsManagerOptions & { cwd?: string } = {}) {
    this.cwd = options.cwd ?? process.cwd();

    // Determine global directory with fallback
    if (options.settingsDir) {
      this.globalDir = options.settingsDir.replace('~', os.homedir());
    } else {
      // Check if ~/.gencode exists, otherwise fallback to ~/.claude
      const primaryGlobal = DEFAULT_SETTINGS_DIR.replace('~', os.homedir());
      const fallbackGlobal = FALLBACK_SETTINGS_DIR.replace('~', os.homedir());
      this.globalDir = directoryExistsSync(primaryGlobal) ? primaryGlobal :
                       directoryExistsSync(fallbackGlobal) ? fallbackGlobal : primaryGlobal;
    }

    // Determine project directory with fallback
    const primaryProject = path.join(this.cwd, PROJECT_SETTINGS_DIR);
    const fallbackProject = path.join(this.cwd, FALLBACK_PROJECT_DIR);
    this.projectDir = directoryExistsSync(primaryProject) ? primaryProject :
                      directoryExistsSync(fallbackProject) ? fallbackProject : primaryProject;
  }

  /**
   * Get all configuration file paths in load order
   */
  private getConfigPaths(): { path: string; level: string }[] {
    return [
      { path: path.join(this.globalDir, SETTINGS_FILE_NAME), level: 'global' },
      { path: path.join(this.projectDir, SETTINGS_FILE_NAME), level: 'project' },
      { path: path.join(this.projectDir, SETTINGS_LOCAL_FILE_NAME), level: 'local' },
    ];
  }

  /**
   * Load a single settings file
   */
  private async loadFile(filePath: string): Promise<Settings | null> {
    try {
      const content = await fs.readFile(filePath, 'utf-8');
      return JSON.parse(content);
    } catch {
      return null;
    }
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
   * Load settings from all levels and merge
   */
  async load(): Promise<Settings> {
    let merged: Settings = {};

    for (const config of this.getConfigPaths()) {
      const settings = await this.loadFile(config.path);
      if (settings) {
        merged = deepMerge(merged, settings);
      }
    }

    this.settings = merged;
    return this.settings;
  }

  /**
   * Save settings to global config
   */
  async save(updates: Partial<Settings>): Promise<void> {
    await this.saveToLevel(updates, 'global');
  }

  /**
   * Save settings to a specific level
   */
  async saveToLevel(
    updates: Partial<Settings>,
    level: 'global' | 'project' | 'local'
  ): Promise<void> {
    let dir: string;
    let fileName: string;

    switch (level) {
      case 'global':
        dir = this.globalDir;
        fileName = SETTINGS_FILE_NAME;
        break;
      case 'project':
        dir = this.projectDir;
        fileName = SETTINGS_FILE_NAME;
        break;
      case 'local':
        dir = this.projectDir;
        fileName = SETTINGS_LOCAL_FILE_NAME;
        break;
    }

    await this.ensureDir(dir);
    const filePath = path.join(dir, fileName);

    // Load existing settings for this level
    const existing = (await this.loadFile(filePath)) ?? {};

    // Merge updates
    const merged = deepMerge(existing, updates);

    await fs.writeFile(filePath, JSON.stringify(merged, null, 2), 'utf-8');

    // Reload all settings
    await this.load();
  }

  /**
   * Add a permission rule
   */
  async addPermissionRule(
    pattern: string,
    type: 'allow' | 'deny',
    level: 'global' | 'project' | 'local' = 'project'
  ): Promise<void> {
    const updates: Settings = {
      permissions: {
        [type]: [pattern],
      },
    };
    await this.saveToLevel(updates, level);
  }

  /**
   * Get current settings
   */
  get(): Settings {
    return { ...this.settings };
  }

  /**
   * Get global settings file path
   */
  getPath(): string {
    return path.join(this.globalDir, SETTINGS_FILE_NAME);
  }

  /**
   * Get project settings file path
   */
  getProjectPath(): string {
    return path.join(this.projectDir, SETTINGS_FILE_NAME);
  }

  /**
   * Get project local settings file path
   */
  getLocalPath(): string {
    return path.join(this.projectDir, SETTINGS_LOCAL_FILE_NAME);
  }

  /**
   * Get current working directory
   */
  getCwd(): string {
    return this.cwd;
  }

  /**
   * Get project settings directory
   */
  getProjectDir(): string {
    return this.projectDir;
  }
}
