/**
 * SettingsManager Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { SettingsManager } from './manager.js';
import type { Settings } from './types.js';

describe('SettingsManager', () => {
  let tempDir: string;
  let globalDir: string;
  let projectDir: string;

  beforeEach(async () => {
    // Create temp directories for testing
    tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'gencode-test-'));
    globalDir = path.join(tempDir, 'global');
    projectDir = path.join(tempDir, 'project');

    await fs.mkdir(globalDir, { recursive: true });
    await fs.mkdir(projectDir, { recursive: true });
  });

  afterEach(async () => {
    // Cleanup temp directories
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  describe('load', () => {
    it('should load settings from a single file', async () => {
      const settingsPath = path.join(globalDir, 'settings.json');
      await fs.writeFile(settingsPath, JSON.stringify({
        model: 'gpt-4o',
        provider: 'openai',
      }));

      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });
      const settings = await manager.load();

      expect(settings.model).toBe('gpt-4o');
      expect(settings.provider).toBe('openai');
    });

    it('should merge settings from multiple levels', async () => {
      // Global settings
      const globalSettings = path.join(globalDir, 'settings.json');
      await fs.writeFile(globalSettings, JSON.stringify({
        model: 'gpt-4o',
        provider: 'openai',
      }));

      // Project settings directory
      const projectSettingsDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectSettingsDir, { recursive: true });

      // Project settings
      const projectSettings = path.join(projectSettingsDir, 'settings.json');
      await fs.writeFile(projectSettings, JSON.stringify({
        model: 'claude-sonnet',
      }));

      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });
      const settings = await manager.load();

      // Model should be overridden by project
      expect(settings.model).toBe('claude-sonnet');
      // Provider should be inherited from global
      expect(settings.provider).toBe('openai');
    });

    it('should concatenate array fields (permissions)', async () => {
      // Global settings
      const globalSettings = path.join(globalDir, 'settings.json');
      await fs.writeFile(globalSettings, JSON.stringify({
        permissions: {
          allow: ['Bash(git:*)'],
        },
      }));

      // Project settings directory
      const projectSettingsDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectSettingsDir, { recursive: true });

      // Project local settings
      const projectLocal = path.join(projectSettingsDir, 'settings.local.json');
      await fs.writeFile(projectLocal, JSON.stringify({
        permissions: {
          allow: ['WebSearch'],
        },
      }));

      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });
      const settings = await manager.load();

      // Arrays should be concatenated
      expect(settings.permissions?.allow).toContain('Bash(git:*)');
      expect(settings.permissions?.allow).toContain('WebSearch');
      expect(settings.permissions?.allow?.length).toBe(2);
    });
  });

  describe('saveToLevel', () => {
    it('should save to global level', async () => {
      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });

      await manager.saveToLevel({ model: 'test-model' }, 'global');

      const content = await fs.readFile(
        path.join(globalDir, 'settings.json'),
        'utf-8'
      );
      const saved = JSON.parse(content);
      expect(saved.model).toBe('test-model');
    });

    it('should save to project local level', async () => {
      // Create project settings directory
      const projectSettingsDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectSettingsDir, { recursive: true });

      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });

      await manager.saveToLevel({ model: 'local-model' }, 'local');

      const content = await fs.readFile(
        path.join(projectSettingsDir, 'settings.local.json'),
        'utf-8'
      );
      const saved = JSON.parse(content);
      expect(saved.model).toBe('local-model');
    });
  });

  describe('addPermissionRule', () => {
    it('should add allow rule to local level', async () => {
      // Create project settings directory
      const projectSettingsDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectSettingsDir, { recursive: true });

      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });

      await manager.addPermissionRule('Bash(npm test:*)', 'allow', 'local');

      const content = await fs.readFile(
        path.join(projectSettingsDir, 'settings.local.json'),
        'utf-8'
      );
      const saved = JSON.parse(content);
      expect(saved.permissions?.allow).toContain('Bash(npm test:*)');
    });

    it('should add deny rule', async () => {
      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });

      await manager.addPermissionRule('Bash(rm -rf:*)', 'deny', 'global');

      const content = await fs.readFile(
        path.join(globalDir, 'settings.json'),
        'utf-8'
      );
      const saved = JSON.parse(content);
      expect(saved.permissions?.deny).toContain('Bash(rm -rf:*)');
    });
  });

  describe('directory fallback', () => {
    it('should use .claude as primary directory', async () => {
      // Create .claude directory (primary)
      const claudeDir = path.join(projectDir, '.claude');
      await fs.mkdir(claudeDir, { recursive: true });

      // Create settings in .claude
      await fs.writeFile(
        path.join(claudeDir, 'settings.json'),
        JSON.stringify({ model: 'claude-model' })
      );

      // Also create .gencode directory (fallback)
      const gencodeDir = path.join(projectDir, '.gencode');
      await fs.mkdir(gencodeDir, { recursive: true });
      await fs.writeFile(
        path.join(gencodeDir, 'settings.json'),
        JSON.stringify({ model: 'gencode-model' })
      );

      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });
      const settings = await manager.load();

      // Should use .claude (primary)
      expect(settings.model).toBe('claude-model');
    });

    it('should use .gencode when explicitly created first', async () => {
      // Create .gencode directory first, then .claude (to test priority)
      const gencodeDir = path.join(projectDir, '.gencode');
      await fs.mkdir(gencodeDir, { recursive: true });
      await fs.writeFile(
        path.join(gencodeDir, 'settings.json'),
        JSON.stringify({ model: 'gencode-model' })
      );

      // Now create .claude (primary) with different model
      const claudeDir = path.join(projectDir, '.claude');
      await fs.mkdir(claudeDir, { recursive: true });
      await fs.writeFile(
        path.join(claudeDir, 'settings.json'),
        JSON.stringify({ model: 'claude-model' })
      );

      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });

      // .claude is primary, so it should be used
      expect(manager.getProjectDir()).toContain('.claude');

      const settings = await manager.load();

      // Should use .claude (primary) over .gencode
      expect(settings.model).toBe('claude-model');
    });
  });

  describe('getCwd', () => {
    it('should return the working directory', () => {
      const manager = new SettingsManager({
        settingsDir: globalDir,
        cwd: projectDir,
      });

      expect(manager.getCwd()).toBe(projectDir);
    });
  });
});
