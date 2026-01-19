/**
 * Configuration Module - Multi-level configuration system (Claude Code compatible)
 *
 * This module provides a hierarchical configuration system that:
 * - Supports multiple levels: user, project, local, managed
 * - Merges .gencode and .claude directories at each level (gencode wins)
 * - Supports GENCODE_CONFIG_DIRS environment variable for extra config dirs
 * - Enforces managed settings that cannot be overridden
 */

// Core types
export type {
  Settings,
  SettingsManagerOptions,
  Provider,
  AuthMethod,
  ProviderName,
  PermissionRules,
  ConfigLevelType,
  ConfigLevel,
  ConfigSource,
  MergedConfig,
  ProvidersConfig,
  ProviderConnection,
  CachedModel,
  ProviderModels,
} from './types.js';

// Constants
export {
  DEFAULT_SETTINGS_DIR,
  PROJECT_SETTINGS_DIR,
  FALLBACK_SETTINGS_DIR,
  FALLBACK_PROJECT_DIR,
  SETTINGS_FILE_NAME,
  SETTINGS_LOCAL_FILE_NAME,
  MANAGED_SETTINGS_FILE_NAME,
  PROVIDERS_FILE_NAME,
  GEN_DIR,
  CLAUDE_DIR,
  USER_GEN_DIR,
  USER_CLAUDE_DIR,
  GEN_CONFIG_ENV,
  getManagedPaths,
} from './types.js';

// Configuration levels
export {
  findProjectRoot,
  parseExtraConfigDirs,
  getConfigLevels,
  getPrimarySettingsDir,
  getSettingsFilePath,
  type ConfigPathInfo,
  type ResolvedLevel,
} from './levels.js';

// Configuration loader
export {
  loadAllSources,
  loadSourcesByLevel,
  loadUserSettings,
  loadProjectSettings,
  loadManagedSettings,
  getConfigInfo,
  getExistingConfigFiles,
} from './loader.js';

// Configuration merger
export {
  deepMerge,
  mergeSettings,
  extractManagedDeny,
  applyManagedRestrictions,
  mergeAllSources,
  mergeWithCliArgs,
  createMergeSummary,
} from './merger.js';

// Configuration managers
export { ConfigManager, SettingsManager } from './manager.js';

// Providers configuration
export { ProvidersConfigManager } from './providers-config.js';
