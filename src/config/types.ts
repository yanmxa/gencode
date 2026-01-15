/**
 * Settings Types - User settings persistence (Claude Code style)
 */

export type ProviderName = 'openai' | 'anthropic' | 'gemini';

/**
 * Settings file structure (~/.gencode/settings.json)
 * Similar to Claude Code's settings.json
 */
export interface Settings {
  model?: string;
  provider?: ProviderName;
  permissions?: {
    allow?: string[];
    deny?: string[];
  };
}

export interface SettingsManagerOptions {
  settingsDir?: string;
}

export const DEFAULT_SETTINGS_DIR = '~/.gencode';
export const SETTINGS_FILE_NAME = 'settings.json';
