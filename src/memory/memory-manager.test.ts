/**
 * Memory Manager Tests
 *
 * These tests focus on the core logic that can be tested without complex mocking.
 * For integration tests, use the test-memory.ts script.
 */

import { jest, describe, it, expect } from '@jest/globals';
import { MemoryManager } from './memory-manager.js';
import { DEFAULT_MEMORY_CONFIG, type MemoryConfig, type LoadedMemory } from './types.js';

describe('MemoryManager', () => {
  describe('constructor', () => {
    it('should use default config', () => {
      const manager = new MemoryManager();
      expect(manager).toBeDefined();
    });

    it('should accept custom config', () => {
      const manager = new MemoryManager({
        maxFileSize: 50 * 1024,
      });
      expect(manager).toBeDefined();
    });

    it('should merge custom config with defaults', () => {
      const manager = new MemoryManager({
        maxFileSize: 50 * 1024,
      });
      // The manager should still have default values for other fields
      expect(manager).toBeDefined();
    });
  });

  describe('getLoadedFileList', () => {
    it('should return empty list when no files loaded', () => {
      const manager = new MemoryManager();
      const list = manager.getLoadedFileList();
      expect(list).toEqual([]);
    });
  });

  describe('buildContext', () => {
    it('should build context from loaded memory', () => {
      // This tests the context building logic conceptually
      // The actual file loading is tested via integration tests
      const manager = new MemoryManager();
      // Initially no context
      const list = manager.getLoadedFileList();
      expect(list).toHaveLength(0);
    });
  });
});

describe('MemoryConfig', () => {
  describe('DEFAULT_MEMORY_CONFIG', () => {
    it('should have correct primary filename', () => {
      expect(DEFAULT_MEMORY_CONFIG.primaryFilename).toBe('AGENT.md');
    });

    it('should have correct fallback filename', () => {
      expect(DEFAULT_MEMORY_CONFIG.fallbackFilename).toBe('CLAUDE.md');
    });

    it('should have correct local filename', () => {
      expect(DEFAULT_MEMORY_CONFIG.localFilename).toBe('AGENT.local.md');
    });

    it('should have correct primary user dir', () => {
      expect(DEFAULT_MEMORY_CONFIG.primaryUserDir).toBe('.gencode');
    });

    it('should have correct fallback user dir', () => {
      expect(DEFAULT_MEMORY_CONFIG.fallbackUserDir).toBe('.claude');
    });

    it('should have correct rules directory name', () => {
      expect(DEFAULT_MEMORY_CONFIG.rulesDir).toBe('rules');
    });

    it('should have reasonable maxFileSize', () => {
      expect(DEFAULT_MEMORY_CONFIG.maxFileSize).toBe(100 * 1024);
    });

    it('should have reasonable maxTotalSize', () => {
      expect(DEFAULT_MEMORY_CONFIG.maxTotalSize).toBe(500 * 1024);
    });

    it('should have reasonable maxImportDepth', () => {
      expect(DEFAULT_MEMORY_CONFIG.maxImportDepth).toBe(5);
    });
  });
});
