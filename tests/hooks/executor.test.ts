/**
 * Tests for hook executor
 */

import { describe, it, expect } from '@jest/globals';
import { executeHook } from '../../src/hooks/executor.js';
import type { HookDefinition, HookContext } from '../../src/hooks/types.js';

describe('executeHook', () => {
  let context: HookContext;

  beforeEach(() => {
    context = {
      event: 'PostToolUse',
      cwd: process.cwd(),
      toolName: 'Write',
      toolInput: { file_path: 'test.ts' },
      timestamp: new Date(),
    };
  });

  it('should execute successful command hook', async () => {
    const hook: HookDefinition = {
      type: 'command',
      command: 'exit 0',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(true);
    expect(result.exitCode).toBe(0);
    expect(result.blocked).toBe(false);
  });

  it('should execute command with output', async () => {
    const hook: HookDefinition = {
      type: 'command',
      command: 'echo "test output"',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(true);
    expect(result.output).toContain('test output');
  });

  it('should handle command failure', async () => {
    const hook: HookDefinition = {
      type: 'command',
      command: 'exit 1',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(false);
    expect(result.exitCode).toBe(1);
    expect(result.blocked).toBe(false);
  });

  it('should detect blocking hook (exit code 2)', async () => {
    const hook: HookDefinition = {
      type: 'command',
      command: 'exit 2',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(false);
    expect(result.exitCode).toBe(2);
    expect(result.blocked).toBe(true);
  });

  it('should handle timeout', async () => {
    const hook: HookDefinition = {
      type: 'command',
      command: 'sleep 10',
      timeout: 100, // 100ms timeout
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(false);
    expect(result.error).toContain('timed out');
  }, 10000); // 10 second test timeout

  it('should reject prompt hooks (not yet implemented)', async () => {
    const hook: HookDefinition = {
      type: 'prompt',
      prompt: 'Should this proceed?',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(false);
    expect(result.error).toContain('not yet implemented');
  });

  it('should pass environment variables', async () => {
    const hook: HookDefinition = {
      type: 'command',
      command: 'echo "$TOOL_NAME"',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(true);
    expect(result.output).toContain('Write');
  });

  it('should pass file path variable', async () => {
    const hook: HookDefinition = {
      type: 'command',
      command: 'echo "$FILE_PATH"',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(true);
    expect(result.output).toContain('test.ts');
  });

  it('should handle missing command field', async () => {
    const hook: HookDefinition = {
      type: 'command',
    };

    const result = await executeHook(hook, context);

    expect(result.success).toBe(false);
    expect(result.error).toContain('missing command');
  });
});
