/**
 * Providers Config Manager - Reads providers.json for model-to-provider mapping
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { ProvidersConfig, ProviderName } from './types.js';
import { DEFAULT_SETTINGS_DIR, PROVIDERS_FILE_NAME } from './types.js';

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
   * Infer provider from model ID using cached models in providers.json
   * Returns undefined if model not found in any provider's cached list
   */
  inferProvider(modelId: string): ProviderName | undefined {
    if (!this.config?.models) {
      return undefined;
    }

    for (const [providerKey, providerModels] of Object.entries(this.config.models)) {
      const found = providerModels.list?.some((m) => m.id === modelId);
      if (found) {
        // Map provider key to ProviderName
        // Note: 'anthropic' in providers.json might use vertex connection
        if (providerKey === 'gemini') {
          return 'gemini';
        } else if (providerKey === 'anthropic') {
          // Check connection method to determine if vertex or direct
          const connection = this.config.connections?.[providerKey];
          if (connection?.method === 'vertex') {
            return 'vertex-ai';
          }
          return 'anthropic';
        } else if (providerKey === 'openai') {
          return 'openai';
        }
      }
    }

    return undefined;
  }

  /**
   * Get all model IDs for a provider
   */
  getModelIds(provider: string): string[] {
    if (!this.config?.models?.[provider]) {
      return [];
    }
    return this.config.models[provider].list?.map((m) => m.id) ?? [];
  }
}
