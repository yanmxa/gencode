/**
 * Input History Manager
 * Manages command history with persistence, navigation, and deduplication
 */

import { promises as fs } from 'fs';
import { homedir } from 'os';
import { join, dirname } from 'path';

export interface HistoryEntry {
  text: string;
  timestamp: string;
}

export interface HistoryConfig {
  enabled?: boolean;
  maxSize?: number;
  savePath?: string;
  deduplicateConsecutive?: boolean;
}

interface HistoryFile {
  entries: HistoryEntry[];
  maxSize: number;
}

const DEFAULT_CONFIG: Required<HistoryConfig> = {
  enabled: true,
  maxSize: 1000,
  savePath: '~/.gen/input-history.json',
  deduplicateConsecutive: true,
};

export class InputHistoryManager {
  private entries: HistoryEntry[] = [];
  private currentPosition = -1; // -1 means not navigating
  private config: Required<HistoryConfig>;
  private savePath: string;
  private saveTimeout: NodeJS.Timeout | null = null;
  private isLoaded = false;

  constructor(config: HistoryConfig = {}) {
    this.config = { ...DEFAULT_CONFIG, ...config };
    this.savePath = this.resolveTildePath(this.config.savePath);
  }

  /**
   * Resolve ~ to home directory
   */
  private resolveTildePath(path: string): string {
    return path.startsWith('~/') ? join(homedir(), path.slice(2)) : path;
  }

  /**
   * Load history from disk
   */
  async load(): Promise<void> {
    if (!this.config.enabled) {
      return;
    }

    try {
      const data = await fs.readFile(this.savePath, 'utf-8');
      const historyFile: HistoryFile = JSON.parse(data);

      // Validate and load entries
      if (Array.isArray(historyFile.entries)) {
        this.entries = historyFile.entries.filter(
          (entry) => entry && typeof entry.text === 'string'
        );

        // Prune if maxSize changed
        if (this.entries.length > this.config.maxSize) {
          this.entries = this.entries.slice(-this.config.maxSize);
        }
      }

      this.isLoaded = true;
    } catch (error: unknown) {
      // File doesn't exist or is corrupt - start with empty history
      if (error instanceof Error && 'code' in error && error.code !== 'ENOENT') {
        console.error('Failed to load input history:', error.message);
      }
      this.entries = [];
      this.isLoaded = true;
    }
  }

  /**
   * Save history to disk (async, debounced)
   */
  async save(): Promise<void> {
    if (!this.config.enabled || !this.isLoaded) {
      return;
    }

    // Debounce saves to avoid excessive writes
    if (this.saveTimeout) {
      clearTimeout(this.saveTimeout);
    }

    this.saveTimeout = setTimeout(async () => {
      try {
        // Ensure directory exists
        const dir = dirname(this.savePath);
        await fs.mkdir(dir, { recursive: true });

        const historyFile: HistoryFile = {
          entries: this.entries,
          maxSize: this.config.maxSize,
        };

        await fs.writeFile(
          this.savePath,
          JSON.stringify(historyFile, null, 2),
          'utf-8'
        );
      } catch (error: unknown) {
        // Log but don't throw - saving history should not break the app
        const message = error instanceof Error ? error.message : String(error);
        console.error('Failed to save input history:', message);
      }
    }, 100); // 100ms debounce
  }

  /**
   * Flush pending saves immediately (for app exit)
   */
  async flush(): Promise<void> {
    if (this.saveTimeout) {
      clearTimeout(this.saveTimeout);
      this.saveTimeout = null;
    }

    if (!this.config.enabled || !this.isLoaded) {
      return;
    }

    try {
      const dir = dirname(this.savePath);
      await fs.mkdir(dir, { recursive: true });

      const historyFile: HistoryFile = {
        entries: this.entries,
        maxSize: this.config.maxSize,
      };

      await fs.writeFile(
        this.savePath,
        JSON.stringify(historyFile, null, 2),
        'utf-8'
      );
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error);
      console.error('Failed to flush input history:', message);
    }
  }

  /**
   * Add a new entry to history
   */
  add(text: string): void {
    if (!this.config.enabled || !text.trim()) {
      return;
    }

    const trimmedText = text.trim();

    // Deduplicate consecutive entries
    if (this.config.deduplicateConsecutive && this.entries.length > 0) {
      const lastEntry = this.entries[this.entries.length - 1];
      if (lastEntry.text === trimmedText) {
        return; // Skip duplicate
      }
    }

    // Add new entry
    this.entries.push({
      text: trimmedText,
      timestamp: new Date().toISOString(),
    });

    // Prune old entries if exceeding maxSize
    if (this.entries.length > this.config.maxSize) {
      this.entries = this.entries.slice(-this.config.maxSize);
    }

    // Reset navigation position
    this.currentPosition = -1;

    // Save asynchronously
    void this.save();
  }

  /**
   * Navigate to previous entry (older)
   * Returns the entry text or null if at beginning
   */
  previous(): string | null {
    if (!this.config.enabled || this.entries.length === 0) {
      return null;
    }

    // First time navigating - start from end
    if (this.currentPosition === -1) {
      this.currentPosition = this.entries.length - 1;
      return this.entries[this.currentPosition].text;
    }

    // Already at beginning
    if (this.currentPosition === 0) {
      return this.entries[0].text;
    }

    // Move to previous entry
    this.currentPosition--;
    return this.entries[this.currentPosition].text;
  }

  /**
   * Navigate to next entry (newer)
   * Returns the entry text or null if at end (original input)
   */
  next(): string | null {
    if (!this.config.enabled || this.entries.length === 0) {
      return null;
    }

    // Not navigating or at end
    if (this.currentPosition === -1) {
      return null;
    }

    // Move to next entry
    this.currentPosition++;

    // Reached end - return to original input
    if (this.currentPosition >= this.entries.length) {
      this.currentPosition = -1;
      return null; // Signal to restore original input
    }

    return this.entries[this.currentPosition].text;
  }

  /**
   * Reset navigation state (cancel history navigation)
   */
  reset(): void {
    this.currentPosition = -1;
  }

  /**
   * Check if currently navigating history
   */
  isNavigating(): boolean {
    return this.currentPosition !== -1;
  }

  /**
   * Get current position in history (-1 if not navigating)
   */
  getPosition(): number {
    return this.currentPosition;
  }

  /**
   * Get total number of entries
   */
  size(): number {
    return this.entries.length;
  }

  /**
   * Get all entries (for debugging/testing)
   */
  getEntries(): readonly HistoryEntry[] {
    return this.entries;
  }

  /**
   * Clear all history
   */
  clear(): void {
    this.entries = [];
    this.currentPosition = -1;
    void this.save();
  }
}
