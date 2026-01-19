/**
 * PermissionPersistence Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { PermissionPersistence } from './persistence.js';

describe('PermissionPersistence', () => {
  let tempDir: string;
  let projectDir: string;

  beforeEach(async () => {
    // Create temp directories for testing
    tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'gencode-perm-test-'));
    projectDir = path.join(tempDir, 'project');

    await fs.mkdir(projectDir, { recursive: true });
  });

  afterEach(async () => {
    // Cleanup temp directories
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  describe('addRule', () => {
    it('should add a rule to project scope', async () => {
      const projectPermDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectPermDir, { recursive: true });

      const persistence = new PermissionPersistence(projectDir);

      const rule = await persistence.addRule('WebSearch', 'auto', {
        scope: 'project',
      });

      expect(rule.id).toBeDefined();
      expect(rule.tool).toBe('WebSearch');
      expect(rule.mode).toBe('auto');
      expect(rule.scope).toBe('project');
    });

    it('should add a rule with pattern', async () => {
      const projectPermDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectPermDir, { recursive: true });

      const persistence = new PermissionPersistence(projectDir);

      const rule = await persistence.addRule('Bash', 'auto', {
        pattern: 'git add:*',
        scope: 'project',
        description: 'Test rule',
      });

      expect(rule.pattern).toBe('git add:*');
      expect(rule.description).toBe('Test rule');
    });
  });

  describe('getRules', () => {
    it('should return empty array for non-existent project file', async () => {
      const projectPermDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectPermDir, { recursive: true });

      const persistence = new PermissionPersistence(projectDir);

      const rules = await persistence.getRules('project');
      expect(rules).toEqual([]);
    });

    it('should return saved project rules', async () => {
      const projectPermDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectPermDir, { recursive: true });

      const persistence = new PermissionPersistence(projectDir);

      await persistence.addRule('Bash', 'auto', {
        pattern: 'npm test:*',
        scope: 'project',
      });

      const rules = await persistence.getRules('project');
      expect(rules.length).toBe(1);
      expect(rules[0].tool).toBe('Bash');
    });
  });

  describe('removeRule', () => {
    it('should remove a rule by ID', async () => {
      const projectPermDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectPermDir, { recursive: true });

      const persistence = new PermissionPersistence(projectDir);

      const rule = await persistence.addRule('Bash', 'auto', {
        pattern: 'npm:*',
        scope: 'project',
      });

      const removed = await persistence.removeRule(rule.id, 'project');
      expect(removed).toBe(true);

      const rules = await persistence.getRules('project');
      expect(rules.length).toBe(0);
    });

    it('should return false for non-existent rule', async () => {
      const projectPermDir = path.join(projectDir, '.claude');
      await fs.mkdir(projectPermDir, { recursive: true });

      const persistence = new PermissionPersistence(projectDir);

      const removed = await persistence.removeRule('non-existent-id', 'project');
      expect(removed).toBe(false);
    });
  });

  describe('parseSettingsPermissions', () => {
    it('should parse allow rules', () => {
      const persistence = new PermissionPersistence(projectDir);

      const rules = persistence.parseSettingsPermissions({
        allow: ['Bash(git add:*)', 'WebSearch'],
      });

      expect(rules.length).toBe(2);
      expect(rules[0].tool).toBe('Bash');
      expect(rules[0].mode).toBe('auto');
      expect(rules[0].pattern).toBe('git add:*');
      expect(rules[1].tool).toBe('WebSearch');
      expect(rules[1].mode).toBe('auto');
    });

    it('should parse ask rules', () => {
      const persistence = new PermissionPersistence(projectDir);

      const rules = persistence.parseSettingsPermissions({
        ask: ['Bash(npm run:*)'],
      });

      expect(rules.length).toBe(1);
      expect(rules[0].tool).toBe('Bash');
      expect(rules[0].mode).toBe('confirm');
      expect(rules[0].pattern).toBe('npm run:*');
    });

    it('should parse deny rules', () => {
      const persistence = new PermissionPersistence(projectDir);

      const rules = persistence.parseSettingsPermissions({
        deny: ['Bash(rm -rf:*)'],
      });

      expect(rules.length).toBe(1);
      expect(rules[0].tool).toBe('Bash');
      expect(rules[0].mode).toBe('deny');
      expect(rules[0].pattern).toBe('rm -rf:*');
    });

    it('should parse mixed rules', () => {
      const persistence = new PermissionPersistence(projectDir);

      const rules = persistence.parseSettingsPermissions({
        allow: ['Bash(git:*)'],
        ask: ['Bash(npm run:*)'],
        deny: ['Bash(rm -rf:*)'],
      });

      expect(rules.length).toBe(3);

      const allowRule = rules.find(r => r.pattern === 'git:*');
      const askRule = rules.find(r => r.pattern === 'npm run:*');
      const denyRule = rules.find(r => r.pattern === 'rm -rf:*');

      expect(allowRule?.mode).toBe('auto');
      expect(askRule?.mode).toBe('confirm');
      expect(denyRule?.mode).toBe('deny');
    });
  });

  describe('persistedToRuntime', () => {
    it('should convert persisted rules to runtime format', () => {
      const persistence = new PermissionPersistence(projectDir);

      const persisted = [
        {
          id: 'test-id',
          tool: 'Bash',
          pattern: 'git:*',
          mode: 'auto' as const,
          scope: 'global' as const,
          createdAt: new Date().toISOString(),
          description: 'Test',
        },
      ];

      const runtime = persistence.persistedToRuntime(persisted);

      expect(runtime.length).toBe(1);
      expect(runtime[0].tool).toBe('Bash');
      expect(runtime[0].mode).toBe('auto');
      expect(runtime[0].pattern).toBe('git:*');
    });
  });

  describe('clearRules', () => {
    it('should clear all rules for a scope', async () => {
      const persistence = new PermissionPersistence(projectDir);

      await persistence.addRule('Bash', 'auto', { scope: 'global' });
      await persistence.addRule('WebSearch', 'auto', { scope: 'global' });

      await persistence.clearRules('global');

      const rules = await persistence.getRules('global');
      expect(rules.length).toBe(0);
    });
  });
});
