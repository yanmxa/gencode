#!/usr/bin/env node
/**
 * Hooks Loading Test
 *
 * Tests hooks loading from settings.json files:
 * - ~/.gen/settings.json
 * - .gen/settings.json
 *
 * Project-level hooks override user-level hooks.
 *
 * Usage:
 *   npm run test:hooks                     # Normal mode
 *   GEN_DEBUG_HOOKS=1 npm run test:hooks   # Debug mode
 *   npm run test:hooks -- --verbose        # Show all hooks
 */

import { HooksManager } from '../src/ext/hooks/hooks-manager.js';
import { logger } from '../src/base/utils/logger.js';

const args = process.argv.slice(2);
const verbose = args.includes('--verbose');

async function main(): Promise<void> {
  console.log('Hooks Loading Test\n');

  const cwd = process.cwd();
  const hooksManager = new HooksManager();

  try {
    logger.info('HooksTest', 'Starting hooks loading', { cwd });

    const startTime = Date.now();
    await hooksManager.initialize(cwd);
    const hookCount = hooksManager.getHookCount();
    const duration = Date.now() - startTime;

    console.log(`✓ Loaded ${hookCount} hooks in ${duration}ms\n`);

    if (verbose && hookCount > 0) {
      console.log('Hook configuration:');
      // Note: HooksManager doesn't expose individual hooks currently
      // This would require extending the API
      console.log('  (Use GEN_DEBUG_HOOKS=1 for detailed hook information)\n');
    } else if (hookCount === 0) {
      console.log('No hooks configured.\n');
      console.log('To add hooks, create settings.json in:');
      console.log('  - ~/.gen/settings.json (user-level)');
      console.log('  - .gen/settings.json (project-level)\n');
      console.log('Example hook configuration:');
      console.log(`{
  "hooks": [
    {
      "event": "ToolCall",
      "matcher": { "tool": "Bash", "pattern": "git push" },
      "command": "echo 'About to push to git'"
    }
  ]
}\n`);
    }

    // Test hook matching (if any hooks exist)
    if (hookCount > 0) {
      console.log('Testing hook matcher...');

      const testEvents = [
        { event: 'ToolCall' as const, tool: 'Bash', command: 'git status' },
        { event: 'ToolCall' as const, tool: 'Read', command: 'package.json' },
        { event: 'Notification' as const },
      ];

      for (const testEvent of testEvents) {
        if (testEvent.event === 'ToolCall') {
          const matched = await hooksManager.executeHooks(
            testEvent.event,
            testEvent.tool,
            testEvent.command
          );
          console.log(`  ${testEvent.tool}("${testEvent.command}"): ${matched ? '✓ matched' : '✗ no match'}`);
        }
      }
      console.log();
    }

    process.exit(0);
  } catch (error) {
    logger.error('HooksTest', 'Hooks loading failed', {
      error: error instanceof Error ? error.message : String(error),
    });
    console.error('✗ Failed to load hooks');
    if (error instanceof Error && error.stack) {
      console.error(error.stack);
    }
    process.exit(1);
  }
}

main();
