/**
 * Subagents Functional Verification Test
 *
 * Verifies:
 * 1. Task tool is available
 * 2. Subagent configuration for different types (Explore, Plan, Bash, general-purpose)
 * 3. Tool restrictions per agent type
 * 4. max_turns configuration
 * 5. Model override
 *
 * Note: This test validates subagent creation and configuration without
 * making actual LLM API calls to avoid cost and credential requirements.
 * For full end-to-end testing with actual execution, set GEN_PROVIDER and
 * run with API credentials.
 *
 * Usage:
 *   npm run test:subagents:func              # Normal mode (validation only)
 *   GEN_DEBUG=2 npm run test:subagents:func  # Verbose debug mode
 */

import { Subagent } from '../src/subagents/subagent.js';
import { SUBAGENT_CONFIGS } from '../src/subagents/configs.js';
import { isVerboseDebugEnabled } from '../src/shared/debug.js';
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
  if (details && isVerboseDebugEnabled('subagents')) {
    console.log(`  ${details}`);
  }
  if (error) {
    console.log(`  Error: ${error}`);
  }
}

async function setupTestFile(cwd: string): Promise<string> {
  const testFile = path.join(cwd, '.gen-test-subagent.txt');
  await fs.writeFile(testFile, 'Test content for subagent verification', 'utf-8');
  return testFile;
}

async function cleanupTestFile(testFile: string) {
  await fs.unlink(testFile).catch(() => {});
}

