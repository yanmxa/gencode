/**
 * Settings Manager - Persists user settings (Claude Code style)
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { Settings, SettingsManagerOptions } from './types.js';
import { DEFAULT_SETTINGS_DIR, SETTINGS_FILE_NAME } from './types.js';

export class SettingsManager {
  private settingsDir: string;
  private settingsPath: string;
  private settings: Settings = {};

  constructor(options: SettingsManagerOptions = {}) {
    const dir = options.settingsDir ?? DEFAULT_SETTINGS_DIR;
    this.settingsDir = dir.replace('~', os.homedir());
    this.settingsPath = path.join(this.settingsDir, SETTINGS_FILE_NAME);
  }

  /**
   * Ensure settings directory exists
   */
  private async ensureDir(): Promise<void> {
    try {
      await fs.mkdir(this.settingsDir, { recursive: true });
    } catch {
      // Directory may already exist
    }
  }

  /**
   * Load settings from disk
   */
  async load(): Promise<Settings> {
    try {
      const content = await fs.readFile(this.settingsPath, 'utf-8');
      this.settings = JSON.parse(content);
      return this.settings;
    } catch {
      // File doesn't exist or is invalid
      this.settings = {};
      return this.settings;
    }
  }

  /**
   * Save settings to disk (merges with existing)
   */
  async save(updates: Partial<Settings>): Promise<void> {
    await this.ensureDir();

    // Merge updates with existing settings
    this.settings = { ...this.settings, ...updates };

    await fs.writeFile(
      this.settingsPath,
      JSON.stringify(this.settings, null, 2),
      'utf-8'
    );
  }

  /**
   * Get current settings
   */
  get(): Settings {
    return { ...this.settings };
  }

  /**
   * Get settings file path
   */
  getPath(): string {
    return this.settingsPath;
  }
}
