/**
 * Unit tests for InputHistoryManager
 */
import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import { InputHistoryManager } from '../src/input/history-manager.js';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';

describe('InputHistoryManager', () => {
  let historyManager: InputHistoryManager;
  let testDir: string;
  let historyPath: string;

  beforeEach(async () => {
    testDir = path.join(os.tmpdir(), `gencode-history-test-${Date.now()}`);
    historyPath = path.join(testDir, 'input-history.json');

    historyManager = new InputHistoryManager({
      savePath: historyPath,
      maxSize: 10, // Small size for testing
      deduplicateConsecutive: true,
    });

    await historyManager.load();
  });

  afterEach(async () => {
    await historyManager.flush();
    try {
      await fs.rm(testDir, { recursive: true, force: true });
    } catch (error) {
      // Ignore cleanup errors
    }
  });

  describe('add()', () => {
    it('should add entries to history', () => {
      historyManager.add('hello');
      historyManager.add('world');

      expect(historyManager.size()).toBe(2);
      const entries = historyManager.getEntries();
      expect(entries[0].text).toBe('hello');
      expect(entries[1].text).toBe('world');
    });

    it('should trim whitespace from entries', () => {
      historyManager.add('  hello  ');
      historyManager.add('world\n');

      const entries = historyManager.getEntries();
      expect(entries[0].text).toBe('hello');
      expect(entries[1].text).toBe('world');
    });

    it('should skip empty entries', () => {
      historyManager.add('hello');
      historyManager.add('');
      historyManager.add('   ');
      historyManager.add('world');

      expect(historyManager.size()).toBe(2);
      const entries = historyManager.getEntries();
      expect(entries[0].text).toBe('hello');
      expect(entries[1].text).toBe('world');
    });

    it('should deduplicate consecutive entries', () => {
      historyManager.add('hello');
      historyManager.add('hello');
      historyManager.add('world');
      historyManager.add('world');

      expect(historyManager.size()).toBe(2);
      const entries = historyManager.getEntries();
      expect(entries[0].text).toBe('hello');
      expect(entries[1].text).toBe('world');
    });

    it('should not deduplicate non-consecutive duplicates', () => {
      historyManager.add('hello');
      historyManager.add('world');
      historyManager.add('hello');

      expect(historyManager.size()).toBe(3);
      const entries = historyManager.getEntries();
      expect(entries[0].text).toBe('hello');
      expect(entries[1].text).toBe('world');
      expect(entries[2].text).toBe('hello');
    });

    it('should prune old entries when exceeding maxSize', () => {
      // Add 15 entries (maxSize is 10)
      for (let i = 0; i < 15; i++) {
        historyManager.add(`entry ${i}`);
      }

      expect(historyManager.size()).toBe(10);
      const entries = historyManager.getEntries();
      expect(entries[0].text).toBe('entry 5'); // First 5 entries pruned
      expect(entries[9].text).toBe('entry 14');
    });

    it('should add timestamp to entries', () => {
      const beforeAdd = new Date();
      historyManager.add('hello');
      const afterAdd = new Date();

      const entries = historyManager.getEntries();
      const timestamp = new Date(entries[0].timestamp);

      expect(timestamp.getTime()).toBeGreaterThanOrEqual(beforeAdd.getTime());
      expect(timestamp.getTime()).toBeLessThanOrEqual(afterAdd.getTime());
    });

    it('should reset navigation position after adding', () => {
      historyManager.add('hello');
      historyManager.add('world');

      // Start navigation
      historyManager.previous();
      expect(historyManager.isNavigating()).toBe(true);

      // Add new entry - should reset navigation
      historyManager.add('test');
      expect(historyManager.isNavigating()).toBe(false);
    });
  });

  describe('previous()', () => {
    beforeEach(() => {
      historyManager.add('first');
      historyManager.add('second');
      historyManager.add('third');
    });

    it('should navigate to most recent entry first', () => {
      const entry = historyManager.previous();
      expect(entry).toBe('third');
    });

    it('should navigate backwards through history', () => {
      expect(historyManager.previous()).toBe('third');
      expect(historyManager.previous()).toBe('second');
      expect(historyManager.previous()).toBe('first');
    });

    it('should stop at beginning of history', () => {
      expect(historyManager.previous()).toBe('third');
      expect(historyManager.previous()).toBe('second');
      expect(historyManager.previous()).toBe('first');
      expect(historyManager.previous()).toBe('first'); // Stays at first
    });

    it('should return null for empty history', () => {
      const emptyManager = new InputHistoryManager({ savePath: historyPath + '.empty' });
      expect(emptyManager.previous()).toBeNull();
    });
  });

  describe('next()', () => {
    beforeEach(() => {
      historyManager.add('first');
      historyManager.add('second');
      historyManager.add('third');
    });

    it('should navigate forward through history', () => {
      historyManager.previous(); // third
      historyManager.previous(); // second
      historyManager.previous(); // first

      expect(historyManager.next()).toBe('second');
      expect(historyManager.next()).toBe('third');
    });

    it('should return null when reaching end', () => {
      historyManager.previous(); // third
      expect(historyManager.next()).toBeNull(); // End of history
    });

    it('should return null when not navigating', () => {
      expect(historyManager.next()).toBeNull();
    });

    it('should reset navigation position when reaching end', () => {
      historyManager.previous(); // Start navigation
      historyManager.next(); // Back to end

      expect(historyManager.isNavigating()).toBe(false);
      expect(historyManager.getPosition()).toBe(-1);
    });
  });

  describe('reset()', () => {
    it('should reset navigation state', () => {
      historyManager.add('hello');
      historyManager.add('world');

      historyManager.previous();
      expect(historyManager.isNavigating()).toBe(true);

      historyManager.reset();
      expect(historyManager.isNavigating()).toBe(false);
      expect(historyManager.getPosition()).toBe(-1);
    });
  });

  describe('persistence', () => {
    it('should save and load history', async () => {
      historyManager.add('hello');
      historyManager.add('world');
      await historyManager.flush();

      // Create new manager and load
      const newManager = new InputHistoryManager({ savePath: historyPath });
      await newManager.load();

      expect(newManager.size()).toBe(2);
      const entries = newManager.getEntries();
      expect(entries[0].text).toBe('hello');
      expect(entries[1].text).toBe('world');
    });

    it('should handle missing history file gracefully', async () => {
      const missingPath = path.join(testDir, 'missing.json');
      const manager = new InputHistoryManager({ savePath: missingPath });

      await expect(manager.load()).resolves.not.toThrow();
      expect(manager.size()).toBe(0);
    });

    it('should handle corrupt history file gracefully', async () => {
      // Write corrupt JSON
      await fs.mkdir(testDir, { recursive: true });
      await fs.writeFile(historyPath, 'invalid json{', 'utf-8');

      const manager = new InputHistoryManager({ savePath: historyPath });
      await expect(manager.load()).resolves.not.toThrow();
      expect(manager.size()).toBe(0);
    });

    it('should preserve maxSize setting on save/load', async () => {
      const manager1 = new InputHistoryManager({
        savePath: historyPath,
        maxSize: 5,
      });
      await manager1.load();

      for (let i = 0; i < 10; i++) {
        manager1.add(`entry ${i}`);
      }
      await manager1.flush();

      // Load with new manager (different maxSize)
      const manager2 = new InputHistoryManager({
        savePath: historyPath,
        maxSize: 3,
      });
      await manager2.load();

      // Should prune to new maxSize
      expect(manager2.size()).toBe(3);
    });
  });

  describe('clear()', () => {
    it('should clear all entries', () => {
      historyManager.add('hello');
      historyManager.add('world');

      historyManager.clear();

      expect(historyManager.size()).toBe(0);
      expect(historyManager.isNavigating()).toBe(false);
    });
  });

  describe('configuration', () => {
    it('should respect enabled flag', () => {
      const disabledManager = new InputHistoryManager({
        enabled: false,
        savePath: historyPath + '.disabled',
      });

      disabledManager.add('hello');
      expect(disabledManager.size()).toBe(0);

      expect(disabledManager.previous()).toBeNull();
      expect(disabledManager.next()).toBeNull();
    });

    it('should respect deduplicateConsecutive flag', () => {
      const noDedupe = new InputHistoryManager({
        savePath: historyPath + '.nodedupe',
        deduplicateConsecutive: false,
      });

      noDedupe.add('hello');
      noDedupe.add('hello');
      noDedupe.add('hello');

      expect(noDedupe.size()).toBe(3);
    });
  });

  describe('navigation scenarios', () => {
    it('should handle up/down/up pattern correctly', () => {
      historyManager.add('first');
      historyManager.add('second');
      historyManager.add('third');

      expect(historyManager.previous()).toBe('third');
      expect(historyManager.previous()).toBe('second');
      expect(historyManager.next()).toBe('third');
      expect(historyManager.previous()).toBe('second');
    });

    it('should handle full navigation cycle', () => {
      historyManager.add('first');
      historyManager.add('second');

      // Navigate all the way back
      expect(historyManager.previous()).toBe('second');
      expect(historyManager.previous()).toBe('first');
      expect(historyManager.previous()).toBe('first');

      // Navigate all the way forward
      expect(historyManager.next()).toBe('second');
      expect(historyManager.next()).toBeNull();
      expect(historyManager.next()).toBeNull();
    });
  });
});
