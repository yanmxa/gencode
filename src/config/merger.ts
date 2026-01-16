/**
 * Configuration Merger - Merge settings from multiple sources
 *
 * Implements the merge strategy:
 * - Scalar values: Higher priority replaces lower
 * - Arrays (permissions.allow, permissions.deny): Concatenate with deduplication
 * - Objects: Deep merge recursively
 * - Managed deny rules: Cannot be overridden by any level
 */

import type { Settings, ConfigSource, MergedConfig, PermissionRules } from './types.js';

/**
 * Check if a value is a plain object
 */
function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/**
 * Deep merge two objects
 *
 * - Arrays are concatenated and deduplicated
 * - Objects are recursively merged
 * - Scalars from override replace base
 */
export function deepMerge<T extends Record<string, unknown>>(
  base: T,
  override: Partial<T>
): T {
  const result = { ...base };

  for (const key in override) {
    const baseValue = result[key];
    const overrideValue = override[key];

    if (overrideValue === undefined) {
      continue;
    }

    if (isPlainObject(baseValue) && isPlainObject(overrideValue)) {
      // Recursively merge objects
      result[key] = deepMerge(
        baseValue as Record<string, unknown>,
        overrideValue as Record<string, unknown>
      ) as T[Extract<keyof T, string>];
    } else if (Array.isArray(baseValue) && Array.isArray(overrideValue)) {
      // Concatenate arrays and deduplicate
      result[key] = deduplicateArray([
        ...baseValue,
        ...overrideValue,
      ]) as T[Extract<keyof T, string>];
    } else {
      // Override scalar values
      result[key] = overrideValue as T[Extract<keyof T, string>];
    }
  }

  return result;
}

/**
 * Deduplicate an array while preserving order
 */
function deduplicateArray<T>(arr: T[]): T[] {
  const seen = new Set<string>();
  const result: T[] = [];

  for (const item of arr) {
    const key = typeof item === 'object' ? JSON.stringify(item) : String(item);
    if (!seen.has(key)) {
      seen.add(key);
      result.push(item);
    }
  }

  return result;
}

/**
 * Merge all configuration sources into a single settings object
 *
 * Sources should be in priority order (lowest first).
 * Each source is merged on top of the previous result.
 */
export function mergeSettings(sources: ConfigSource[]): Settings {
  let merged: Settings = {};

  for (const source of sources) {
    merged = deepMerge(merged, source.settings);
  }

  return merged;
}

/**
 * Extract managed deny rules from sources
 *
 * These rules are extracted from managed-level sources and cannot be
 * overridden by any lower-level configuration.
 */
export function extractManagedDeny(sources: ConfigSource[]): string[] {
  const managedDeny: string[] = [];

  for (const source of sources) {
    if (source.level === 'managed') {
      const deny = source.settings.permissions?.deny;
      if (deny) {
        managedDeny.push(...deny);
      }
    }
  }

  return deduplicateArray(managedDeny);
}

/**
 * Apply managed restrictions to the merged settings
 *
 * - Ensures managed deny rules are always in the deny list
 * - Removes managed deny patterns from allow list
 */
export function applyManagedRestrictions(
  settings: Settings,
  managedDeny: string[]
): Settings {
  if (managedDeny.length === 0) {
    return settings;
  }

  const result = { ...settings };
  const permissions: PermissionRules = { ...(result.permissions || {}) };

  // Ensure deny list includes all managed deny rules
  const currentDeny = permissions.deny || [];
  permissions.deny = deduplicateArray([...currentDeny, ...managedDeny]);

  // Remove managed deny patterns from allow list
  if (permissions.allow) {
    permissions.allow = permissions.allow.filter(
      (pattern) => !managedDeny.includes(pattern)
    );
  }

  result.permissions = permissions;
  return result;
}

/**
 * Merge all configuration sources and apply restrictions
 *
 * This is the main entry point for configuration merging.
 */
export function mergeAllSources(sources: ConfigSource[]): MergedConfig {
  // Extract managed deny rules first
  const managedDeny = extractManagedDeny(sources);

  // Merge all settings
  let settings = mergeSettings(sources);

  // Apply managed restrictions
  settings = applyManagedRestrictions(settings, managedDeny);

  return {
    settings,
    sources,
    managedDeny,
  };
}

/**
 * Merge settings with CLI arguments
 *
 * CLI arguments have the highest priority (except managed restrictions).
 */
export function mergeWithCliArgs(
  merged: MergedConfig,
  cliArgs: Partial<Settings>
): MergedConfig {
  // Merge CLI args as a virtual source
  const settings = deepMerge(merged.settings, cliArgs);

  // Re-apply managed restrictions to ensure they're not overridden
  const finalSettings = applyManagedRestrictions(settings, merged.managedDeny);

  return {
    ...merged,
    settings: finalSettings,
    sources: [
      ...merged.sources,
      {
        level: 'cli',
        path: '<cli>',
        namespace: 'gencode',
        settings: cliArgs,
      },
    ],
  };
}

/**
 * Create a debug summary of the merge process
 */
export function createMergeSummary(merged: MergedConfig): string {
  const lines: string[] = ['Configuration Sources (in priority order):'];

  for (const source of merged.sources) {
    const marker = source.level === 'managed' ? ' [enforced]' : '';
    lines.push(`  ${source.level}:${source.namespace} - ${source.path}${marker}`);
  }

  if (merged.managedDeny.length > 0) {
    lines.push('');
    lines.push('Managed Deny Rules (cannot be overridden):');
    for (const rule of merged.managedDeny) {
      lines.push(`  - ${rule}`);
    }
  }

  return lines.join('\n');
}
