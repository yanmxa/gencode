/**
 * Commands Functional Verification Test
 *
 * Verifies:
 * 1. CommandManager initialization
 * 2. Command parsing
 * 3. Template variable expansion ({{arguments}}, {{cwd}}, {{arg0}}, etc.)
 * 4. allowed-tools parsing
 * 5. model override
 * 6. Non-existent command handling
 *
 * Usage:
 *   npm run test:commands:func              # Normal mode
 *   GEN_DEBUG=2 npm run test:commands:func  # Verbose debug mode
 */

import { CommandManager } from '../src/extensions/commands/manager.js';
import { isVerboseDebugEnabled } from '../src/base/utils/debug.js';
import * as fs from 'fs/promises';
import * as path from 'path';

interface TestResult {
  name: string;
  passed: boolean;
  error?: string;
  details?: string;
}

const results: TestResult[] = [];

function logTest(name: string, passed: boolean, details?: string, error?: string) {
  results.push({ name, passed, error, details });
  const icon = passed ? '✓' : '✗';
  const color = passed ? '\x1b[32m' : '\x1b[31m';
  const reset = '\x1b[0m';
  console.log(`${color}${icon}${reset} ${name}`);
  if (details && isVerboseDebugEnabled('commands')) {
    console.log(`  ${details}`);
  }
  if (error) {
    console.log(`  Error: ${error}`);
  }
}

async function setupTestCommand(cwd: string): Promise<string> {
  const testDir = path.join(cwd, '.gen', 'commands');
  await fs.mkdir(testDir, { recursive: true });

  const commandContent = `---
name: test-command
description: A test command for functional verification
allowed-tools: [Read, Write, Bash]
model: claude-sonnet-4
---

# Test Command

You are executing a test command with the following parameters:

**Arguments**: $ARGUMENTS
**First Argument**: $1
**Second Argument**: $2

## Task

This is a test command designed to verify:
1. Template variable expansion ($ARGUMENTS, $1, $2)
2. allowed-tools configuration
3. Model override functionality

Please confirm you received this command with the expanded variables.
`;

  const commandPath = path.join(testDir, 'test-command.md');
  await fs.writeFile(commandPath, commandContent, 'utf-8');

  return commandPath;
}

async function cleanupTestCommand(cwd: string) {
  const commandPath = path.join(cwd, '.gen', 'commands', 'test-command.md');
  await fs.unlink(commandPath).catch(() => {});
}

