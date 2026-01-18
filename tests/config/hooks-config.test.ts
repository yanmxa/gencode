/**
 * Tests for hooks configuration merging and fallback mechanism
 *
 * Verifies that hooks config:
 * 1. Merges correctly from .claude and .gencode directories
 * 2. GenCode settings override Claude settings at the same level
 * 3. Arrays are concatenated properly
 * 4. Deep merge works for nested objects
 */

import { describe, it, expect } from '@jest/globals';
import { deepMerge } from '../../src/config/merger.js';
import type { Settings } from '../../src/config/types.js';
import type { HooksConfig } from '../../src/hooks/types.js';

describe('Hooks Configuration Merging', () => {
  it('should use gencode hooks when only gencode settings exist', () => {
    const base: Settings = {};
    const gencode: Settings = {
      hooks: {
        PostToolUse: [
          {
            matcher: 'Write',
            hooks: [
              {
                type: 'command',
                command: 'echo "gencode hook"',
              },
            ],
          },
        ],
      },
    };

    const merged = deepMerge(base, gencode);

    expect(merged.hooks).toBeDefined();
    expect(merged.hooks?.PostToolUse).toHaveLength(1);
    expect(merged.hooks?.PostToolUse?.[0].matcher).toBe('Write');
    expect(merged.hooks?.PostToolUse?.[0].hooks[0].command).toBe('echo "gencode hook"');
  });

  it('should use claude hooks when gencode has no hooks (fallback)', () => {
    const claude: Settings = {
      hooks: {
        PostToolUse: [
          {
            matcher: 'Edit',
            hooks: [
              {
                type: 'command',
                command: 'echo "claude hook"',
              },
            ],
          },
        ],
      },
    };

    const gencode: Settings = {
      provider: 'google', // Has other settings but no hooks
    };

    // Simulate merge order: claude first, then gencode
    const merged = deepMerge(claude, gencode);

    expect(merged.hooks).toBeDefined();
    expect(merged.hooks?.PostToolUse).toHaveLength(1);
    expect(merged.hooks?.PostToolUse?.[0].matcher).toBe('Edit');
    expect(merged.hooks?.PostToolUse?.[0].hooks[0].command).toBe('echo "claude hook"');
    expect(merged.provider).toBe('google');
  });

  it('should merge hooks from both claude and gencode', () => {
    const claude: Settings = {
      hooks: {
        PostToolUse: [
          {
            matcher: 'Edit',
            hooks: [
              {
                type: 'command',
                command: 'echo "claude hook"',
              },
            ],
          },
        ],
      },
    };

    const gencode: Settings = {
      hooks: {
        PostToolUse: [
          {
            matcher: 'Write',
            hooks: [
              {
                type: 'command',
                command: 'echo "gencode hook"',
              },
            ],
          },
        ],
      },
    };

    // Simulate merge order: claude first, then gencode
    const merged = deepMerge(claude, gencode);

    expect(merged.hooks).toBeDefined();
    expect(merged.hooks?.PostToolUse).toHaveLength(2);

    // Both hooks should be present
    const editHook = merged.hooks?.PostToolUse?.find(h => h.matcher === 'Edit');
    const writeHook = merged.hooks?.PostToolUse?.find(h => h.matcher === 'Write');

    expect(editHook).toBeDefined();
    expect(editHook?.hooks[0].command).toBe('echo "claude hook"');

    expect(writeHook).toBeDefined();
    expect(writeHook?.hooks[0].command).toBe('echo "gencode hook"');
  });

  it('should merge hooks from different events', () => {
    const claude: Settings = {
      hooks: {
        PostToolUse: [
          {
            matcher: 'Edit',
            hooks: [
              {
                type: 'command',
                command: 'echo "claude post"',
              },
            ],
          },
        ],
      },
    };

    const gencode: Settings = {
      hooks: {
        Stop: [
          {
            hooks: [
              {
                type: 'command',
                command: 'echo "gencode stop"',
              },
            ],
          },
        ],
      },
    };

    const merged = deepMerge(claude, gencode);

    expect(merged.hooks).toBeDefined();
    expect(merged.hooks?.PostToolUse).toBeDefined();
    expect(merged.hooks?.Stop).toBeDefined();
    expect(merged.hooks?.PostToolUse?.[0].hooks[0].command).toBe('echo "claude post"');
    expect(merged.hooks?.Stop?.[0].hooks[0].command).toBe('echo "gencode stop"');
  });

  it('should handle gencode overriding same event as claude', () => {
    const claude: Settings = {
      hooks: {
        SessionStart: [
          {
            hooks: [
              {
                type: 'command',
                command: 'echo "claude session"',
              },
            ],
          },
        ],
      },
    };

    const gencode: Settings = {
      hooks: {
        SessionStart: [
          {
            hooks: [
              {
                type: 'command',
                command: 'echo "gencode session"',
              },
            ],
          },
        ],
      },
    };

    const merged = deepMerge(claude, gencode);

    expect(merged.hooks).toBeDefined();
    expect(merged.hooks?.SessionStart).toHaveLength(2); // Both merged (arrays concatenated)

    // Check both are present
    const claudeHook = merged.hooks?.SessionStart?.[0];
    const gencodeHook = merged.hooks?.SessionStart?.[1];

    expect(claudeHook?.hooks[0].command).toBe('echo "claude session"');
    expect(gencodeHook?.hooks[0].command).toBe('echo "gencode session"');
  });

  it('should deduplicate identical hooks', () => {
    const claude: Settings = {
      hooks: {
        Stop: [
          {
            hooks: [
              {
                type: 'command',
                command: 'echo "done"',
              },
            ],
          },
        ],
      },
    };

    const gencode: Settings = {
      hooks: {
        Stop: [
          {
            hooks: [
              {
                type: 'command',
                command: 'echo "done"', // Same as claude
              },
            ],
          },
        ],
      },
    };

    const merged = deepMerge(claude, gencode);

    expect(merged.hooks).toBeDefined();
    expect(merged.hooks?.Stop).toHaveLength(1); // Deduplicated
    expect(merged.hooks?.Stop?.[0].hooks[0].command).toBe('echo "done"');
  });

  it('should handle complex nested hooks configuration', () => {
    const claude: Settings = {
      hooks: {
        PreToolUse: [
          {
            matcher: 'Bash',
            hooks: [
              {
                type: 'command',
                command: 'echo "pre bash"',
                timeout: 5000,
                blocking: true,
              },
            ],
          },
        ],
        PostToolUse: [
          {
            matcher: 'Write|Edit',
            hooks: [
              {
                type: 'command',
                command: 'npm run lint',
                statusMessage: 'Running linter...',
              },
            ],
          },
        ],
      },
    };

    const gencode: Settings = {
      hooks: {
        PostToolUse: [
          {
            matcher: 'Bash',
            hooks: [
              {
                type: 'command',
                command: 'echo "post bash"',
              },
            ],
          },
        ],
        Stop: [
          {
            hooks: [
              {
                type: 'command',
                command: 'afplay done.mp3',
              },
            ],
          },
        ],
      },
    };

    const merged = deepMerge(claude, gencode);

    expect(merged.hooks).toBeDefined();
    expect(merged.hooks?.PreToolUse).toHaveLength(1);
    expect(merged.hooks?.PostToolUse).toHaveLength(2); // Merged from both
    expect(merged.hooks?.Stop).toHaveLength(1);

    // Verify PreToolUse from claude
    expect(merged.hooks?.PreToolUse?.[0].matcher).toBe('Bash');
    expect(merged.hooks?.PreToolUse?.[0].hooks[0].blocking).toBe(true);

    // Verify PostToolUse from both
    const lintHook = merged.hooks?.PostToolUse?.find(h => h.matcher === 'Write|Edit');
    const bashHook = merged.hooks?.PostToolUse?.find(h => h.matcher === 'Bash');
    expect(lintHook).toBeDefined();
    expect(bashHook).toBeDefined();

    // Verify Stop from gencode
    expect(merged.hooks?.Stop?.[0].hooks[0].command).toBe('afplay done.mp3');
  });
});
