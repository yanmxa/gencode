/**
 * Providers Config Manager - Reads providers.json for model-to-provider mapping
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { ProvidersConfig, Provider, AuthMethod } from './types.js';
import { DEFAULT_SETTINGS_DIR, PROVIDERS_FILE_NAME } from './types.js';
import { parseModelCacheKey } from '../../core/providers/store.js';

export class ProvidersConfigManager {
  private settingsDir: string;
  private providersPath: string;
  private config: ProvidersConfig | null = null;

  constructor(settingsDir?: string) {
    const dir = settingsDir ?? DEFAULT_SETTINGS_DIR;
    this.settingsDir = dir.replace('~', os.homedir());
    this.providersPath = path.join(this.settingsDir, PROVIDERS_FILE_NAME);
  }

  /**
   * Load providers config from disk
   */
  async load(): Promise<ProvidersConfig | null> {
    try {
      const content = await fs.readFile(this.providersPath, 'utf-8');
      this.config = JSON.parse(content);
      return this.config;
    } catch {
      // File doesn't exist or is invalid
      this.config = null;
      return null;
    }
  }

  /**
   * Get cached config (call load() first)
   */
  get(): ProvidersConfig | null {
    return this.config;
  }

  /**
   * Infer provider and auth method from model ID using cached models
   * Returns { provider, authMethod } if found, undefined if not in cache
   */
  inferProviderFromCache(
    modelId: string
  ): { provider: Provider; authMethod: AuthMethod } | undefined {
    if (!this.config?.models) {
      return undefined;
    }

    for (const [key, providerModels] of Object.entries(this.config.models)) {
      const found = providerModels.list?.some((m) => m.id === modelId);
      if (found) {
        // Parse key to get provider and authMethod
        const parsed = parseModelCacheKey(key);
        if (parsed) {
          return parsed;
        }
      }
    }

    return undefined;
  }

  /**
   * Get all model IDs for a provider (across all authMethods)
   */
  getModelIds(provider: string): string[] {
    if (!this.config?.models) {
      return [];
    }

    const modelIds: string[] = [];
    for (const [key, cache] of Object.entries(this.config.models)) {
      const parsed = parseModelCacheKey(key);
      if (parsed && parsed.provider === provider) {
        modelIds.push(...(cache.list?.map((m) => m.id) ?? []));
      }
    }
    return modelIds;
  }
}
