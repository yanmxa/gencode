#!/usr/bin/env node
/**
 * Commands Loading Test
 *
 * Tests command discovery from all configured directories with proper precedence:
 * 1. ~/.claude/commands/ (user-level Claude Code) - lowest priority
 * 2. ~/.gen/commands/ (user-level GenCode)
 * 3. .claude/commands/ (project-level Claude Code)
 * 4. .gen/commands/ (project-level GenCode) - highest priority
 *
 * Usage:
 *   npm run test:commands                     # Normal mode
 *   GEN_DEBUG_COMMANDS=1 npm run test:commands  # Debug mode
 *   npm run test:commands -- --verbose        # Show all commands
 */

import { discoverCommands } from '../src/ext/commands/discovery.js';
import { logger } from '../src/base/utils/logger.js';

const args = process.argv.slice(2);
const verbose = args.includes('--verbose');

async function main(): Promise<void> {
  console.log('Commands Discovery Test\n');

  const cwd = process.cwd();

  try {
    logger.info('CommandsTest', 'Starting commands discovery', { cwd });

    const startTime = Date.now();
    const commands = await discoverCommands(cwd);
    const duration = Date.now() - startTime;

    console.log(`✓ Loaded ${commands.size} commands in ${duration}ms\n`);

    if (verbose || commands.size <= 20) {
      console.log('Discovered commands:');
      for (const [name, cmd] of commands) {
        console.log(`  /${name}`);
        console.log(`    Description: ${cmd.description}`);
        if (cmd.argumentHint) {
          console.log(`    Args: ${cmd.argumentHint}`);
        }
        if (cmd.allowedTools) {
          const tools = Array.isArray(cmd.allowedTools)
            ? cmd.allowedTools.join(', ')
            : cmd.allowedTools;
          console.log(`    Allowed tools: ${tools}`);
        }
        console.log();
      }
    } else {
      console.log('Top 10 commands (use --verbose to see all):');
      let count = 0;
      for (const [name, cmd] of commands) {
        if (count++ >= 10) break;
        console.log(`  /${name}: ${cmd.description}`);
      }
      console.log();
    }

    // Test precedence by checking for common command names
    const commonNames = ['help', 'commit', 'review', 'test'];
    const foundCommon = commonNames.filter(name => commands.has(name));
    if (foundCommon.length > 0) {
      console.log(`Common commands found: ${foundCommon.map(n => '/' + n).join(', ')}\n`);
    }

    process.exit(0);
  } catch (error) {
    logger.error('CommandsTest', 'Commands discovery failed', {
      error: error instanceof Error ? error.message : String(error),
    });
    console.error('✗ Failed to load commands');
    if (error instanceof Error && error.stack) {
      console.error(error.stack);
    }
    process.exit(1);
  }
}

main();
