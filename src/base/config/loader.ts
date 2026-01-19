/**
 * Configuration Loader - Load settings from various sources
 *
 * Loads configuration files from all levels and namespaces.
 * Each level loads both .gencode and .claude directories.
 */

import * as fs from 'fs/promises';
import type { Settings, ConfigSource, ConfigLevelType } from './types.js';
import { getConfigLevels, type ResolvedLevel, type ConfigPathInfo } from './levels.js';

/**
 * Load a single JSON settings file
 */
async function loadJsonFile(filePath: string): Promise<Settings | null> {
  try {
    const content = await fs.readFile(filePath, 'utf-8');
    return JSON.parse(content);
  } catch {
    return null;
  }
}

/**
 * Load settings from a specific path info
 */
async function loadFromPathInfo(
  pathInfo: ConfigPathInfo,
  levelType: ConfigLevelType
): Promise<ConfigSource | null> {
  const settings = await loadJsonFile(pathInfo.settingsPath);
  if (!settings) return null;

  return {
    level: levelType,
    path: pathInfo.settingsPath,
    namespace: pathInfo.namespace,
    settings,
  };
}

/**
 * Load all configuration sources in order
 *
 * Returns an array of ConfigSource in priority order (lowest first).
 * Within each level:
 * - .claude is loaded first (lower priority)
 * - .gencode is loaded second (higher priority)
 *
 * This allows proper merging where:
 * - Later sources override earlier ones
 * - GenCode settings override Claude settings at the same level
 */
export async function loadAllSources(cwd: string): Promise<ConfigSource[]> {
  const levels = await getConfigLevels(cwd);
  const sources: ConfigSource[] = [];

  for (const level of levels) {
    // Load from each path in the level (claude first, then gencode)
    for (const pathInfo of level.paths) {
      const source = await loadFromPathInfo(pathInfo, level.type);
      if (source) {
        sources.push(source);
      }
    }
  }

  return sources;
}

/**
 * Load configuration sources grouped by level
 */
export async function loadSourcesByLevel(
  cwd: string
): Promise<Map<ConfigLevelType, ConfigSource[]>> {
  const levels = await getConfigLevels(cwd);
  const sourcesByLevel = new Map<ConfigLevelType, ConfigSource[]>();

  for (const level of levels) {
    const levelSources: ConfigSource[] = [];

    for (const pathInfo of level.paths) {
      const source = await loadFromPathInfo(pathInfo, level.type);
      if (source) {
        levelSources.push(source);
      }
    }

    if (levelSources.length > 0) {
      sourcesByLevel.set(level.type, levelSources);
    }
  }

  return sourcesByLevel;
}

/**
 * Load only user-level settings
 */
export async function loadUserSettings(cwd: string): Promise<ConfigSource[]> {
  const levels = await getConfigLevels(cwd);
  const userLevel = levels.find((l) => l.type === 'user');
  if (!userLevel) return [];

  const sources: ConfigSource[] = [];
  for (const pathInfo of userLevel.paths) {
    const source = await loadFromPathInfo(pathInfo, 'user');
    if (source) {
      sources.push(source);
    }
  }

  return sources;
}

/**
 * Load only project-level settings
 */
export async function loadProjectSettings(cwd: string): Promise<ConfigSource[]> {
  const levels = await getConfigLevels(cwd);
  const projectLevel = levels.find((l) => l.type === 'project');
  if (!projectLevel) return [];

  const sources: ConfigSource[] = [];
  for (const pathInfo of projectLevel.paths) {
    const source = await loadFromPathInfo(pathInfo, 'project');
    if (source) {
      sources.push(source);
    }
  }

  return sources;
}

/**
 * Load only managed settings
 */
export async function loadManagedSettings(cwd: string): Promise<ConfigSource[]> {
  const levels = await getConfigLevels(cwd);
  const managedLevel = levels.find((l) => l.type === 'managed');
  if (!managedLevel) return [];

  const sources: ConfigSource[] = [];
  for (const pathInfo of managedLevel.paths) {
    const source = await loadFromPathInfo(pathInfo, 'managed');
    if (source) {
      sources.push(source);
    }
  }

  return sources;
}

/**
 * Get information about all configuration levels
 */
export async function getConfigInfo(cwd: string): Promise<ResolvedLevel[]> {
  return getConfigLevels(cwd);
}

/**
 * Check which config files exist
 */
export async function getExistingConfigFiles(cwd: string): Promise<string[]> {
  const levels = await getConfigLevels(cwd);
  const existingFiles: string[] = [];

  for (const level of levels) {
    for (const pathInfo of level.paths) {
      if (pathInfo.exists) {
        existingFiles.push(pathInfo.settingsPath);
      }
    }
  }

  return existingFiles;
}
