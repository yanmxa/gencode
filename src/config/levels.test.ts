/**
 * Config Levels Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import {
  findProjectRoot,
  parseExtraConfigDirs,
  getConfigLevels,
  getPrimarySettingsDir,
  getSettingsFilePath,
} from './levels.js';
import { createTestProject, type TestProject } from './test-utils.js';

describe('findProjectRoot', () => {
  let test: TestProject;

  beforeEach(async () => {
    test = await createTestProject('gencode-levels-');
  });

  afterEach(() => test.cleanup());

  it('should find git root', async () => {
    const subDir = path.join(test.projectDir, 'src', 'components');
    await fs.mkdir(subDir, { recursive: true });

    expect(await findProjectRoot(subDir)).toBe(test.projectDir);
  });

  it('should find .gencode directory as project root', async () => {
    // Remove .git, add .gencode
    await fs.rm(path.join(test.projectDir, '.git'), { recursive: true });
    await fs.mkdir(path.join(test.projectDir, '.gencode'));
    const subDir = path.join(test.projectDir, 'src');
    await fs.mkdir(subDir, { recursive: true });

    expect(await findProjectRoot(subDir)).toBe(test.projectDir);
  });

  it('should find .claude directory as project root', async () => {
    await fs.rm(path.join(test.projectDir, '.git'), { recursive: true });
    await fs.mkdir(path.join(test.projectDir, '.claude'));
    const subDir = path.join(test.projectDir, 'lib');
    await fs.mkdir(subDir, { recursive: true });

    expect(await findProjectRoot(subDir)).toBe(test.projectDir);
  });

  it('should return cwd if no project markers found', async () => {
    const subDir = path.join(test.tempDir, 'random', 'path');
    await fs.mkdir(subDir, { recursive: true });

    expect(await findProjectRoot(subDir)).toBe(subDir);
  });
});

describe('parseExtraConfigDirs', () => {
  const originalEnv = process.env.GENCODE_CONFIG_DIRS;

  afterEach(() => {
    if (originalEnv === undefined) {
      delete process.env.GENCODE_CONFIG_DIRS;
    } else {
      process.env.GENCODE_CONFIG_DIRS = originalEnv;
    }
  });

  it('should return empty array when env var not set', () => {
    delete process.env.GENCODE_CONFIG_DIRS;
    expect(parseExtraConfigDirs()).toEqual([]);
  });

  it('should parse single directory', () => {
    process.env.GENCODE_CONFIG_DIRS = '/team/config';
    expect(parseExtraConfigDirs()).toEqual(['/team/config']);
  });

  it('should parse multiple directories', () => {
    process.env.GENCODE_CONFIG_DIRS = '/team/config:/shared/rules';
    expect(parseExtraConfigDirs()).toEqual(['/team/config', '/shared/rules']);
  });

  it('should expand tilde to home directory', () => {
    process.env.GENCODE_CONFIG_DIRS = '~/my-config';
    expect(parseExtraConfigDirs()[0]).toBe(path.join(os.homedir(), 'my-config'));
  });

  it('should trim whitespace and filter empty strings', () => {
    process.env.GENCODE_CONFIG_DIRS = '  /path/one  : : /path/two  ';
    expect(parseExtraConfigDirs()).toEqual(['/path/one', '/path/two']);
  });
});

describe('getConfigLevels', () => {
  let test: TestProject;

  beforeEach(async () => {
    test = await createTestProject('gencode-levels-');
  });

  afterEach(() => test.cleanup());

  it('should return levels in priority order', async () => {
    const types = (await getConfigLevels(test.projectDir)).map((l) => l.type);

    expect(types.indexOf('user')).toBeLessThan(types.indexOf('project'));
    expect(types.indexOf('project')).toBeLessThan(types.indexOf('local'));
    expect(types.indexOf('local')).toBeLessThan(types.indexOf('managed'));
  });

  it('should include both claude and gencode paths at each level', async () => {
    const userLevel = (await getConfigLevels(test.projectDir)).find((l) => l.type === 'user');

    expect(userLevel?.paths.length).toBe(2);
    expect(userLevel?.paths.some((p) => p.namespace === 'claude')).toBe(true);
    expect(userLevel?.paths.some((p) => p.namespace === 'gencode')).toBe(true);
  });

  it('should include extra dirs when env var is set', async () => {
    process.env.GENCODE_CONFIG_DIRS = '/team/config';
    const extraLevels = (await getConfigLevels(test.projectDir)).filter((l) => l.type === 'extra');

    expect(extraLevels.length).toBeGreaterThan(0);
  });

  it('should have claude before gencode in each level (for merge order)', async () => {
    for (const level of await getConfigLevels(test.projectDir)) {
      if (level.paths.length >= 2) {
        const claudeIdx = level.paths.findIndex((p) => p.namespace === 'claude');
        const gencodeIdx = level.paths.findIndex((p) => p.namespace === 'gencode');
        if (claudeIdx !== -1 && gencodeIdx !== -1) {
          expect(claudeIdx).toBeLessThan(gencodeIdx);
        }
      }
    }
  });
});

describe('getPrimarySettingsDir', () => {
  it('should return ~/.gencode for user level', () => {
    expect(getPrimarySettingsDir('user', '/project')).toBe(path.join(os.homedir(), '.gencode'));
  });

  it('should return project/.gencode for project and local levels', () => {
    expect(getPrimarySettingsDir('project', '/my/project')).toBe('/my/project/.gencode');
    expect(getPrimarySettingsDir('local', '/my/project')).toBe('/my/project/.gencode');
  });
});

describe('getSettingsFilePath', () => {
  it('should return correct paths for each level', () => {
    expect(getSettingsFilePath('user', '/project'))
      .toBe(path.join(os.homedir(), '.gencode', 'settings.json'));
    expect(getSettingsFilePath('project', '/my/project'))
      .toBe('/my/project/.gencode/settings.json');
    expect(getSettingsFilePath('local', '/my/project'))
      .toBe('/my/project/.gencode/settings.local.json');
  });
});
