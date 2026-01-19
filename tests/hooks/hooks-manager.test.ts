/**
 * Tests for HooksManager
 */

import { describe, it, expect, beforeEach } from '@jest/globals';
import {
  HooksManager,
  hasBlockingResult,
  getFirstBlockingResult,
  allSucceeded,
  getTotalDuration,
} from '../../src/hooks/hooks-manager.js';
import type { HooksConfig, HookContext, HookResult } from '../../src/hooks/types.js';

describe('HooksManager', () => {
  let manager: HooksManager;
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

  describe('constructor', () => {
    it('should create manager with empty config', () => {
      manager = new HooksManager();
      expect(manager.getConfig()).toEqual({});
    });

    it('should create manager with provided config', () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: 'Write',
            hooks: [{ type: 'command', command: 'echo test' }],
          },
        ],
      };
      manager = new HooksManager(config);
      expect(manager.getConfig()).toEqual(config);
    });
  });

  describe('hasHooks', () => {
    it('should return false for events without hooks', () => {
      manager = new HooksManager({});
      expect(manager.hasHooks('PostToolUse')).toBe(false);
    });

    it('should return true for events with hooks', () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: 'Write',
            hooks: [{ type: 'command', command: 'echo test' }],
          },
        ],
      };
      manager = new HooksManager(config);
      expect(manager.hasHooks('PostToolUse')).toBe(true);
    });
  });

  describe('getMatchers', () => {
    it('should return empty array for events without hooks', () => {
      manager = new HooksManager({});
      expect(manager.getMatchers('PostToolUse')).toEqual([]);
    });

    it('should return matchers for configured events', () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: 'Write',
            hooks: [{ type: 'command', command: 'echo test' }],
          },
        ],
      };
      manager = new HooksManager(config);
      const matchers = manager.getMatchers('PostToolUse');
      expect(matchers).toHaveLength(1);
      expect(matchers[0].matcher).toBe('Write');
    });
  });

  describe('setConfig/getConfig', () => {
    it('should update configuration', () => {
      manager = new HooksManager({});
      const newConfig: HooksConfig = {
        Stop: [
          {
            hooks: [{ type: 'command', command: 'echo done' }],
          },
        ],
      };
      manager.setConfig(newConfig);
      expect(manager.getConfig()).toEqual(newConfig);
    });
  });

  describe('trigger', () => {
    it('should return empty array when no hooks match', async () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: 'Read',
            hooks: [{ type: 'command', command: 'echo test' }],
          },
        ],
      };
      manager = new HooksManager(config);

      const results = await manager.trigger('PostToolUse', {
        ...context,
        toolName: 'Write', // Different tool
      });

      expect(results).toEqual([]);
    });

    it('should execute matching hooks', async () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: 'Write',
            hooks: [
              { type: 'command', command: 'exit 0' },
              { type: 'command', command: 'exit 0' },
            ],
          },
        ],
      };
      manager = new HooksManager(config);

      const results = await manager.trigger('PostToolUse', context);

      expect(results).toHaveLength(2);
      expect(results[0].success).toBe(true);
      expect(results[1].success).toBe(true);
    });

    it('should match wildcard patterns', async () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: '*',
            hooks: [{ type: 'command', command: 'exit 0' }],
          },
        ],
      };
      manager = new HooksManager(config);

      const results = await manager.trigger('PostToolUse', {
        ...context,
        toolName: 'AnyTool',
      });

      expect(results).toHaveLength(1);
      expect(results[0].success).toBe(true);
    });

    it('should match regex patterns', async () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: 'Write|Edit',
            hooks: [{ type: 'command', command: 'exit 0' }],
          },
        ],
      };
      manager = new HooksManager(config);

      // Should match Write
      const results1 = await manager.trigger('PostToolUse', {
        ...context,
        toolName: 'Write',
      });
      expect(results1).toHaveLength(1);

      // Should match Edit
      const results2 = await manager.trigger('PostToolUse', {
        ...context,
        toolName: 'Edit',
      });
      expect(results2).toHaveLength(1);

      // Should not match Read
      const results3 = await manager.trigger('PostToolUse', {
        ...context,
        toolName: 'Read',
      });
      expect(results3).toHaveLength(0);
    });

    it('should respect maxHooks option', async () => {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: '*',
            hooks: [
              { type: 'command', command: 'exit 0' },
              { type: 'command', command: 'exit 0' },
              { type: 'command', command: 'exit 0' },
            ],
          },
        ],
      };
      manager = new HooksManager(config);

      const results = await manager.trigger('PostToolUse', context, { maxHooks: 2 });

      expect(results).toHaveLength(2);
    });
  });
});

describe('Utility Functions', () => {
  describe('hasBlockingResult', () => {
    it('should return false when no results are blocking', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0 },
        { success: true, exitCode: 0 },
      ];
      expect(hasBlockingResult(results)).toBe(false);
    });

    it('should return true when any result is blocking', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0 },
        { success: false, blocked: true, exitCode: 2 },
      ];
      expect(hasBlockingResult(results)).toBe(true);
    });
  });

  describe('getFirstBlockingResult', () => {
    it('should return undefined when no results are blocking', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0 },
        { success: true, exitCode: 0 },
      ];
      expect(getFirstBlockingResult(results)).toBeUndefined();
    });

    it('should return first blocking result', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0 },
        { success: false, blocked: true, exitCode: 2, error: 'First block' },
        { success: false, blocked: true, exitCode: 2, error: 'Second block' },
      ];
      const firstBlocking = getFirstBlockingResult(results);
      expect(firstBlocking).toBeDefined();
      expect(firstBlocking?.error).toBe('First block');
    });
  });

  describe('allSucceeded', () => {
    it('should return true when all hooks succeeded', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0 },
        { success: true, exitCode: 0 },
      ];
      expect(allSucceeded(results)).toBe(true);
    });

    it('should return false when any hook failed', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0 },
        { success: false, exitCode: 1 },
      ];
      expect(allSucceeded(results)).toBe(false);
    });

    it('should return true for empty array', () => {
      expect(allSucceeded([])).toBe(true);
    });
  });

  describe('getTotalDuration', () => {
    it('should sum all durations', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0, durationMs: 100 },
        { success: true, exitCode: 0, durationMs: 200 },
        { success: true, exitCode: 0, durationMs: 50 },
      ];
      expect(getTotalDuration(results)).toBe(350);
    });

    it('should handle missing durations', () => {
      const results: HookResult[] = [
        { success: true, exitCode: 0, durationMs: 100 },
        { success: true, exitCode: 0 },
        { success: true, exitCode: 0, durationMs: 50 },
      ];
      expect(getTotalDuration(results)).toBe(150);
    });

    it('should return 0 for empty array', () => {
      expect(getTotalDuration([])).toBe(0);
    });
  });
});
