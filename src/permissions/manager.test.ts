/**
 * PermissionManager Tests
 */

import { jest } from '@jest/globals';
import { PermissionManager } from './manager.js';
import type { PermissionConfig, ApprovalAction } from './types.js';

describe('PermissionManager', () => {
  describe('default behavior', () => {
    it('should auto-approve read-only tools by default', async () => {
      const manager = new PermissionManager();

      const readDecision = await manager.checkPermission({
        tool: 'Read',
        input: { file_path: '/test/file.ts' },
      });
      expect(readDecision.allowed).toBe(true);
      expect(readDecision.requiresConfirmation).toBe(false);

      const globDecision = await manager.checkPermission({
        tool: 'Glob',
        input: { pattern: '*.ts' },
      });
      expect(globDecision.allowed).toBe(true);

      const grepDecision = await manager.checkPermission({
        tool: 'Grep',
        input: { pattern: 'test' },
      });
      expect(grepDecision.allowed).toBe(true);
    });

    it('should require confirmation for write operations by default', async () => {
      const manager = new PermissionManager();

      const bashDecision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'echo hello' },
      });
      expect(bashDecision.allowed).toBe(false);
      expect(bashDecision.requiresConfirmation).toBe(true);

      const writeDecision = await manager.checkPermission({
        tool: 'Write',
        input: { file_path: '/test/file.ts', content: 'test' },
      });
      expect(writeDecision.allowed).toBe(false);
      expect(writeDecision.requiresConfirmation).toBe(true);
    });
  });

  describe('allow rules', () => {
    it('should auto-approve operations matching allow rules', async () => {
      const manager = new PermissionManager({
        config: {
          defaultMode: 'confirm',
          rules: [
            { tool: 'Bash', mode: 'auto', pattern: 'git add:*' },
          ],
          allowedPrompts: [],
        },
      });

      const decision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'git add .' },
      });
      expect(decision.allowed).toBe(true);
      expect(decision.requiresConfirmation).toBe(false);
    });

    it('should still require confirmation for non-matching operations', async () => {
      const manager = new PermissionManager({
        config: {
          defaultMode: 'confirm',
          rules: [
            { tool: 'Bash', mode: 'auto', pattern: 'git add:*' },
          ],
          allowedPrompts: [],
        },
      });

      const decision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'rm -rf /' },
      });
      expect(decision.allowed).toBe(false);
      expect(decision.requiresConfirmation).toBe(true);
    });
  });

  describe('deny rules', () => {
    it('should block operations matching deny rules', async () => {
      const manager = new PermissionManager({
        config: {
          defaultMode: 'confirm',
          rules: [
            { tool: 'Bash', mode: 'deny', pattern: 'rm -rf:*' },
          ],
          allowedPrompts: [],
        },
      });

      const decision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'rm -rf /' },
      });
      expect(decision.allowed).toBe(false);
      expect(decision.requiresConfirmation).toBe(false);
    });

    it('should prioritize deny over allow rules', async () => {
      const manager = new PermissionManager({
        config: {
          defaultMode: 'confirm',
          rules: [
            { tool: 'Bash', mode: 'auto', pattern: 'rm:*' },
            { tool: 'Bash', mode: 'deny', pattern: 'rm -rf:*' },
          ],
          allowedPrompts: [],
        },
      });

      const decision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'rm -rf /' },
      });
      expect(decision.allowed).toBe(false);
      expect(decision.requiresConfirmation).toBe(false);
    });
  });

  describe('session caching', () => {
    it('should cache session approvals', async () => {
      const manager = new PermissionManager();

      manager.approveForSession('Bash', 'npm test');

      const decision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'npm test' },
      });
      expect(decision.allowed).toBe(true);
    });

    it('should clear session cache', async () => {
      const manager = new PermissionManager();

      manager.approveForSession('Bash', 'npm test');
      manager.clearSessionApprovals();

      const decision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'npm test' },
      });
      expect(decision.requiresConfirmation).toBe(true);
    });
  });

  describe('prompt-based permissions', () => {
    it('should approve operations matching allowed prompts', async () => {
      const manager = new PermissionManager();
      manager.addAllowedPrompts([
        { tool: 'Bash', prompt: 'run tests' },
      ]);

      const decision = await manager.checkPermission({
        tool: 'Bash',
        input: { command: 'npm test' },
      });
      expect(decision.allowed).toBe(true);
    });

    it('should clear allowed prompts', () => {
      const manager = new PermissionManager();
      manager.addAllowedPrompts([
        { tool: 'Bash', prompt: 'run tests' },
      ]);
      manager.clearAllowedPrompts();

      expect(manager.getAllowedPrompts()).toHaveLength(0);
    });
  });

  describe('confirmation callback', () => {
    it('should call enhanced confirm callback when confirmation required', async () => {
      const manager = new PermissionManager();
      const mockCallback = jest.fn().mockResolvedValue('allow_once' as ApprovalAction);
      manager.setConfirmCallback(mockCallback);

      const result = await manager.requestPermission('Bash', { command: 'echo hello' });

      expect(mockCallback).toHaveBeenCalledWith(
        'Bash',
        { command: 'echo hello' },
        expect.any(Array)
      );
      expect(result).toBe(true);
    });

    it('should call saveRuleCallback when allow_always is selected', async () => {
      const manager = new PermissionManager();
      const mockSaveCallback = jest.fn().mockResolvedValue(undefined);
      const mockConfirmCallback = jest.fn().mockResolvedValue('allow_always' as ApprovalAction);

      manager.setConfirmCallback(mockConfirmCallback);
      manager.setSaveRuleCallback(mockSaveCallback);

      await manager.requestPermission('Bash', { command: 'npm run build' });

      expect(mockSaveCallback).toHaveBeenCalledWith('Bash', 'npm run:*');
    });
  });

  describe('getModeForTool', () => {
    it('should return auto for read-only tools', () => {
      const manager = new PermissionManager();

      expect(manager.getModeForTool('Read')).toBe('auto');
      expect(manager.getModeForTool('Glob')).toBe('auto');
      expect(manager.getModeForTool('Grep')).toBe('auto');
    });

    it('should return confirm for write tools', () => {
      const manager = new PermissionManager();

      expect(manager.getModeForTool('Bash')).toBe('confirm');
      expect(manager.getModeForTool('Write')).toBe('confirm');
      expect(manager.getModeForTool('Edit')).toBe('confirm');
    });
  });

  describe('audit logging', () => {
    it('should log permission decisions', async () => {
      const manager = new PermissionManager({ enableAudit: true });

      await manager.checkPermission({
        tool: 'Read',
        input: { file_path: '/test/file.ts' },
      });

      const auditLog = manager.getAuditLog(10);
      expect(auditLog.length).toBeGreaterThan(0);
      expect(auditLog[0].tool).toBe('Read');
    });

    it('should provide audit stats', async () => {
      const manager = new PermissionManager({ enableAudit: true });

      await manager.checkPermission({
        tool: 'Read',
        input: { file_path: '/test/file.ts' },
      });

      const stats = manager.getAuditStats();
      expect(stats.allowed).toBeGreaterThanOrEqual(1);
    });
  });
});
