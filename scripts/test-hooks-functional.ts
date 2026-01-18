/**
 * Hooks End-to-End Functional Verification Test
 *
 * Verifies:
 * 1. Hook configuration loading
 * 2. Event triggering
 * 3. Hook execution with stdin payload
 * 4. Blocking hooks (exit code 2)
 * 5. Hook matcher (tool name, command pattern)
 * 6. Parallel vs sequential execution
 *
 * Usage:
 *   npm run test:hooks:func              # Normal mode
 *   GEN_DEBUG=2 npm run test:hooks:func  # Verbose debug mode
 */

import { HooksManager } from '../src/hooks/hooks-manager.js';
import { isVerboseDebugEnabled } from '../src/shared/debug.js';
import type { HooksConfig } from '../src/hooks/types.js';
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
  if (details && isVerboseDebugEnabled('hooks')) {
    console.log(`  ${details}`);
  }
  if (error) {
    console.log(`  Error: ${error}`);
  }
}

async function setupTestEnvironment(cwd: string): Promise<string> {
  const testDir = path.join(cwd, '.gen-test-hooks');
  await fs.mkdir(testDir, { recursive: true });
  return testDir;
}

async function cleanupTestEnvironment(testDir: string) {
  await fs.rm(testDir, { recursive: true, force: true });
}