async function testCommandParsing() {
  console.log('Commands Functional Verification Test');
  console.log('======================================\n');

  const cwd = process.cwd();
  let commandPath: string | undefined;

  try {
    // Setup: Create test command BEFORE initializing manager
    console.log('Setup: Creating test command...');
    commandPath = await setupTestCommand(cwd);
    console.log(`Created test command at ${commandPath}\n`);

    // Test 1: Initialize CommandManager (after test command is created)
    console.log('Test 1: Initializing CommandManager...');
    let manager: CommandManager;
    try {
      manager = new CommandManager(cwd);
      await manager.initialize();
      logTest('CommandManager initialized successfully', true);
    } catch (error) {
      logTest(
        'CommandManager initialization failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
      return;
    }

    // Test 2: Parse simple command
    console.log('\nTest 2: Parsing simple command...');
    try {
      const result = await manager.parseCommand('test-command', 'arg1 arg2');

      if (!result) {
        logTest('Command parsing returned null', false, 'Expected ParsedCommand object');
      } else {
        const hasPrompt = result.expandedPrompt.length > 0;
        const hasTools = result.preAuthorizedTools.length > 0;
        const hasModel = result.modelOverride === 'claude-sonnet-4';
        logTest(
          'Command parsed successfully',
          hasPrompt && hasTools && hasModel,
          `Prompt length: ${result.expandedPrompt.length}, Tools: ${result.preAuthorizedTools.join(', ')}, Model: ${result.modelOverride}`
        );
      }
    } catch (error) {
      logTest(
        'Command parsing threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 3: Verify {{arguments}} expansion
    console.log('\nTest 3: Verifying {{arguments}} expansion...');
    try {
      const result = await manager.parseCommand('test-command', 'my-test-args');

      if (!result) {
        logTest('Command parsing failed', false);
      } else {
        const hasArgs = result.expandedPrompt.includes('my-test-args');
        logTest(
          '{{arguments}} template variable expanded',
          hasArgs,
          hasArgs ? 'Found arguments in expanded prompt' : 'Arguments not found'
        );
      }
    } catch (error) {
      logTest(
        'Arguments expansion test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 4: Verify positional arguments ($1, $2)
    console.log('\nTest 4: Verifying positional argument expansion...');
    try {
      const result = await manager.parseCommand('test-command', 'first-arg second-arg');

      if (!result) {
        logTest('Command parsing failed', false);
      } else {
        const hasArg1 = result.expandedPrompt.includes('first-arg');
        const hasArg2 = result.expandedPrompt.includes('second-arg');
        logTest(
          'Positional arguments expanded ($1, $2)',
          hasArg1 && hasArg2,
          `$1 found: ${hasArg1}, $2 found: ${hasArg2}`
        );
      }
    } catch (error) {
      logTest(
        'Positional arguments test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 6: Verify allowed-tools parsing
    console.log('\nTest 6: Verifying allowed-tools parsing...');
    try {
      const result = await manager.parseCommand('test-command', 'test');

      if (!result) {
        logTest('Command parsing failed', false);
      } else {
        const expectedTools = ['Read', 'Write', 'Bash'];
        const hasAllTools = expectedTools.every((tool) =>
          result.preAuthorizedTools.includes(tool)
        );
        logTest(
          'allowed-tools correctly parsed',
          hasAllTools,
          `Tools: ${result.preAuthorizedTools.join(', ')}`
        );
      }
    } catch (error) {
      logTest(
        'allowed-tools parsing test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 7: Verify model override
    console.log('\nTest 7: Verifying model override...');
    try {
      const result = await manager.parseCommand('test-command', 'test');

      if (!result) {
        logTest('Command parsing failed', false);
      } else {
        const correctModel = result.modelOverride === 'claude-sonnet-4';
        logTest(
          'Model override correctly set',
          correctModel,
          `Model: ${result.modelOverride || 'none'}`
        );
      }
    } catch (error) {
      logTest(
        'Model override test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 8: Non-existent command
    console.log('\nTest 8: Handling non-existent command...');
    try {
      const result = await manager.parseCommand('non-existent-command', '');

      if (result === null) {
        logTest('Non-existent command correctly returns null', true);
      } else {
        logTest('Non-existent command should return null but returned object', false);
      }
    } catch (error) {
      logTest(
        'Non-existent command test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 9: List commands
    console.log('\nTest 9: Listing commands...');
    try {
      const commands = await manager.listCommands();
      const hasTestCommand = commands.some((cmd) => cmd.name === 'test-command');
      logTest(
        'listCommands includes test command',
        hasTestCommand,
        `Total commands: ${commands.length}`
      );
    } catch (error) {
      logTest(
        'List commands test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

  } finally {
    // Cleanup
    if (commandPath) {
      console.log('\nCleanup: Removing test command...');
      await cleanupTestCommand(cwd);
    }
  }

  // Print summary
  console.log('\n======================================');
  console.log('Test Summary');
  console.log('======================================');
  const passed = results.filter((r) => r.passed).length;
  const failed = results.filter((r) => !r.passed).length;
  console.log(`Total: ${results.length}`);
  console.log(`Passed: ${passed}`);
  console.log(`Failed: ${failed}`);

  if (failed > 0) {
    console.log('\nFailed Tests:');
    results
      .filter((r) => !r.passed)
      .forEach((r) => {
        console.log(`  - ${r.name}`);
        if (r.error) {
          console.log(`    ${r.error}`);
        }
      });
  }

  console.log('\n');

  // Exit with appropriate code
  process.exit(failed > 0 ? 1 : 0);
}

testCommandParsing().catch((error) => {
  console.error('Fatal error during Commands functional test:', error);
  process.exit(1);
});