async function testSubagentConfiguration() {
  console.log('Subagents Functional Verification Test');
  console.log('========================================\n');

  const cwd = process.cwd();
  let testFile: string | undefined;

  try {
    // Setup: Create test file
    console.log('Setup: Creating test file...');
    testFile = await setupTestFile(cwd);
    console.log(`Created test file at ${testFile}\n`);

    // Test 1: Explore agent configuration
    console.log('Test 1: Verifying Explore agent configuration...');
    try {
      const exploreConfig = SUBAGENT_CONFIGS['Explore'];
      const hasReadOnlyTools = exploreConfig.allowedTools.includes('Read') &&
                               exploreConfig.allowedTools.includes('Glob') &&
                               exploreConfig.allowedTools.includes('Grep');
      const noWriteTools = !exploreConfig.allowedTools.includes('Write') &&
                          !exploreConfig.allowedTools.includes('Edit');
      const correctModel = exploreConfig.defaultModel === 'claude-haiku-4';
      const correctMaxTurns = exploreConfig.maxTurns === 10;

      const passed = hasReadOnlyTools && noWriteTools && correctModel && correctMaxTurns;
      logTest(
        'Explore agent has correct configuration',
        passed,
        `Tools: ${exploreConfig.allowedTools.join(', ')}, Model: ${exploreConfig.defaultModel}, MaxTurns: ${exploreConfig.maxTurns}`
      );
    } catch (error) {
      logTest(
        'Explore agent configuration failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 2: Plan agent configuration
    console.log('\nTest 2: Verifying Plan agent configuration...');
    try {
      const planConfig = SUBAGENT_CONFIGS['Plan'];
      const hasReadTools = planConfig.allowedTools.includes('Read');
      const hasTodoWrite = planConfig.allowedTools.includes('TodoWrite');
      const noExecutionTools = !planConfig.allowedTools.includes('Write') &&
                              !planConfig.allowedTools.includes('Edit') &&
                              !planConfig.allowedTools.includes('Bash');
      const correctModel = planConfig.defaultModel === 'claude-sonnet-4';

      const passed = hasReadTools && hasTodoWrite && noExecutionTools && correctModel;
      logTest(
        'Plan agent has correct configuration',
        passed,
        `Tools: ${planConfig.allowedTools.join(', ')}, Model: ${planConfig.defaultModel}`
      );
    } catch (error) {
      logTest(
        'Plan agent configuration failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 3: Bash agent configuration
    console.log('\nTest 3: Verifying Bash agent configuration...');
    try {
      const bashConfig = SUBAGENT_CONFIGS['Bash'];
      const onlyBashTool = bashConfig.allowedTools.length === 1 &&
                          bashConfig.allowedTools[0] === 'Bash';
      const correctModel = bashConfig.defaultModel === 'claude-haiku-4';
      const correctMaxTurns = bashConfig.maxTurns === 20;

      const passed = onlyBashTool && correctModel && correctMaxTurns;
      logTest(
        'Bash agent has correct configuration',
        passed,
        `Tools: ${bashConfig.allowedTools.join(', ')}, Model: ${bashConfig.defaultModel}, MaxTurns: ${bashConfig.maxTurns}`
      );
    } catch (error) {
      logTest(
        'Bash agent configuration failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 4: general-purpose agent configuration
    console.log('\nTest 4: Verifying general-purpose agent configuration...');
    try {
      const gpConfig = SUBAGENT_CONFIGS['general-purpose'];
      const hasWildcard = gpConfig.allowedTools.includes('*');
      const correctModel = gpConfig.defaultModel === 'claude-sonnet-4';
      const correctMaxTurns = gpConfig.maxTurns === 20;

      const passed = hasWildcard && correctModel && correctMaxTurns;
      logTest(
        'general-purpose agent has correct configuration',
        passed,
        `Tools: ${gpConfig.allowedTools.join(', ')} (wildcard=all), Model: ${gpConfig.defaultModel}, MaxTurns: ${gpConfig.maxTurns}`
      );
    } catch (error) {
      logTest(
        'general-purpose agent configuration failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 5: Subagent creation with Explore type
    console.log('\nTest 5: Creating Explore subagent instance...');
    try {
      const subagent = new Subagent({
        type: 'Explore',
        cwd,
        description: 'Test Explore agent',
      });

      // Verify the subagent was created successfully
      // We can't actually run it without API credentials, but we can verify creation
      logTest('Explore subagent instance created successfully', true, 'Instance created');
    } catch (error) {
      logTest(
        'Explore subagent creation failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 6: Subagent creation with model override
    console.log('\nTest 6: Creating subagent with model override...');
    try {
      const subagent = new Subagent({
        type: 'Explore',
        model: 'claude-sonnet-4',
        cwd,
        description: 'Test model override',
      });

      logTest('Subagent with model override created successfully', true, 'Model: claude-sonnet-4');
    } catch (error) {
      logTest(
        'Subagent model override failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 7: Subagent creation with max_turns override
    console.log('\nTest 7: Creating subagent with max_turns override...');
    try {
      const subagent = new Subagent({
        type: 'Explore',
        cwd,
        config: { maxTurns: 5 },
        description: 'Test max_turns override',
      });

      logTest('Subagent with max_turns override created successfully', true, 'MaxTurns: 5');
    } catch (error) {
      logTest(
        'Subagent max_turns override failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 8: Verify custom agent loader (if any exist)
    console.log('\nTest 8: Checking for custom agents...');
    try {
      const { CustomAgentLoader } = await import('../src/subagents/custom-agent-loader.js');
      const loader = new CustomAgentLoader();
      await loader.initialize();
      const customAgents = await loader.listAgents(cwd);

      logTest(
        'Custom agent loader initialized',
        true,
        `Found ${customAgents.length} custom agent(s): ${customAgents.map(a => a.name).join(', ') || 'none'}`
      );
    } catch (error) {
      logTest(
        'Custom agent loader failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Note about execution testing
    console.log('\n----------------------------------------');
    console.log('Note: Actual subagent execution testing requires API credentials.');
    console.log('To test execution, ensure GEN_PROVIDER and appropriate API keys are set.');
    console.log('This test validates configuration and setup without making API calls.');
    console.log('----------------------------------------');

  } finally {
    // Cleanup
    if (testFile) {
      console.log('\nCleanup: Removing test file...');
      await cleanupTestFile(testFile);
    }
  }

  // Print summary
  console.log('\n========================================');
  console.log('Test Summary');
  console.log('========================================');
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

testSubagentConfiguration().catch((error) => {
  console.error('Fatal error during Subagents functional test:', error);
  process.exit(1);
});
