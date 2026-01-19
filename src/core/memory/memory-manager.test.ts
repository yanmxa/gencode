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
    it('should load GEN.md from .gen directory', async () => {
      await writeMemory(test.projectDir, 'gen', '# GenCode\n\nTest content - MARKER_123');

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const file = memory.files.find((f) => f.namespace === 'gen' && f.level === 'project');

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
      await writeMemory(test.projectDir, 'gen', '# GenCode - ORDER_TEST');

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const projectFiles = memory.files.filter((f) => f.level === 'project');

      const claudeIdx = projectFiles.findIndex((f) => f.namespace === 'claude');
      const gencodeIdx = projectFiles.findIndex((f) => f.namespace === 'gen');

      expect(claudeIdx).toBeLessThan(gencodeIdx); // gencode appears later = higher priority
    });

    it('should load root-level GEN.md and CLAUDE.md', async () => {
      await writeMemory(test.projectDir, 'claude', '# Root Claude', { inDir: false });
      await writeMemory(test.projectDir, 'gen', '# Root GenCode', { inDir: false });

      const memory = await new MemoryManager().load({ cwd: test.projectDir, strategy: 'both' });

      expect(memory.context).toContain('Root Claude');
      expect(memory.context).toContain('Root GenCode');
    });

    it('should load local memory files', async () => {
      await writeMemory(test.projectDir, 'gen', '# Local Notes\n\nPersonal content', { local: true });

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const localFile = memory.files.find((f) => f.level === 'local');

      expect(localFile?.content).toContain('Personal content');
    });

    it('should load from extra config dirs', async () => {
      const extraDir = path.join(test.tempDir, 'extra-config');
      await fs.mkdir(extraDir, { recursive: true });
      await fs.writeFile(path.join(extraDir, 'GEN.md'), '# Extra\n\nShared team content');
      process.env.GEN_CONFIG = extraDir;

      const memory = await new MemoryManager().load({ cwd: test.projectDir });
      const extraFile = memory.files.find((f) => f.level === 'extra');

      expect(extraFile?.content).toContain('Shared team content');
    });
  });

  describe('getLoadedFileList', () => {
    it('should return list with namespace information', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude');
      await writeMemory(test.projectDir, 'gen', '# GenCode');

      const manager = new MemoryManager();
      await manager.load({ cwd: test.projectDir, strategy: 'both' });
      const list = manager.getLoadedFileList().filter((f) => f.level === 'project');

      expect(list.find((f) => f.namespace === 'claude')).toBeDefined();
      expect(list.find((f) => f.namespace === 'gen')).toBeDefined();
    });
  });

  describe('getDebugSummary', () => {
    it('should return summary or indicate not loaded', async () => {
      const manager = new MemoryManager();

      expect(manager.getDebugSummary()).toBe('Memory not loaded');

      await writeMemory(test.projectDir, 'gen', '# Test');
      await manager.load({ cwd: test.projectDir });

      expect(manager.getDebugSummary()).toContain('Memory Sources');
    });
  });

  describe('hasMemory', () => {
    it('should return true when memory files exist', async () => {
      await writeMemory(test.projectDir, 'gen', '# Test', { inDir: false });

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

      // Root GEN.md
      await writeMemory(test.projectDir, 'gen', '# Test', { inDir: false });
      expect(await manager.hasProjectMemory(test.projectDir)).toBe(true);
    });

    it('should detect CLAUDE.md and .gen/GEN.md', async () => {
      const manager = new MemoryManager();

      await writeMemory(test.projectDir, 'claude', '# Claude', { inDir: false });
      expect(await manager.hasProjectMemory(test.projectDir)).toBe(true);
    });
  });

  describe('quickAdd', () => {
    it('should create or append to GEN.md', async () => {
      const manager = new MemoryManager();

      // Create new
      const filePath = await manager.quickAdd('First content', 'project', test.projectDir);
      expect(filePath).toBe(path.join(test.projectDir, 'GEN.md'));
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
      expect(DEFAULT_MEMORY_CONFIG.genFilename).toBe('GEN.md');
      expect(DEFAULT_MEMORY_CONFIG.claudeFilename).toBe('CLAUDE.md');
      expect(DEFAULT_MEMORY_CONFIG.genLocalFilename).toBe('GEN.local.md');
      expect(DEFAULT_MEMORY_CONFIG.claudeLocalFilename).toBe('CLAUDE.local.md');
      expect(DEFAULT_MEMORY_CONFIG.genDir).toBe('.gen');
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

describe('Memory Merge Strategies', () => {
  let test: TestProject;

  beforeEach(async () => {
    test = await createTestProject('gencode-strategy-');
  });

  afterEach(() => test.cleanup());

  describe('fallback strategy', () => {
    it('should load only GEN.md when both files exist', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude Content');
      await writeMemory(test.projectDir, 'gen', '# GenCode Content');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'fallback',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(1);
      expect(projectFiles[0].namespace).toBe('gen');
      expect(projectFiles[0].content).toContain('GenCode Content');

      // Check that CLAUDE.md was skipped
      expect(memory.skippedFiles.some((f) => f.includes('CLAUDE.md'))).toBe(true);
    });

    it('should load CLAUDE.md when only CLAUDE.md exists', async () => {
      await writeMemory(test.projectDir, 'claude', '# Only Claude');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'fallback',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(1);
      expect(projectFiles[0].namespace).toBe('claude');
      expect(projectFiles[0].content).toContain('Only Claude');

      // No project-level files should be skipped (GEN.md doesn't exist at project level)
      const projectSkipped = memory.skippedFiles.filter((f) => f.includes(test.projectDir));
      expect(projectSkipped.length).toBe(0);
    });

    it('should load GEN.md when only GEN.md exists', async () => {
      await writeMemory(test.projectDir, 'gen', '# Only GenCode');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'fallback',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(1);
      expect(projectFiles[0].namespace).toBe('gen');
      expect(projectFiles[0].content).toContain('Only GenCode');
    });

    it('should load nothing when neither file exists', async () => {
      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'fallback',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(0);
    });
  });

  describe('both strategy', () => {
    it('should load both CLAUDE.md and GEN.md when they exist', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude Content');
      await writeMemory(test.projectDir, 'gen', '# GenCode Content');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'both',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(2);

      const claudeFile = projectFiles.find((f) => f.namespace === 'claude');
      const gencodeFile = projectFiles.find((f) => f.namespace === 'gen');

      expect(claudeFile?.content).toContain('Claude Content');
      expect(gencodeFile?.content).toContain('GenCode Content');

      // No files should be skipped in 'both' mode
      expect(memory.skippedFiles.length).toBe(0);
    });

    it('should load whatever exists when only one file exists', async () => {
      await writeMemory(test.projectDir, 'gen', '# Only GenCode');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'both',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(1);
      expect(projectFiles[0].content).toContain('Only GenCode');
    });
  });

  describe('gen-only strategy', () => {
    it('should load only GEN.md even when both exist', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude Content');
      await writeMemory(test.projectDir, 'gen', '# GenCode Content');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'gen-only',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(1);
      expect(projectFiles[0].namespace).toBe('gen');
      expect(projectFiles[0].content).toContain('GenCode Content');

      // CLAUDE.md should be marked as skipped
      expect(memory.skippedFiles.some((f) => f.includes('CLAUDE.md'))).toBe(true);
    });

    it('should load nothing when only CLAUDE.md exists', async () => {
      await writeMemory(test.projectDir, 'claude', '# Only Claude');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'gen-only',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(0);
    });
  });

  describe('claude-only strategy', () => {
    it('should load only CLAUDE.md even when both exist', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude Content');
      await writeMemory(test.projectDir, 'gen', '# GenCode Content');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'claude-only',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(1);
      expect(projectFiles[0].namespace).toBe('claude');
      expect(projectFiles[0].content).toContain('Claude Content');

      // GEN.md should be marked as skipped
      expect(memory.skippedFiles.some((f) => f.includes('GEN.md'))).toBe(true);
    });

    it('should load nothing when only GEN.md exists', async () => {
      await writeMemory(test.projectDir, 'gen', '# Only GenCode');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        strategy: 'claude-only',
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(0);
    });
  });

  describe('default strategy', () => {
    it('should default to fallback when no strategy specified', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude Content');
      await writeMemory(test.projectDir, 'gen', '# GenCode Content');

      const memory = await new MemoryManager().load({
        cwd: test.projectDir,
        // No strategy specified
      });

      const projectFiles = memory.files.filter((f) => f.level === 'project');
      expect(projectFiles.length).toBe(1);
      expect(projectFiles[0].namespace).toBe('gen');
      expect(memory.skippedFiles.some((f) => f.includes('CLAUDE.md'))).toBe(true);
    });
  });

  describe('getVerboseSummary', () => {
    it('should show strategy and skipped files', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude');
      await writeMemory(test.projectDir, 'gen', '# GenCode');

      const manager = new MemoryManager();
      await manager.load({ cwd: test.projectDir, strategy: 'fallback' });

      const summary = manager.getVerboseSummary('fallback');
      expect(summary).toContain('[Memory] Strategy: fallback');
      expect(summary).toContain('Skipped:');
      expect(summary).toContain('CLAUDE.md');
      expect(summary).toContain('skipped)');
    });

    it('should show no skipped files in both mode', async () => {
      await writeMemory(test.projectDir, 'claude', '# Claude');
      await writeMemory(test.projectDir, 'gen', '# GenCode');

      const manager = new MemoryManager();
      await manager.load({ cwd: test.projectDir, strategy: 'both' });

      const summary = manager.getVerboseSummary('both');
      expect(summary).toContain('[Memory] Strategy: both');
      expect(summary).toContain('files loaded, 0 skipped');
    });
  });
});
