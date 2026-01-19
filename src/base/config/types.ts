/**
 * Configuration Types - Multi-level configuration system (Claude Code compatible)
 *
 * Configuration hierarchy (priority from low to high):
 * 1. User Level: ~/.gen/ + ~/.claude/ (merged, gen wins)
 * 2. Extra Dirs: GEN_CONFIG environment variable
 * 3. Project Level: .gen/ + .claude/ (merged, gen wins)
 * 4. Local Level: .gen/*.local.* + .claude/*.local.* (merged, gen wins)
 * 5. CLI Arguments: Command line overrides
 * 6. Managed Level: System-wide enforced settings (cannot be overridden)
 */

import * as os from 'os';
import * as path from 'path';

// =============================================================================
// Provider Types
// =============================================================================

export type Provider = 'openai' | 'anthropic' | 'google';
export type AuthMethod = 'api_key' | 'vertex' | 'bedrock' | 'azure' | 'oauth';

// Legacy type alias for backward compatibility
/** @deprecated Use Provider instead */
export type ProviderName = Provider;

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
  provider?: Provider;

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

  // Memory configuration
  memoryMergeStrategy?: 'fallback' | 'both' | 'gen-only' | 'claude-only';

  // Compression configuration
  compression?: {
    enabled?: boolean;
    enablePruning?: boolean;
    enableCompaction?: boolean;
    pruneMinimum?: number;
    pruneProtect?: number;
    reservedOutputTokens?: number;
    model?: string;
  };

  // Input history configuration
  inputHistory?: {
    enabled?: boolean;
    maxSize?: number;
    savePath?: string;
    deduplicateConsecutive?: boolean;
  };

  // Input behavior configuration
  input?: {
    multilineEnabled?: boolean; // Default: true - Enable Shift+Enter for multi-line input
    ctrlCClear?: boolean; // Default: true - Clear input on Ctrl+C if text present
  };

  // Hooks configuration (event-driven shell commands)
  hooks?: {
    [event: string]: Array<{
      matcher?: string;
      hooks: Array<{
        type: 'command' | 'prompt';
        command?: string;
        prompt?: string;
        timeout?: number;
        statusMessage?: string;
        blocking?: boolean;
      }>;
    }>;
  };

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
  namespace: 'gen' | 'claude' | 'extra';
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

export const GEN_CONFIG_ENV = 'GEN_CONFIG';

// File names
export const SETTINGS_FILE_NAME = 'settings.json';
export const SETTINGS_LOCAL_FILE_NAME = 'settings.local.json';
export const MANAGED_SETTINGS_FILE_NAME = 'managed-settings.json';
export const PROVIDERS_FILE_NAME = 'providers.json';

// Directory names - imported from common utilities and re-exported
import { GEN_DIR, CLAUDE_DIR, getManagedPaths, type ManagedPaths } from '../utils/path-utils.js';
export { GEN_DIR, CLAUDE_DIR, getManagedPaths };
export type { ManagedPaths };

// User directory paths
export const USER_GEN_DIR = path.join(os.homedir(), GEN_DIR);
export const USER_CLAUDE_DIR = path.join(os.homedir(), CLAUDE_DIR);

// =============================================================================
// Legacy Types (for backward compatibility)
// =============================================================================

export interface SettingsManagerOptions {
  settingsDir?: string;
}

// Legacy exports
export const DEFAULT_SETTINGS_DIR = '~/.gen';
export const PROJECT_SETTINGS_DIR = '.gen';
export const FALLBACK_SETTINGS_DIR = '~/.claude';
export const FALLBACK_PROJECT_DIR = '.claude';

// =============================================================================
// Provider Connection Types
// =============================================================================

/**
 * Provider connection info
 */
export interface ProviderConnection {
  authMethod: AuthMethod; // Authentication method
  method?: string; // Legacy: Connection name (e.g., "Direct API", "Google Vertex AI")
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
 * Note: provider and authMethod are encoded in the key as "provider:authMethod"
 */
export interface ProviderModels {
  cachedAt: string;
  list: CachedModel[];
}

/**
 * Providers config file structure (~/.gen/providers.json)
 */
export interface ProvidersConfig {
  connections: Record<string, ProviderConnection>;
  models: Record<string, ProviderModels>;
}
