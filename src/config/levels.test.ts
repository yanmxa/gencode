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

  it('should find .gen directory as project root', async () => {
    // Remove .git, add .gen
    await fs.rm(path.join(test.projectDir, '.git'), { recursive: true });
    await fs.mkdir(path.join(test.projectDir, '.gen'));
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
  const originalEnv = process.env.GEN_CONFIG;

  afterEach(() => {
    if (originalEnv === undefined) {
      delete process.env.GEN_CONFIG;
    } else {
      process.env.GEN_CONFIG = originalEnv;
    }
  });

  it('should return empty array when env var not set', () => {
    delete process.env.GEN_CONFIG;
    expect(parseExtraConfigDirs()).toEqual([]);
  });

  it('should parse single directory', () => {
    process.env.GEN_CONFIG = '/team/config';
    expect(parseExtraConfigDirs()).toEqual(['/team/config']);
  });

  it('should parse multiple directories', () => {
    process.env.GEN_CONFIG = '/team/config:/shared/rules';
    expect(parseExtraConfigDirs()).toEqual(['/team/config', '/shared/rules']);
  });

  it('should expand tilde to home directory', () => {
    process.env.GEN_CONFIG = '~/my-config';
    expect(parseExtraConfigDirs()[0]).toBe(path.join(os.homedir(), 'my-config'));
  });

  it('should trim whitespace and filter empty strings', () => {
    process.env.GEN_CONFIG = '  /path/one  : : /path/two  ';
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
    expect(userLevel?.paths.some((p) => p.namespace === 'gen')).toBe(true);
  });

  it('should include extra dirs when env var is set', async () => {
    process.env.GEN_CONFIG = '/team/config';
    const extraLevels = (await getConfigLevels(test.projectDir)).filter((l) => l.type === 'extra');

    expect(extraLevels.length).toBeGreaterThan(0);
  });

  it('should have claude before gencode in each level (for merge order)', async () => {
    for (const level of await getConfigLevels(test.projectDir)) {
      if (level.paths.length >= 2) {
        const claudeIdx = level.paths.findIndex((p) => p.namespace === 'claude');
        const gencodeIdx = level.paths.findIndex((p) => p.namespace === 'gen');
        if (claudeIdx !== -1 && gencodeIdx !== -1) {
          expect(claudeIdx).toBeLessThan(gencodeIdx);
        }
      }
    }
  });
});

describe('getPrimarySettingsDir', () => {
  it('should return ~/.gen for user level', () => {
    expect(getPrimarySettingsDir('user', '/project')).toBe(path.join(os.homedir(), '.gen'));
  });

  it('should return project/.gen for project and local levels', () => {
    expect(getPrimarySettingsDir('project', '/my/project')).toBe('/my/project/.gen');
    expect(getPrimarySettingsDir('local', '/my/project')).toBe('/my/project/.gen');
  });
});

describe('getSettingsFilePath', () => {
  it('should return correct paths for each level', () => {
    expect(getSettingsFilePath('user', '/project'))
      .toBe(path.join(os.homedir(), '.gen', 'settings.json'));
    expect(getSettingsFilePath('project', '/my/project'))
      .toBe('/my/project/.gen/settings.json');
    expect(getSettingsFilePath('local', '/my/project'))
      .toBe('/my/project/.gen/settings.local.json');
  });
});
