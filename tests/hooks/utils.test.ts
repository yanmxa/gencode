/**
 * Tests for hook utilities
 */

import { describe, it, expect } from '@jest/globals';
import {
  expandVariables,
  buildStdinPayload,
  validateHook,
  formatResult,
  sanitizePath,
} from '../../src/hooks/utils.js';
import type { HookContext, HookDefinition } from '../../src/hooks/types.js';

describe('Hook Utilities', () => {
  describe('expandVariables', () => {
    it('should expand tool name', () => {
      const context: HookContext = {
        event: 'PostToolUse',
        cwd: '/test',
        toolName: 'Write',
        timestamp: new Date(),
      };
      expect(expandVariables('Tool: $TOOL_NAME', context)).toBe('Tool: Write');
    });

    it('should expand file path', () => {
      const context: HookContext = {
        event: 'PostToolUse',
        cwd: '/test',
        toolName: 'Write',
        toolInput: { file_path: '/test/file.ts' },
        timestamp: new Date(),
      };
      expect(expandVariables('File: $FILE_PATH', context)).toBe('File: /test/file.ts');
    });

    it('should expand multiple variables', () => {
      const context: HookContext = {
        event: 'PostToolUse',
        cwd: '/home/user',
        toolName: 'Edit',
        toolInput: { file_path: 'src/app.ts' },
        sessionId: 'session-123',
        timestamp: new Date(),
      };
      expect(expandVariables('$TOOL_NAME: $FILE_PATH in $CWD ($SESSION_ID)', context)).toBe(
        'Edit: src/app.ts in /home/user (session-123)'
      );
    });

    it('should handle missing variables gracefully', () => {
      const context: HookContext = {
        event: 'Stop',
        cwd: '/test',
        timestamp: new Date(),
      };
      expect(expandVariables('$TOOL_NAME', context)).toBe('$TOOL_NAME');
      expect(expandVariables('$FILE_PATH', context)).toBe('$FILE_PATH');
    });
  });

  describe('buildStdinPayload', () => {
    it('should build complete payload', () => {
      const context: HookContext = {
        event: 'PostToolUse',
        cwd: '/test',
        toolName: 'Write',
        toolInput: { file_path: 'test.ts', content: 'hello' },
        sessionId: 'session-123',
        timestamp: new Date(),
      };

      const payload = buildStdinPayload(context);
      expect(payload).toEqual({
        session_id: 'session-123',
        cwd: '/test',
        hook_event_name: 'PostToolUse',
        tool_name: 'Write',
        tool_input: { file_path: 'test.ts', content: 'hello' },
      });
    });

    it('should handle missing optional fields', () => {
      const context: HookContext = {
        event: 'Stop',
        cwd: '/test',
        timestamp: new Date(),
      };

      const payload = buildStdinPayload(context);
      expect(payload).toEqual({
        session_id: '',
        cwd: '/test',
        hook_event_name: 'Stop',
        tool_name: '',
        tool_input: {},
      });
    });
  });

  describe('validateHook', () => {
    it('should validate valid command hooks', () => {
      const hook: HookDefinition = {
        type: 'command',
        command: 'echo test',
      };
      const result = validateHook(hook);
      expect(result.valid).toBe(true);
      expect(result.errors).toEqual([]);
    });

    it('should reject hooks without type', () => {
      const hook = {} as HookDefinition;
      const result = validateHook(hook);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Hook must have a type');
    });

    it('should reject command hooks without command', () => {
      const hook: HookDefinition = {
        type: 'command',
      };
      const result = validateHook(hook);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Command hook must have a command field');
    });

    it('should reject empty commands', () => {
      const hook: HookDefinition = {
        type: 'command',
        command: '   ',
      };
      const result = validateHook(hook);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Command cannot be empty');
    });

    it('should reject invalid timeout values', () => {
      const hook: HookDefinition = {
        type: 'command',
        command: 'echo test',
        timeout: -100,
      };
      const result = validateHook(hook);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Timeout must be positive');
    });

    it('should reject timeout > 10 minutes', () => {
      const hook: HookDefinition = {
        type: 'command',
        command: 'echo test',
        timeout: 700000,
      };
      const result = validateHook(hook);
      expect(result.valid).toBe(false);
      expect(result.errors).toContain('Timeout cannot exceed 10 minutes (600000ms)');
    });
  });

  describe('formatResult', () => {
    it('should format successful result', () => {
      const result = {
        success: true,
        output: 'Test output',
        exitCode: 0,
        durationMs: 123,
      };
      const formatted = formatResult(result);
      expect(formatted).toContain('✓ Success');
      expect(formatted).toContain('(exit 0)');
      expect(formatted).toContain('123ms');
      expect(formatted).toContain('Output: Test output');
    });

    it('should format blocked result', () => {
      const result = {
        success: false,
        blocked: true,
        error: 'Validation failed',
        exitCode: 2,
        durationMs: 50,
      };
      const formatted = formatResult(result);
      expect(formatted).toContain('🚫 BLOCKED');
      expect(formatted).toContain('(exit 2)');
      expect(formatted).toContain('Error: Validation failed');
    });

    it('should format failed result', () => {
      const result = {
        success: false,
        error: 'Command failed',
        exitCode: 1,
      };
      const formatted = formatResult(result);
      expect(formatted).toContain('✗ Failed');
      expect(formatted).toContain('(exit 1)');
      expect(formatted).toContain('Error: Command failed');
    });
  });

  describe('sanitizePath', () => {
    it('should allow safe paths', () => {
      expect(sanitizePath('src/file.ts')).toBe('src/file.ts');
      expect(sanitizePath('test/data.json')).toBe('test/data.json');
      expect(sanitizePath('/home/user/project/file.ts')).toBe('/home/user/project/file.ts');
    });

    it('should reject paths with ..', () => {
      expect(sanitizePath('../etc/passwd')).toBeNull();
      expect(sanitizePath('src/../../../etc/passwd')).toBeNull();
    });

    it('should reject sensitive directory paths', () => {
      expect(sanitizePath('/etc/passwd')).toBeNull();
      expect(sanitizePath('/var/log/system')).toBeNull();
      expect(sanitizePath('/sys/kernel')).toBeNull();
      expect(sanitizePath('/proc/self')).toBeNull();
      expect(sanitizePath('/dev/null')).toBeNull();
    });
  });
});
