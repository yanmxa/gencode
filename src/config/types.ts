/**
 * Settings Types - User settings persistence (Claude Code style)
 */

export type ProviderName = 'openai' | 'anthropic' | 'gemini' | 'vertex-ai';

/**
 * Settings file structure (~/.gencode/settings.json)
 * Similar to Claude Code's settings.json
 */
export interface Settings {
  model?: string;
  provider?: ProviderName;
  permissions?: {
    allow?: string[];
    ask?: string[];    // Claude Code compatible: require confirmation
    deny?: string[];
  };
}

export interface SettingsManagerOptions {
  settingsDir?: string;
}

// Primary config directories (Claude Code compatible)
export const DEFAULT_SETTINGS_DIR = '~/.claude';
export const PROJECT_SETTINGS_DIR = '.claude';

// Fallback to GenCode directories
export const FALLBACK_SETTINGS_DIR = '~/.gencode';
export const FALLBACK_PROJECT_DIR = '.gencode';

export const SETTINGS_FILE_NAME = 'settings.json';
export const SETTINGS_LOCAL_FILE_NAME = 'settings.local.json';
export const PROVIDERS_FILE_NAME = 'providers.json';

/**
 * Provider connection info
 */
export interface ProviderConnection {
  method: 'api_key' | 'vertex' | 'oauth';
  connectedAt: string;
}

/**
 * Cached model info
 */
export interface CachedModel {
  id: string;
  name: string;
  description?: string;
}

/**
 * Cached models for a provider
 */
export interface ProviderModels {
  cachedAt: string;
  list: CachedModel[];
}

/**
 * Providers config file structure (~/.gencode/providers.json)
 */
export interface ProvidersConfig {
  connections: Record<string, ProviderConnection>;
  models: Record<string, ProviderModels>;
}
