/**
 * Provider Store - Manages provider connections and model cache
 *
 * Storage location: ~/.gencode/providers.json
 */

import { existsSync, readFileSync, writeFileSync, mkdirSync } from 'fs';
import { join } from 'path';
import { homedir } from 'os';
import type { ProviderName } from './index.js';
import type { SearchProviderName } from './search/types.js';

export interface ModelInfo {
  id: string;
  name: string;
}

export interface ProviderConnection {
  method: string;
  connectedAt: string;
}

export interface ModelCache {
  cachedAt: string;
  list: ModelInfo[];
}

export interface ProvidersConfig {
  connections: Record<string, ProviderConnection>;
  models: Record<string, ModelCache>;
  searchProvider?: SearchProviderName;
}

const CONFIG_DIR = join(homedir(), '.gencode');
const CONFIG_FILE = join(CONFIG_DIR, 'providers.json');

/**
 * Provider Store - manages connection state and model cache
 */
export class ProviderStore {
  private config: ProvidersConfig;

  constructor() {
    this.config = this.load();
  }

  /**
   * Load configuration from disk
   */
  private load(): ProvidersConfig {
    try {
      if (existsSync(CONFIG_FILE)) {
        const data = readFileSync(CONFIG_FILE, 'utf-8');
        return JSON.parse(data);
      }
    } catch {
      // Ignore parse errors, start fresh
    }
    return { connections: {}, models: {} };
  }

  /**
   * Save configuration to disk
   */
  private save(): void {
    try {
      if (!existsSync(CONFIG_DIR)) {
        mkdirSync(CONFIG_DIR, { recursive: true });
      }
      writeFileSync(CONFIG_FILE, JSON.stringify(this.config, null, 2));
    } catch {
      // Silently fail if we can't write
    }
  }

  /**
   * Check if a provider is connected
   */
  isConnected(providerId: ProviderName): boolean {
    return !!this.config.connections[providerId];
  }

  /**
   * Get connection info for a provider
   */
  getConnection(providerId: ProviderName): ProviderConnection | undefined {
    return this.config.connections[providerId];
  }

  /**
   * Get all connected provider IDs
   */
  getConnectedProviders(): ProviderName[] {
    return Object.keys(this.config.connections) as ProviderName[];
  }

  /**
   * Connect a provider
   */
  connect(providerId: ProviderName, method: string): void {
    this.config.connections[providerId] = {
      method,
      connectedAt: new Date().toISOString(),
    };
    this.save();
  }

  /**
   * Disconnect a provider
   */
  disconnect(providerId: ProviderName): void {
    delete this.config.connections[providerId];
    delete this.config.models[providerId];
    this.save();
  }

  /**
   * Get cached models for a provider
   */
  getModels(providerId: ProviderName): ModelInfo[] {
    return this.config.models[providerId]?.list ?? [];
  }

  /**
   * Get all cached models grouped by provider
   */
  getAllModels(): Record<ProviderName, ModelInfo[]> {
    const result: Record<string, ModelInfo[]> = {};
    for (const [providerId, cache] of Object.entries(this.config.models)) {
      result[providerId] = cache.list;
    }
    return result as Record<ProviderName, ModelInfo[]>;
  }

  /**
   * Cache models for a provider
   */
  cacheModels(providerId: ProviderName, models: ModelInfo[]): void {
    this.config.models[providerId] = {
      cachedAt: new Date().toISOString(),
      list: models,
    };
    this.save();
  }

  /**
   * Get cache timestamp for a provider
   */
  getCacheTime(providerId: ProviderName): Date | undefined {
    const cache = this.config.models[providerId];
    return cache ? new Date(cache.cachedAt) : undefined;
  }

  /**
   * Check if model cache is stale (older than 24 hours)
   */
  isCacheStale(providerId: ProviderName): boolean {
    const cacheTime = this.getCacheTime(providerId);
    if (!cacheTime) return true;
    const hoursSinceCache = (Date.now() - cacheTime.getTime()) / (1000 * 60 * 60);
    return hoursSinceCache > 24;
  }

  /**
   * Get total model count across all connected providers
   */
  getTotalModelCount(): number {
    return Object.values(this.config.models).reduce(
      (sum, cache) => sum + cache.list.length,
      0
    );
  }

  /**
   * Get model count for a specific provider
   */
  getModelCount(providerId: ProviderName): number {
    return this.config.models[providerId]?.list.length ?? 0;
  }

  /**
   * Get the configured search provider
   */
  getSearchProvider(): SearchProviderName | undefined {
    return this.config.searchProvider;
  }

  /**
   * Set the search provider
   */
  setSearchProvider(id: SearchProviderName): void {
    this.config.searchProvider = id;
    this.save();
  }

  /**
   * Clear the search provider (use default)
   */
  clearSearchProvider(): void {
    delete this.config.searchProvider;
    this.save();
  }
}

// Singleton instance
let storeInstance: ProviderStore | null = null;

/**
 * Get the singleton provider store instance
 */
export function getProviderStore(): ProviderStore {
  if (!storeInstance) {
    storeInstance = new ProviderStore();
  }
  return storeInstance;
}
