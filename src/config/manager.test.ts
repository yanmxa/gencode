/**
 * ConfigManager Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import { ConfigManager } from './manager.js';
import { createTestProject, writeSettings, type TestProject } from './test-utils.js';

describe('ConfigManager', () => {
  let test: TestProject;

  beforeEach(async () => {
    test = await createTestProject('gencode-config-');
  });

  afterEach(() => test.cleanup());

  describe('load', () => {
    it('should load settings from .gencode directory', async () => {
      await writeSettings(test.projectDir, 'gencode', { provider: 'anthropic', model: 'claude-sonnet' });

      const config = await new ConfigManager({ cwd: test.projectDir }).load();

      expect(config.settings.provider).toBe('anthropic');
      expect(config.settings.model).toBe('claude-sonnet');
    });

    it('should load settings from .claude directory', async () => {
      await writeSettings(test.projectDir, 'claude', { provider: 'openai', model: 'gpt-4' });

      const config = await new ConfigManager({ cwd: test.projectDir }).load();

      expect(config.settings.provider).toBe('openai');
      expect(config.settings.model).toBe('gpt-4');
    });

    it('should merge both .claude and .gencode with gencode winning', async () => {
      await writeSettings(test.projectDir, 'claude', { provider: 'openai', model: 'gpt-4', theme: 'dark' });
      await writeSettings(test.projectDir, 'gencode', { provider: 'anthropic' });

      const config = await new ConfigManager({ cwd: test.projectDir }).load();

      expect(config.settings.provider).toBe('anthropic'); // gencode wins
      expect(config.settings.model).toBe('gpt-4'); // preserved from claude
      expect(config.settings.theme).toBe('dark'); // preserved from claude
    });

    it('should concatenate permission arrays from both namespaces', async () => {
      await writeSettings(test.projectDir, 'claude', {
        permissions: { allow: ['Bash(git:*)'], deny: ['WebFetch'] },
      });
      await writeSettings(test.projectDir, 'gencode', {
        permissions: { allow: ['Bash(npm:*)'], deny: ['Bash(rm -rf:*)'] },
      });

      const { settings } = await new ConfigManager({ cwd: test.projectDir }).load();

      expect(settings.permissions?.allow).toContain('Bash(git:*)');
      expect(settings.permissions?.allow).toContain('Bash(npm:*)');
      expect(settings.permissions?.deny).toContain('WebFetch');
      expect(settings.permissions?.deny).toContain('Bash(rm -rf:*)');
    });

    it('should load local settings with higher priority', async () => {
      await writeSettings(test.projectDir, 'gencode', { model: 'claude-sonnet', theme: 'light' });
      await writeSettings(test.projectDir, 'gencode', { model: 'claude-opus' }, true);

      const { settings } = await new ConfigManager({ cwd: test.projectDir }).load();

      expect(settings.model).toBe('claude-opus'); // local wins
      expect(settings.theme).toBe('light'); // preserved
    });

    it('should load from extra config dirs', async () => {
      const extraDir = path.join(test.tempDir, 'team-config');
      await fs.mkdir(extraDir, { recursive: true });
      await fs.writeFile(path.join(extraDir, 'settings.json'), JSON.stringify({ teamSetting: 'enabled' }));
      process.env.GENCODE_CONFIG_DIRS = extraDir;

      const { settings } = await new ConfigManager({ cwd: test.projectDir }).load();

      expect(settings.teamSetting).toBe('enabled');
    });
  });

  describe('setCliArgs', () => {
    it('should apply CLI args with highest priority', async () => {
      await writeSettings(test.projectDir, 'gencode', { model: 'claude-sonnet', provider: 'anthropic' });

      const manager = new ConfigManager({ cwd: test.projectDir });
      manager.setCliArgs({ model: 'gpt-4o' });
      const { settings } = await manager.load();

      expect(settings.model).toBe('gpt-4o'); // CLI wins
      expect(settings.provider).toBe('anthropic'); // unchanged
    });
  });

  describe('saveToLevel', () => {
    it('should save to project and local levels', async () => {
      const manager = new ConfigManager({ cwd: test.projectDir });
      await manager.load();

      await manager.saveToLevel({ model: 'project-model' }, 'project');
      await manager.saveToLevel({ debug: true }, 'local');

      const projectContent = JSON.parse(await fs.readFile(
        path.join(test.projectDir, '.gencode', 'settings.json'), 'utf-8'
      ));
      const localContent = JSON.parse(await fs.readFile(
        path.join(test.projectDir, '.gencode', 'settings.local.json'), 'utf-8'
      ));

      expect(projectContent.model).toBe('project-model');
      expect(localContent.debug).toBe(true);
    });

    it('should merge with existing settings', async () => {
      await writeSettings(test.projectDir, 'gencode', { model: 'old', theme: 'dark' });

      const manager = new ConfigManager({ cwd: test.projectDir });
      await manager.load();
      await manager.saveToLevel({ model: 'new' }, 'project');

      const saved = JSON.parse(await fs.readFile(
        path.join(test.projectDir, '.gencode', 'settings.json'), 'utf-8'
      ));

      expect(saved.model).toBe('new');
      expect(saved.theme).toBe('dark'); // preserved
    });
  });

  describe('addPermissionRule', () => {
    it('should add allow and deny rules', async () => {
      const manager = new ConfigManager({ cwd: test.projectDir });
      await manager.load();

      await manager.addPermissionRule('Bash(npm:*)', 'allow', 'project');
      await manager.addPermissionRule('Bash(rm:*)', 'deny', 'project');

      const saved = JSON.parse(await fs.readFile(
        path.join(test.projectDir, '.gencode', 'settings.json'), 'utf-8'
      ));

      expect(saved.permissions?.allow).toContain('Bash(npm:*)');
      expect(saved.permissions?.deny).toContain('Bash(rm:*)');
    });
  });

  describe('getEffectivePermissions', () => {
    it('should return all permission lists', async () => {
      await writeSettings(test.projectDir, 'gencode', {
        permissions: { allow: ['A'], ask: ['B'], deny: ['C'] },
      });

      const manager = new ConfigManager({ cwd: test.projectDir });
      await manager.load();
      const perms = manager.getEffectivePermissions();

      expect(perms.allow).toContain('A');
      expect(perms.ask).toContain('B');
      expect(perms.deny).toContain('C');
    });
  });

  describe('isAllowed and shouldAsk', () => {
    it('should check permissions correctly', async () => {
      await writeSettings(test.projectDir, 'gencode', {
        permissions: { allow: ['Bash(git:*)'], deny: ['Bash(rm:*)'], ask: ['WebFetch'] },
      });

      const manager = new ConfigManager({ cwd: test.projectDir });
      await manager.load();

      expect(manager.isAllowed('Bash(git:status)')).toBe(true);
      expect(manager.isAllowed('Bash(rm:file)')).toBe(false);
      expect(manager.isAllowed('Unknown')).toBe(false);

      expect(manager.shouldAsk('Bash(git:status)')).toBe(false); // allowed
      expect(manager.shouldAsk('Bash(rm:file)')).toBe(false); // denied
      expect(manager.shouldAsk('WebFetch')).toBe(true);
      expect(manager.shouldAsk('Unknown')).toBe(true); // default ask
    });
  });

  describe('getSources', () => {
    it('should return all loaded sources', async () => {
      await writeSettings(test.projectDir, 'claude', { model: 'gpt-4' });
      await writeSettings(test.projectDir, 'gencode', { provider: 'anthropic' });

      const manager = new ConfigManager({ cwd: test.projectDir });
      await manager.load();
      const sources = manager.getSources();

      expect(sources.find((s) => s.namespace === 'claude')).toBeDefined();
      expect(sources.find((s) => s.namespace === 'gencode')).toBeDefined();
    });
  });

  describe('getDebugSummary', () => {
    it('should return summary or indicate not loaded', async () => {
      const manager = new ConfigManager({ cwd: test.projectDir });

      expect(manager.getDebugSummary()).toBe('Configuration not loaded');

      await writeSettings(test.projectDir, 'gencode', { model: 'test' });
      await manager.load();

      expect(manager.getDebugSummary()).toContain('Configuration Sources');
    });
  });
});
