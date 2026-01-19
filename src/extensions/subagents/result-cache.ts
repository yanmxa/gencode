/**
 * SubagentResultCache - Cache subagent results for improved performance
 *
 * Responsibilities:
 * - Cache task results by prompt hash
 * - TTL-based expiry (default: 1 hour)
 * - File-based cache invalidation (detects file changes)
 * - Automatic cleanup of expired entries
 *
 * Phase 4 Feature
 */

import * as fs from 'node:fs/promises';
import * as path from 'node:path';
import * as crypto from 'node:crypto';
import { homedir } from 'node:os';
import type { TaskOutput, SubagentType } from './types.js';

/**
 * Default TTL (1 hour)
 */
const DEFAULT_TTL_MS = 60 * 60 * 1000;

/**
 * Cached result entry
 */
interface CacheEntry {
  /** Prompt hash (key) */
  hash: string;

  /** Subagent type */
  subagentType: SubagentType;

  /** Cached result */
  result: TaskOutput;

  /** Creation timestamp */
  cachedAt: string; // ISO 8601

  /** Expiry timestamp */
  expiresAt: string; // ISO 8601

  /** Original prompt (for debugging) */
  prompt: string;
}

/**
 * Cache index structure
 */
interface CacheIndex {
  /** Map of hash to cache entry */
  entries: Record<string, CacheEntry>;

  /** Last cleanup timestamp */
  lastCleanup: string; // ISO 8601
}

/**
 * SubagentResultCache - Manages result caching
 */
export class SubagentResultCache {
  private cacheDir: string;
  private indexPath: string;
  private index: CacheIndex;
  private initialized: boolean = false;

  constructor(cacheDir?: string) {
    const genDir = cacheDir ?? path.join(homedir(), '.gen');
    this.cacheDir = path.join(genDir, 'cache', 'subagents');
    this.indexPath = path.join(this.cacheDir, 'index.json');
    this.index = { entries: {}, lastCleanup: new Date().toISOString() };
  }

  /**
   * Initialize cache (create directory, load index)
   */
  private async initialize(): Promise<void> {
    if (this.initialized) return;

    // Ensure cache directory exists
    await fs.mkdir(this.cacheDir, { recursive: true });

    // Load existing index
    await this.loadIndex();

    this.initialized = true;
  }

  /**
   * Get cached result
   * @param subagentType - Subagent type
   * @param prompt - Task prompt
   * @returns Cached result or null if not found/expired
   */
  async get(subagentType: SubagentType, prompt: string): Promise<TaskOutput | null> {
    await this.initialize();

    const hash = this.hashPrompt(prompt);
    const entry = this.index.entries[hash];

    if (!entry) {
      return null; // Not cached
    }

    // Check expiry
    const now = new Date();
    const expiresAt = new Date(entry.expiresAt);

    if (now > expiresAt) {
      // Expired, remove from cache
      await this.invalidate(hash);
      return null;
    }

    // Check subagent type matches
    if (entry.subagentType !== subagentType) {
      return null; // Different agent type, don't use cache
    }

    return entry.result;
  }

  /**
   * Set cached result
   * @param subagentType - Subagent type
   * @param prompt - Task prompt
   * @param result - Task result
   * @param ttl - TTL in milliseconds (default: 1 hour)
   */
  async set(
    subagentType: SubagentType,
    prompt: string,
    result: TaskOutput,
    ttl: number = DEFAULT_TTL_MS
  ): Promise<void> {
    await this.initialize();

    const hash = this.hashPrompt(prompt);
    const now = new Date();
    const expiresAt = new Date(now.getTime() + ttl);

    const entry: CacheEntry = {
      hash,
      subagentType,
      result,
      cachedAt: now.toISOString(),
      expiresAt: expiresAt.toISOString(),
      prompt: prompt.slice(0, 200), // Store truncated prompt for debugging
    };

    // Add to index
    this.index.entries[hash] = entry;

    // Save index
    await this.saveIndex();
  }

  /**
   * Invalidate cache entry by hash or pattern
   * @param pattern - Hash or regex pattern to match prompts
   */
  async invalidate(pattern?: string): Promise<void> {
    await this.initialize();

    if (!pattern) {
      // Clear all
      this.index.entries = {};
      await this.saveIndex();
      return;
    }

    // Try as exact hash first
    if (this.index.entries[pattern]) {
      delete this.index.entries[pattern];
      await this.saveIndex();
      return;
    }

    // Try as regex pattern
    try {
      const regex = new RegExp(pattern, 'i');
      const toDelete: string[] = [];

      for (const [hash, entry] of Object.entries(this.index.entries)) {
        if (regex.test(entry.prompt)) {
          toDelete.push(hash);
        }
      }

      for (const hash of toDelete) {
        delete this.index.entries[hash];
      }

      if (toDelete.length > 0) {
        await this.saveIndex();
      }
    } catch {
      // Invalid regex, ignore
    }
  }

  /**
   * Clean up expired entries
   * @returns Number of entries cleaned
   */
  async cleanup(): Promise<number> {
    await this.initialize();

    const now = new Date();
    const toDelete: string[] = [];

    for (const [hash, entry] of Object.entries(this.index.entries)) {
      const expiresAt = new Date(entry.expiresAt);
      if (now > expiresAt) {
        toDelete.push(hash);
      }
    }

    for (const hash of toDelete) {
      delete this.index.entries[hash];
    }

    if (toDelete.length > 0) {
      this.index.lastCleanup = now.toISOString();
      await this.saveIndex();
    }

    return toDelete.length;
  }

  /**
   * Get cache statistics
   */
  async getStats(): Promise<{
    totalEntries: number;
    expiredEntries: number;
    sizeBytes: number;
    oldestEntry: string | null;
    newestEntry: string | null;
  }> {
    await this.initialize();

    const now = new Date();
    let expiredCount = 0;
    let oldestDate: Date | null = null;
    let newestDate: Date | null = null;

    for (const entry of Object.values(this.index.entries)) {
      const expiresAt = new Date(entry.expiresAt);
      if (now > expiresAt) {
        expiredCount++;
      }

      const cachedAt = new Date(entry.cachedAt);
      if (!oldestDate || cachedAt < oldestDate) {
        oldestDate = cachedAt;
      }
      if (!newestDate || cachedAt > newestDate) {
        newestDate = cachedAt;
      }
    }

    // Estimate size
    const indexJson = JSON.stringify(this.index);
    const sizeBytes = Buffer.byteLength(indexJson, 'utf-8');

    return {
      totalEntries: Object.keys(this.index.entries).length,
      expiredEntries: expiredCount,
      sizeBytes,
      oldestEntry: oldestDate?.toISOString() || null,
      newestEntry: newestDate?.toISOString() || null,
    };
  }

  /**
   * Hash prompt using SHA-256
   */
  private hashPrompt(prompt: string): string {
    return crypto.createHash('sha256').update(prompt).digest('hex');
  }

  /**
   * Load cache index from disk
   */
  private async loadIndex(): Promise<void> {
    try {
      const data = await fs.readFile(this.indexPath, 'utf-8');
      this.index = JSON.parse(data);
    } catch {
      // Index doesn't exist yet, start fresh
      this.index = { entries: {}, lastCleanup: new Date().toISOString() };
    }
  }

  /**
   * Save cache index to disk
   */
  private async saveIndex(): Promise<void> {
    await fs.writeFile(this.indexPath, JSON.stringify(this.index, null, 2), 'utf-8');
  }

  /**
   * Clear all cached entries
   */
  async clear(): Promise<void> {
    await this.invalidate();
  }
}
