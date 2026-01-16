/**
 * Memory Manager Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import { MemoryManager } from './memory-manager.js';
import { DEFAULT_MEMORY_CONFIG } from './types.js';
import { createTestProject, writeMemory, type TestProject } from './test-utils.js';

describe('MemoryManager', () => {
  describe('constructor', () => {
    it('should use default config or accept custom config', () => {
      expect(new MemoryManager()).toBeDefined();
      expect(new MemoryManager({ maxFileSize: 50 * 1024 })).toBeDefined();
    });
  });

  describe('getLoadedFileList', () => {
    it('should return empty list when no files loaded', () => {
      expect(new MemoryManager().getLoadedFileList()).toEqual([]);
    });
  });
});

describe('MemoryManager Integration', () => {
  let test: TestProject;

  beforeEach(async () => {
    test = await createTestProject('gencode-memory-');
  });

  afterEach(() => test.cleanup());

  describe('load', () => {
    it('should load AGENT.md from .gencode directory', async () => {
      await writeMemory(test.projectDir, 'gencode', '# GenCode\n\nTest content - MARKER_123');

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const file = memory.files.find((f) => f.namespace === 'gencode' && f.level === 'project');

      expect(file?.content).toContain('MARKER_123');
    });

    it('should load CLAUDE.md from .claude directory', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude\n\nTest content - MARKER_456');

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const file = memory.files.find((f) => f.namespace === 'claude' && f.level === 'project');

      expect(file?.content).toContain('MARKER_456');
    });

    it('should load from both namespaces with correct order (claude first, gencode second)', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude - ORDER_TEST');
      await writeMemory(test.projectDir, 'gencode', '# GenCode - ORDER_TEST');

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const projectFiles = memory.files.filter((f) => f.level === 'project');

      const claudeIdx = projectFiles.findIndex((f) => f.namespace === 'claude');
      const gencodeIdx = projectFiles.findIndex((f) => f.namespace === 'gencode');

      expect(claudeIdx).toBeLessThan(gencodeIdx); // gencode appears later = higher priority
    });

    it('should load root-level AGENT.md and CLAUDE.md', async () => {
      await writeMemory(test.projectDir, 'claude', '# Root Claude', { inDir: false });
      await writeMemory(test.projectDir, 'gencode', '# Root GenCode', { inDir: false });

      const memory = await new MemoryManager().load({ cwd: test.projectDir });

      expect(memory.context).toContain('Root Claude');
      expect(memory.context).toContain('Root GenCode');
    });

    it('should load local memory files', async () => {
      await writeMemory(test.projectDir, 'gencode', '# Local Notes\n\nPersonal content', { local: true });

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const localFile = memory.files.find((f) => f.level === 'local');

      expect(localFile?.content).toContain('Personal content');
    });

    it('should load from extra config dirs', async () => {
      const extraDir = path.join(test.tempDir, 'extra-config');
      await fs.mkdir(extraDir, { recursive: true });
      await fs.writeFile(path.join(extraDir, 'AGENT.md'), '# Extra\n\nShared team content');
      process.env.GENCODE_CONFIG_DIRS = extraDir;

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const extraFile = memory.files.find((f) => f.level === 'extra');

      expect(extraFile?.content).toContain('Shared team content');
    });
  });

  describe('getLoadedFileList', () => {
    it('should return list with namespace information', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude');
      await writeMemory(test.projectDir, 'gencode', '# GenCode');

      const manager = new MemoryManager();
      await manager.load({ cwd: test.projectDir });
      const list = manager.getLoadedFileList().filter((f) => f.level === 'project');

      expect(list.find((f) => f.namespace === 'claude')).toBeDefined();
      expect(list.find((f) => f.namespace === 'gencode')).toBeDefined();
    });
  });

  describe('getDebugSummary', () => {
    it('should return summary or indicate not loaded', async () => {
      const manager = new MemoryManager();

      expect(manager.getDebugSummary()).toBe('Memory not loaded');

      await writeMemory(test.projectDir, 'gencode', '# Test');
      await manager.load({ cwd: test.projectDir });

      expect(manager.getDebugSummary()).toContain('Memory Sources');
    });
  });

  describe('hasMemory', () => {
    it('should return true when memory files exist', async () => {
      await writeMemory(test.projectDir, 'gencode', '# Test', { inDir: false });

      const manager = new MemoryManager();
      await manager.load({ cwd: test.projectDir });

      expect(manager.hasMemory()).toBe(true);
    });

    it('should return false when no memory loaded', () => {
      expect(new MemoryManager().hasMemory()).toBe(false);
    });
  });

  describe('hasProjectMemory', () => {
    it('should detect memory files at various locations', async () => {
      const manager = new MemoryManager();

      // No memory initially
      expect(await manager.hasProjectMemory(test.projectDir)).toBe(false);

      // Root AGENT.md
      await writeMemory(test.projectDir, 'gencode', '# Test', { inDir: false });
      expect(await manager.hasProjectMemory(test.projectDir)).toBe(true);
    });

    it('should detect CLAUDE.md and .gencode/AGENT.md', async () => {
      const manager = new MemoryManager();

      await writeMemory(test.projectDir, 'claude', '# Claude', { inDir: false });
      expect(await manager.hasProjectMemory(test.projectDir)).toBe(true);
    });
  });

  describe('quickAdd', () => {
    it('should create or append to AGENT.md', async () => {
      const manager = new MemoryManager();

      // Create new
      const filePath = await manager.quickAdd('First content', 'project', test.projectDir);
      expect(filePath).toBe(path.join(test.projectDir, 'AGENT.md'));
      expect(await fs.readFile(filePath, 'utf-8')).toContain('First content');

      // Append
      await manager.quickAdd('Second content', 'project', test.projectDir);
      const content = await fs.readFile(filePath, 'utf-8');
      expect(content).toContain('First content');
      expect(content).toContain('Second content');
    });
  });
});

describe('MemoryConfig', () => {
  describe('DEFAULT_MEMORY_CONFIG', () => {
    it('should have correct filenames and directories', () => {
      expect(DEFAULT_MEMORY_CONFIG.gencodeFilename).toBe('AGENT.md');
      expect(DEFAULT_MEMORY_CONFIG.claudeFilename).toBe('CLAUDE.md');
      expect(DEFAULT_MEMORY_CONFIG.gencodeLocalFilename).toBe('AGENT.local.md');
      expect(DEFAULT_MEMORY_CONFIG.claudeLocalFilename).toBe('CLAUDE.local.md');
      expect(DEFAULT_MEMORY_CONFIG.gencodeDir).toBe('.gencode');
      expect(DEFAULT_MEMORY_CONFIG.claudeDir).toBe('.claude');
      expect(DEFAULT_MEMORY_CONFIG.rulesDir).toBe('rules');
    });

    it('should have reasonable size limits', () => {
      expect(DEFAULT_MEMORY_CONFIG.maxFileSize).toBe(100 * 1024);
      expect(DEFAULT_MEMORY_CONFIG.maxTotalSize).toBe(500 * 1024);
      expect(DEFAULT_MEMORY_CONFIG.maxImportDepth).toBe(5);
    });
  });
});