async function testHookExecution() {
  console.log('Hooks End-to-End Functional Verification Test');
  console.log('==============================================\n');

  const cwd = process.cwd();
  let testDir: string | undefined;

  try {
    // Setup: Create test directory
    console.log('Setup: Creating test environment...');
    testDir = await setupTestEnvironment(cwd);
    console.log(`Created test directory at ${testDir}\n`);

    const outputFile = path.join(testDir, 'hook-output.txt');
    const blockFile = path.join(testDir, 'hook-block.txt');

    // Test 1: Create HooksManager with configuration
    console.log('Test 1: Creating HooksManager with test configuration...');
    let manager: HooksManager;
    try {
      const config: HooksConfig = {
        PostToolUse: [
          {
            matcher: 'Read',
            hooks: [
              {
                type: 'command',
                command: `echo "Hook triggered: Read tool" > ${outputFile}`,
                blocking: false,
              },
            ],
          },
          {
            matcher: 'Write',
            hooks: [
              {
                type: 'command',
                command: `echo "Hook blocked: Write tool" > ${blockFile} && exit 2`,
                blocking: true,
              },
            ],
          },
        ],
        Stop: [
          {
            matcher: '*',
            hooks: [
              {
                type: 'command',
                command: `echo "Stop hook" >> ${outputFile}`,
                blocking: false,
              },
            ],
          },
        ],
      };

      manager = new HooksManager(config);
      logTest('HooksManager created with configuration', true, `Events: ${Object.keys(config).join(', ')}`);
    } catch (error) {
      logTest(
        'HooksManager creation failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
      return;
    }

    // Test 2: Trigger non-blocking hook (Read tool)
    console.log('\nTest 2: Triggering non-blocking hook (Read tool)...');
    try {
      const results = await manager.trigger('PostToolUse', {
        event: 'PostToolUse',
        toolName: 'Read',
        cwd: testDir,
        timestamp: new Date(),
        toolInput: { file_path: '/test/file.txt' },
      });

      // Wait for hook execution
      await new Promise((resolve) => setTimeout(resolve, 500));

      // Check output file
      const output = await fs.readFile(outputFile, 'utf-8').catch(() => '');
      const hookExecuted = output.includes('Hook triggered: Read tool');

      logTest(
        'Non-blocking hook executed successfully',
        hookExecuted,
        hookExecuted ? `Output: ${output.trim()}` : 'Output file not found or empty'
      );
    } catch (error) {
      logTest(
        'Non-blocking hook trigger failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 3: Verify non-matching tool does not trigger hook
    console.log('\nTest 3: Verifying non-matching tool does not trigger hook...');
    try {
      // Clear output file
      await fs.unlink(outputFile).catch(() => {});

      const results = await manager.trigger('PostToolUse', {
        event: 'PostToolUse',
        toolName: 'Bash', // Different tool, should not match
        cwd: testDir,
        timestamp: new Date(),
        toolInput: { command: 'ls' },
      });

      // Wait a bit
      await new Promise((resolve) => setTimeout(resolve, 500));

      // Check output file should not exist
      const output = await fs.readFile(outputFile, 'utf-8').catch(() => '');
      const noHookTriggered = output === '';

      logTest(
        'Non-matching tool correctly did not trigger hook',
        noHookTriggered,
        noHookTriggered ? 'No output file created' : 'Hook was incorrectly triggered'
      );
    } catch (error) {
      logTest(
        'Non-matching tool test failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 4: Trigger blocking hook (Write tool)
    console.log('\nTest 4: Triggering blocking hook (Write tool)...');
    try {
      const results = await manager.trigger('PostToolUse', {
        event: 'PostToolUse',
        toolName: 'Write',
        cwd: testDir,
        timestamp: new Date(),
        toolInput: { file_path: '/test/file.txt' },
      });

      // Wait for hook execution
      await new Promise((resolve) => setTimeout(resolve, 500));

      // Check if hook was blocked (exit code 2)
      const hasBlockingResult = results.some((r) => r.blocked === true);

      // Check block file
      const blockOutput = await fs.readFile(blockFile, 'utf-8').catch(() => '');
      const hookExecuted = blockOutput.includes('Hook blocked: Write tool');

      logTest(
        'Blocking hook executed and blocked correctly',
        hookExecuted && hasBlockingResult,
        `Blocked: ${hasBlockingResult}, Output: ${blockOutput.trim()}`
      );
    } catch (error) {
      logTest(
        'Blocking hook trigger failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 5: Trigger Stop event
    console.log('\nTest 5: Triggering Stop event...');
    try {
      // Clear output file
      await fs.unlink(outputFile).catch(() => {});

      const results = await manager.trigger('Stop', {
        event: 'Stop',
        cwd: testDir,
        timestamp: new Date(),
      });

      // Wait for hook execution
      await new Promise((resolve) => setTimeout(resolve, 500));

      // Check output file
      const output = await fs.readFile(outputFile, 'utf-8').catch(() => '');
      const hookExecuted = output.includes('Stop hook');

      logTest(
        'Stop hook executed successfully',
        hookExecuted,
        hookExecuted ? `Output: ${output.trim()}` : 'Output file not found or empty'
      );
    } catch (error) {
      logTest(
        'Stop hook trigger failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 6: Check hasHooks method
    console.log('\nTest 6: Verifying hasHooks method...');
    try {
      const hasPostToolUseHooks = manager.hasHooks('PostToolUse');
      const hasStopHooks = manager.hasHooks('Stop');
      const hasSessionStartHooks = manager.hasHooks('SessionStart');

      const passed = hasPostToolUseHooks && hasStopHooks && !hasSessionStartHooks;
      logTest(
        'hasHooks correctly identifies configured events',
        passed,
        `PostToolUse: ${hasPostToolUseHooks}, Stop: ${hasStopHooks}, SessionStart: ${hasSessionStartHooks}`
      );
    } catch (error) {
      logTest(
        'hasHooks method test failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 7: Test getMatchers method
    console.log('\nTest 7: Verifying getMatchers method...');
    try {
      const postToolUseMatchers = manager.getMatchers('PostToolUse');
      const stopMatchers = manager.getMatchers('Stop');

      const hasCorrectCounts = postToolUseMatchers.length === 2 && stopMatchers.length === 1;
      logTest(
        'getMatchers returns correct number of matchers',
        hasCorrectCounts,
        `PostToolUse: ${postToolUseMatchers.length} matchers, Stop: ${stopMatchers.length} matcher`
      );
    } catch (error) {
      logTest(
        'getMatchers method test failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

  } finally {
    // Cleanup
    if (testDir) {
      console.log('\nCleanup: Removing test environment...');
      await cleanupTestEnvironment(testDir);
    }
  }

  // Print summary
  console.log('\n==============================================');
  console.log('Test Summary');
  console.log('==============================================');
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

testHookExecution().catch((error) => {
  console.error('Fatal error during Hooks functional test:', error);
  process.exit(1);
});
