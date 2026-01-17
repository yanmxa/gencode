/**
 * Config Loader Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import {
  loadAllSources,
  loadSourcesByLevel,
  loadProjectSettings,
  getExistingConfigFiles,
} from './loader.js';
import { createTestProject, writeSettings, type TestProject } from './test-utils.js';

describe('Config Loader', () => {
  let test: TestProject;

  beforeEach(async () => {
    test = await createTestProject('gencode-loader-');
  });

  afterEach(() => test.cleanup());

  describe('loadAllSources', () => {
    it('should load settings from project .claude directory', async () => {
      await writeSettings(test.projectDir, 'claude', { provider: 'openai' });
      const sources = await loadAllSources(test.projectDir);

      const source = sources.find((s) => s.level === 'project' && s.namespace === 'claude');
      expect(source?.settings.provider).toBe('openai');
    });

    it('should load settings from project .gen directory', async () => {
      await writeSettings(test.projectDir, 'gen', { provider: 'anthropic' });
      const sources = await loadAllSources(test.projectDir);

      const source = sources.find((s) => s.level === 'project' && s.namespace === 'gen');
      expect(source?.settings.provider).toBe('anthropic');
    });

    it('should load both .claude and .gen settings at same level', async () => {
      await writeSettings(test.projectDir, 'claude', { provider: 'openai', model: 'gpt-4' });
      await writeSettings(test.projectDir, 'gen', { provider: 'anthropic' });

      const sources = await loadAllSources(test.projectDir);
      const claude = sources.find((s) => s.level === 'project' && s.namespace === 'claude');
      const gencode = sources.find((s) => s.level === 'project' && s.namespace === 'gen');

      expect(claude?.settings.provider).toBe('openai');
      expect(gencode?.settings.provider).toBe('anthropic');
    });

    it('should load local settings', async () => {
      await writeSettings(test.projectDir, 'gen', { alwaysThinkingEnabled: true }, true);
      const sources = await loadAllSources(test.projectDir);

      const local = sources.find((s) => s.level === 'local');
      expect(local?.settings.alwaysThinkingEnabled).toBe(true);
    });

    it('should load from extra config dirs', async () => {
      const extraDir = path.join(test.tempDir, 'extra-config');
      await fs.mkdir(extraDir, { recursive: true });
      await fs.writeFile(path.join(extraDir, 'settings.json'), JSON.stringify({ theme: 'dark' }));
      process.env.GEN_CONFIG = extraDir;

      const sources = await loadAllSources(test.projectDir);
      const extra = sources.find((s) => s.level === 'extra');
      expect(extra?.settings.theme).toBe('dark');
    });
  });

  describe('loadSourcesByLevel', () => {
    it('should group sources by level', async () => {
      await writeSettings(test.projectDir, 'claude', { model: 'gpt-4' });
      await writeSettings(test.projectDir, 'gen', { model: 'claude-sonnet' });

      const sourcesByLevel = await loadSourcesByLevel(test.projectDir);

      expect(sourcesByLevel.has('project')).toBe(true);
      expect(sourcesByLevel.get('project')?.length).toBe(2);
    });
  });

  describe('loadProjectSettings', () => {
    it('should only load project-level settings', async () => {
      await writeSettings(test.projectDir, 'gen', { model: 'claude-sonnet' });
      await writeSettings(test.projectDir, 'gen', { theme: 'dark' }, true);

      const sources = await loadProjectSettings(test.projectDir);

      expect(sources.length).toBe(1);
      expect(sources[0].level).toBe('project');
      expect(sources[0].settings.model).toBe('claude-sonnet');
    });
  });

  describe('getExistingConfigFiles', () => {
    it('should list all existing config files', async () => {
      await writeSettings(test.projectDir, 'claude', {});
      await writeSettings(test.projectDir, 'gen', {});
      await writeSettings(test.projectDir, 'gen', {}, true);

      const files = await getExistingConfigFiles(test.projectDir);

      expect(files.length).toBeGreaterThanOrEqual(3);
      expect(files.some((f) => f.includes('.claude/settings.json'))).toBe(true);
      expect(files.some((f) => f.includes('.gen/settings.json'))).toBe(true);
      expect(files.some((f) => f.includes('settings.local.json'))).toBe(true);
    });

    it('should return array when no config files exist', async () => {
      const emptyDir = path.join(test.tempDir, 'empty');
      await fs.mkdir(emptyDir, { recursive: true });

      expect(Array.isArray(await getExistingConfigFiles(emptyDir))).toBe(true);
    });
  });
});
