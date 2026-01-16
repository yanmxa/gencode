/**
 * Configuration Types - Multi-level configuration system (Claude Code compatible)
 *
 * Configuration hierarchy (priority from low to high):
 * 1. User Level: ~/.gencode/ + ~/.claude/ (merged, gencode wins)
 * 2. Extra Dirs: GENCODE_CONFIG_DIRS environment variable
 * 3. Project Level: .gencode/ + .claude/ (merged, gencode wins)
 * 4. Local Level: .gencode/*.local.* + .claude/*.local.* (merged, gencode wins)
 * 5. CLI Arguments: Command line overrides
 * 6. Managed Level: System-wide enforced settings (cannot be overridden)
 */

import * as os from 'os';
import * as path from 'path';

// =============================================================================
// Provider Types
// =============================================================================

export type ProviderName = 'openai' | 'anthropic' | 'gemini' | 'vertex-ai';

// =============================================================================
// Settings Types
// =============================================================================

/**
 * Permission rules for tools
 */
export interface PermissionRules {
  allow?: string[];
  ask?: string[];
  deny?: string[];
}

/**
 * Settings file structure
 * Compatible with Claude Code's settings.json
 */
export interface Settings {
  // Provider configuration
  model?: string;
  provider?: ProviderName;

  // Permissions
  permissions?: PermissionRules;

  // UI/Display
  theme?: string;
  language?: string;
  alwaysThinkingEnabled?: boolean;

  // Environment variables
  env?: Record<string, string>;

  // Attribution for commits/PRs
  attribution?: {
    commit?: string;
    pr?: string;
  };

  // Plugin configuration
  enabledPlugins?: Record<string, boolean>;
  extraKnownMarketplaces?: Record<string, unknown>;

  // Managed-only fields (cannot be overridden by lower levels)
  strictKnownMarketplaces?: unknown[];

  // Catch-all for future fields
  [key: string]: unknown;
}

// =============================================================================
// Configuration Level Types
// =============================================================================

/**
 * Configuration level identifiers
 */
export type ConfigLevelType = 'managed' | 'user' | 'extra' | 'project' | 'local' | 'cli';

/**
 * Represents a configuration level with its metadata
 */
export interface ConfigLevel {
  type: ConfigLevelType;
  priority: number; // Higher number = higher priority
  paths: string[]; // Paths to check (in order: gencode first, then claude)
  description: string;
}

/**
 * A loaded configuration source
 */
export interface ConfigSource {
  level: ConfigLevelType;
  path: string;
  namespace: 'gencode' | 'claude' | 'extra';
  settings: Settings;
}

/**
 * Result of merging all configuration sources
 */
export interface MergedConfig {
  settings: Settings;
  sources: ConfigSource[];
  managedDeny: string[]; // Deny rules that cannot be overridden
}

// =============================================================================
// Constants
// =============================================================================

export const GENCODE_CONFIG_DIRS_ENV = 'GENCODE_CONFIG_DIRS';

// File names
export const SETTINGS_FILE_NAME = 'settings.json';
export const SETTINGS_LOCAL_FILE_NAME = 'settings.local.json';
export const MANAGED_SETTINGS_FILE_NAME = 'managed-settings.json';
export const PROVIDERS_FILE_NAME = 'providers.json';

// Directory names
export const GENCODE_DIR = '.gencode';
export const CLAUDE_DIR = '.claude';

// User directory paths
export const USER_GENCODE_DIR = path.join(os.homedir(), GENCODE_DIR);
export const USER_CLAUDE_DIR = path.join(os.homedir(), CLAUDE_DIR);

// Managed settings locations by platform
export function getManagedPaths(): { gencode: string; claude: string } {
  const platform = os.platform();

  if (platform === 'darwin') {
    return {
      gencode: '/Library/Application Support/GenCode',
      claude: '/Library/Application Support/ClaudeCode',
    };
  } else if (platform === 'win32') {
    return {
      gencode: 'C:\\Program Files\\GenCode',
      claude: 'C:\\Program Files\\ClaudeCode',
    };
  } else {
    // Linux and other Unix-like systems
    return {
      gencode: '/etc/gencode',
      claude: '/etc/claude-code',
    };
  }
}

// =============================================================================
// Legacy Types (for backward compatibility)
// =============================================================================

export interface SettingsManagerOptions {
  settingsDir?: string;
}

// Legacy exports
export const DEFAULT_SETTINGS_DIR = '~/.claude';
export const PROJECT_SETTINGS_DIR = '.claude';
export const FALLBACK_SETTINGS_DIR = '~/.gencode';
export const FALLBACK_PROJECT_DIR = '.gencode';

// =============================================================================
// Provider Connection Types
// =============================================================================

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
