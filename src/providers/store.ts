/**
 * Provider Store - Manages provider connections and model cache
 *
 * Storage location: ~/.gen/providers.json
 */

import { existsSync, readFileSync, writeFileSync, mkdirSync } from 'fs';
import { join } from 'path';
import { homedir } from 'os';
import type { Provider, AuthMethod } from './types.js';
import type { SearchProviderName } from './search/types.js';

export interface ModelInfo {
  id: string;
  name: string;
}

export interface ProviderConnection {
  authMethod: AuthMethod; // Authentication method
  method?: string; // Legacy: Connection name (e.g., "Direct API", "Google Vertex AI")
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

const CONFIG_DIR = join(homedir(), '.gen');
const CONFIG_FILE = join(CONFIG_DIR, 'providers.json');

/**
 * Generate model cache key from provider and authMethod
 */
export function getModelCacheKey(provider: Provider, authMethod: AuthMethod): string {
  return `${provider}:${authMethod}`;
}

/**
 * Parse model cache key to extract provider and authMethod
 */
export function parseModelCacheKey(key: string): { provider: Provider; authMethod: AuthMethod } | null {
  const parts = key.split(':');
  if (parts.length !== 2) return null;
  return {
    provider: parts[0] as Provider,
    authMethod: parts[1] as AuthMethod,
  };
}

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
  isConnected(providerId: Provider): boolean {
    return !!this.config.connections[providerId];
  }

  /**
   * Get connection info for a provider
   */
  getConnection(providerId: Provider): ProviderConnection | undefined {
    return this.config.connections[providerId];
  }

  /**
   * Get all connected provider IDs
   */
  getConnectedProviders(): Provider[] {
    return Object.keys(this.config.connections) as Provider[];
  }

  /**
   * Connect a provider with auth method
   */
  connect(providerId: Provider, authMethod: AuthMethod, displayName?: string): void {
    this.config.connections[providerId] = {
      authMethod,
      method: displayName, // Optional legacy field for display
      connectedAt: new Date().toISOString(),
    };
    this.save();
  }

  /**
   * Disconnect a provider and clear all its model caches
   */
  disconnect(providerId: Provider): void {
    delete this.config.connections[providerId];

    // Remove all model caches for this provider (across all authMethods)
    for (const key of Object.keys(this.config.models)) {
      const parsed = parseModelCacheKey(key);
      if (parsed && parsed.provider === providerId) {
        delete this.config.models[key];
      }
    }

    this.save();
  }

  /**
   * Get cached models for a provider
   * @param providerId Provider ID
   * @param authMethod Optional: specific auth method. If not provided, returns all models for the provider
   */
  getModels(providerId: Provider, authMethod?: AuthMethod): ModelInfo[] {
    if (authMethod) {
      // Get models for specific authMethod
      const key = getModelCacheKey(providerId, authMethod);
      return this.config.models[key]?.list ?? [];
    }

    // Get all models for this provider (across all authMethods)
    const allModels: ModelInfo[] = [];
    for (const [key, cache] of Object.entries(this.config.models)) {
      const parsed = parseModelCacheKey(key);
      if (parsed && parsed.provider === providerId) {
        allModels.push(...cache.list);
      }
    }
    return allModels;
  }

  /**
   * Get all cached models grouped by provider
   */
  getAllModels(): Record<Provider, ModelInfo[]> {
    const result: Record<string, ModelInfo[]> = {};
    for (const [providerId, cache] of Object.entries(this.config.models)) {
      result[providerId] = cache.list;
    }
    return result as Record<Provider, ModelInfo[]>;
  }

  /**
   * Cache models for a provider with auth method
   */
  cacheModels(providerId: Provider, authMethod: AuthMethod, models: ModelInfo[]): void {
    const key = getModelCacheKey(providerId, authMethod);
    this.config.models[key] = {
      cachedAt: new Date().toISOString(),
      list: models,
    };
    this.save();
  }

  /**
   * Get cache timestamp for a provider
   * @param providerId Provider ID
   * @param authMethod Optional: specific auth method. If not provided, returns latest cache time
   */
  getCacheTime(providerId: Provider, authMethod?: AuthMethod): Date | undefined {
    if (authMethod) {
      const key = getModelCacheKey(providerId, authMethod);
      const cache = this.config.models[key];
      return cache ? new Date(cache.cachedAt) : undefined;
    }

    // Get latest cache time across all authMethods for this provider
    let latestTime: Date | undefined;
    for (const [key, cache] of Object.entries(this.config.models)) {
      const parsed = parseModelCacheKey(key);
      if (parsed && parsed.provider === providerId) {
        const cacheTime = new Date(cache.cachedAt);
        if (!latestTime || cacheTime > latestTime) {
          latestTime = cacheTime;
        }
      }
    }
    return latestTime;
  }

  /**
   * Check if model cache is stale (older than 24 hours)
   * @param providerId Provider ID
   * @param authMethod Optional: specific auth method. If not provided, checks if any cache is fresh
   */
  isCacheStale(providerId: Provider, authMethod?: AuthMethod): boolean {
    const cacheTime = this.getCacheTime(providerId, authMethod);
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
   * @param providerId Provider ID
   * @param authMethod Optional: specific auth method. If not provided, counts all models for the provider
   */
  getModelCount(providerId: Provider, authMethod?: AuthMethod): number {
    if (authMethod) {
      const key = getModelCacheKey(providerId, authMethod);
      return this.config.models[key]?.list.length ?? 0;
    }

    // Count all models for this provider (across all authMethods)
    let count = 0;
    for (const [key, cache] of Object.entries(this.config.models)) {
      const parsed = parseModelCacheKey(key);
      if (parsed && parsed.provider === providerId) {
        count += cache.list.length;
      }
    }
    return count;
  }

  /**
   * Get the model cache info
   * @param providerId Provider ID
   * @param authMethod Optional: specific auth method
   */
  getModelCache(providerId: Provider, authMethod?: AuthMethod): ModelCache | undefined {
    if (authMethod) {
      const key = getModelCacheKey(providerId, authMethod);
      return this.config.models[key];
    }

    // If no authMethod specified, try to find any cache for this provider
    for (const [key, cache] of Object.entries(this.config.models)) {
      const parsed = parseModelCacheKey(key);
      if (parsed && parsed.provider === providerId) {
        return cache;
      }
    }
    return undefined;
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
