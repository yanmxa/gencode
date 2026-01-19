/**
 * End-to-end integration tests for hooks configuration loading
 *
 * Tests the complete flow of loading hooks from .claude and .gencode directories
 */

import { describe, it, expect, beforeEach, afterEach } from '@jest/globals';
import { ConfigManager } from '../../src/base/config/manager.js';
import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { HooksConfig } from '../../src/ext/hooks/types.js';

describe('Hooks Configuration Loading (Integration)', () => {
  let testProjectDir: string;
  let claudeDir: string;
  let genDir: string;

  beforeEach(async () => {
    // Create a temporary project directory
    testProjectDir = await fs.mkdtemp(path.join(os.tmpdir(), 'gencode-hooks-integration-'));
    claudeDir = path.join(testProjectDir, '.claude');
    genDir = path.join(testProjectDir, '.gen');
  });

  afterEach(async () => {
    try {
      await fs.rm(testProjectDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup errors
    }
  });

  it('should load hooks from .gencode when only .gencode exists', async () => {
    // Setup: Only .gencode directory with hooks
    await fs.mkdir(genDir, { recursive: true });
    const gencodeHooks: HooksConfig = {
      PostToolUse: [
        {
          matcher: 'Write',
          hooks: [
            {
              type: 'command',
              command: 'echo "gencode only"',
            },
          ],
        },
      ],
    };

    await fs.writeFile(
      path.join(genDir, 'settings.json'),
      JSON.stringify({ hooks: gencodeHooks }, null, 2)
    );

    // Test: Load configuration
    const configManager = new ConfigManager({ cwd: testProjectDir });
    await configManager.load();
    const settings = configManager.get();

    // Verify
    expect(settings.hooks).toBeDefined();
    expect(settings.hooks?.PostToolUse).toBeDefined();
    expect(settings.hooks?.PostToolUse?.[0].hooks[0].command).toBe('echo "gencode only"');
  });

  it('should fallback to .claude when .gencode has no hooks', async () => {
    // Setup: Both directories exist, but only .claude has hooks
    await fs.mkdir(claudeDir, { recursive: true });
    await fs.mkdir(genDir, { recursive: true });

    const claudeHooks: HooksConfig = {
      Stop: [
        {
          hooks: [
            {
              type: 'command',
              command: 'echo "claude fallback"',
            },
          ],
        },
      ],
    };

    await fs.writeFile(
      path.join(claudeDir, 'settings.json'),
      JSON.stringify({ hooks: claudeHooks }, null, 2)
    );

    await fs.writeFile(
      path.join(genDir, 'settings.json'),
      JSON.stringify({ provider: 'google' }, null, 2) // No hooks
    );

    // Test
    const configManager = new ConfigManager({ cwd: testProjectDir });
    await configManager.load();
    const settings = configManager.get();

    // Verify: Should use claude hooks since gencode has none
    expect(settings.hooks).toBeDefined();
    expect(settings.hooks?.Stop).toBeDefined();
    expect(settings.hooks?.Stop?.[0].hooks[0].command).toBe('echo "claude fallback"');
    expect(settings.provider).toBe('google'); // gencode settings also loaded
  });

  it('should merge hooks from both .claude and .gencode', async () => {
    // Setup: Both have different hooks
    await fs.mkdir(claudeDir, { recursive: true });
    await fs.mkdir(genDir, { recursive: true });

    const claudeHooks: HooksConfig = {
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
    };

    const gencodeHooks: HooksConfig = {
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
    };

    await fs.writeFile(
      path.join(claudeDir, 'settings.json'),
      JSON.stringify({ hooks: claudeHooks }, null, 2)
    );

    await fs.writeFile(
      path.join(genDir, 'settings.json'),
      JSON.stringify({ hooks: gencodeHooks }, null, 2)
    );

    // Test
    const configManager = new ConfigManager({ cwd: testProjectDir });
    await configManager.load();
    const settings = configManager.get();

    // Verify: Both hooks should be present
    expect(settings.hooks).toBeDefined();
    expect(settings.hooks?.SessionStart).toBeDefined();
    expect(settings.hooks?.Stop).toBeDefined();
    expect(settings.hooks?.SessionStart?.[0].hooks[0].command).toBe('echo "claude session"');
    expect(settings.hooks?.Stop?.[0].hooks[0].command).toBe('echo "gencode stop"');
  });

  it('should merge hooks for the same event from both sources', async () => {
    // Setup: Both have hooks for PostToolUse
    await fs.mkdir(claudeDir, { recursive: true });
    await fs.mkdir(genDir, { recursive: true });

    const claudeHooks: HooksConfig = {
      PostToolUse: [
        {
          matcher: 'Edit',
          hooks: [
            {
              type: 'command',
              command: 'eslint --fix $FILE_PATH',
            },
          ],
        },
      ],
    };

    const gencodeHooks: HooksConfig = {
      PostToolUse: [
        {
          matcher: 'Write',
          hooks: [
            {
              type: 'command',
              command: 'prettier --write $FILE_PATH',
            },
          ],
        },
      ],
    };

    await fs.writeFile(
      path.join(claudeDir, 'settings.json'),
      JSON.stringify({ hooks: claudeHooks }, null, 2)
    );

    await fs.writeFile(
      path.join(genDir, 'settings.json'),
      JSON.stringify({ hooks: gencodeHooks }, null, 2)
    );

    // Test
    const configManager = new ConfigManager({ cwd: testProjectDir });
    await configManager.load();
    const settings = configManager.get();

    // Verify: Both PostToolUse hooks should be merged
    expect(settings.hooks).toBeDefined();
    expect(settings.hooks?.PostToolUse).toBeDefined();
    expect(settings.hooks?.PostToolUse).toHaveLength(2);

    const editHook = settings.hooks?.PostToolUse?.find(h => h.matcher === 'Edit');
    const writeHook = settings.hooks?.PostToolUse?.find(h => h.matcher === 'Write');

    expect(editHook).toBeDefined();
    expect(editHook?.hooks[0].command).toContain('eslint');

    expect(writeHook).toBeDefined();
    expect(writeHook?.hooks[0].command).toContain('prettier');
  });

  it('should only use .claude when .gencode does not exist', async () => {
    // Setup: Only .claude directory
    await fs.mkdir(claudeDir, { recursive: true });

    const claudeHooks: HooksConfig = {
      PreToolUse: [
        {
          matcher: 'Bash',
          hooks: [
            {
              type: 'command',
              command: 'echo "claude only"',
              blocking: true,
            },
          ],
        },
      ],
    };

    await fs.writeFile(
      path.join(claudeDir, 'settings.json'),
      JSON.stringify({ hooks: claudeHooks }, null, 2)
    );

    // Test
    const configManager = new ConfigManager({ cwd: testProjectDir });
    await configManager.load();
    const settings = configManager.get();

    // Verify
    expect(settings.hooks).toBeDefined();
    expect(settings.hooks?.PreToolUse).toBeDefined();
    expect(settings.hooks?.PreToolUse?.[0].hooks[0].command).toBe('echo "claude only"');
    expect(settings.hooks?.PreToolUse?.[0].hooks[0].blocking).toBe(true);
  });

  it('should load complex multi-event hooks from both sources', async () => {
    // Setup: Complex real-world scenario
    await fs.mkdir(claudeDir, { recursive: true });
    await fs.mkdir(genDir, { recursive: true });

    // Claude: PostToolUse for linting + Stop notification
    const claudeHooks: HooksConfig = {
      PostToolUse: [
        {
          matcher: 'Write|Edit',
          hooks: [
            {
              type: 'command',
              command: 'npm run lint:fix $FILE_PATH',
              timeout: 30000,
              statusMessage: 'Running linter...',
            },
          ],
        },
      ],
      Stop: [
        {
          hooks: [
            {
              type: 'command',
              command: 'afplay ~/.claude/sounds/done.mp3',
            },
          ],
        },
      ],
    };

    // GenCode: PreToolUse validation + additional PostToolUse
    const gencodeHooks: HooksConfig = {
      PreToolUse: [
        {
          matcher: 'Bash',
          hooks: [
            {
              type: 'command',
              command: 'echo "Validating bash command..."',
            },
          ],
        },
      ],
      PostToolUse: [
        {
          matcher: 'Bash',
          hooks: [
            {
              type: 'command',
              command: 'echo "Bash command completed"',
            },
          ],
        },
      ],
      SessionStart: [
        {
          hooks: [
            {
              type: 'command',
              command: 'echo "GenCode session started at $(date)"',
            },
          ],
        },
      ],
    };

    await fs.writeFile(
      path.join(claudeDir, 'settings.json'),
      JSON.stringify({ hooks: claudeHooks }, null, 2)
    );

    await fs.writeFile(
      path.join(genDir, 'settings.json'),
      JSON.stringify({ hooks: gencodeHooks }, null, 2)
    );

    // Test
    const configManager = new ConfigManager({ cwd: testProjectDir });
    await configManager.load();
    const settings = configManager.get();

    // Verify all events are loaded
    expect(settings.hooks).toBeDefined();
    expect(settings.hooks?.PreToolUse).toBeDefined();
    expect(settings.hooks?.PostToolUse).toBeDefined();
    expect(settings.hooks?.Stop).toBeDefined();
    expect(settings.hooks?.SessionStart).toBeDefined();

    // Verify PostToolUse has both hooks (merged)
    expect(settings.hooks?.PostToolUse).toHaveLength(2);

    // Verify each hook source
    const lintHook = settings.hooks?.PostToolUse?.find(h => h.matcher === 'Write|Edit');
    const bashHook = settings.hooks?.PostToolUse?.find(h => h.matcher === 'Bash');

    expect(lintHook).toBeDefined();
    expect(lintHook?.hooks[0].command).toContain('lint:fix');

    expect(bashHook).toBeDefined();
    expect(bashHook?.hooks[0].command).toContain('Bash command completed');
  });
});
