/**
 * Comprehensive Functional Verification Test Runner
 *
 * Runs all functional tests in sequence and reports overall results.
 *
 * Tests included:
 * 1. Skills functional verification
 * 2. Commands functional verification
 * 3. Subagents functional verification
 * 4. Hooks functional verification
 *
 * Usage:
 *   npm run test:functional              # Normal mode
 *   GEN_DEBUG=2 npm run test:functional  # Verbose debug mode
 */

import { spawn } from 'child_process';
import * as path from 'path';

interface TestSuite {
  name: string;
  script: string;
  description: string;
}

const testSuites: TestSuite[] = [
  {
    name: 'Skills',
    script: 'scripts/test-skills-functional.ts',
    description: 'Skills activation and argument passing',
  },
  {
    name: 'Commands',
    script: 'scripts/test-commands-functional.ts',
    description: 'Command parsing and template expansion',
  },
  {
    name: 'Subagents',
    script: 'scripts/test-subagents-functional.ts',
    description: 'Subagent configuration and tool restrictions',
  },
  {
    name: 'Hooks',
    script: 'scripts/test-hooks-functional.ts',
    description: 'Hook execution and event triggering',
  },
];

interface TestResult {
  suite: string;
  passed: boolean;
  duration: number;
  error?: string;
}

async function runTestSuite(suite: TestSuite): Promise<TestResult> {
  const startTime = Date.now();

  console.log(`\n${'='.repeat(70)}`);
  console.log(`Running: ${suite.name} Functional Tests`);
  console.log(`Description: ${suite.description}`);
  console.log(`Script: ${suite.script}`);
  console.log('='.repeat(70) + '\n');

  return new Promise((resolve) => {
    const proc = spawn('npx', ['tsx', suite.script], {
      stdio: 'inherit',
      env: { ...process.env },
    });

    proc.on('close', (code) => {
      const duration = Date.now() - startTime;
      resolve({
        suite: suite.name,
        passed: code === 0,
        duration,
        error: code !== 0 ? `Exit code: ${code}` : undefined,
      });
    });

    proc.on('error', (error) => {
      const duration = Date.now() - startTime;
      resolve({
        suite: suite.name,
        passed: false,
        duration,
        error: error.message,
      });
    });
  });
}

async function main() {
  console.log('╔════════════════════════════════════════════════════════════════════╗');
  console.log('║        GenCode Comprehensive Functional Verification Tests        ║');
  console.log('╚════════════════════════════════════════════════════════════════════╝');

  const debugLevel = process.env.GEN_DEBUG || '0';
  console.log(`\nDebug Level: GEN_DEBUG=${debugLevel}`);
  if (debugLevel === '2') {
    console.log('Running in VERBOSE mode with detailed execution information\n');
  } else if (debugLevel === '1') {
    console.log('Running in DEBUG mode\n');
  } else {
    console.log('Running in NORMAL mode (set GEN_DEBUG=2 for verbose output)\n');
  }

  const results: TestResult[] = [];

  // Run all test suites sequentially
  for (const suite of testSuites) {
    const result = await runTestSuite(suite);
    results.push(result);
  }

  // Print final summary
  console.log('\n' + '='.repeat(70));
  console.log('FINAL SUMMARY');
  console.log('='.repeat(70));

  const passed = results.filter((r) => r.passed).length;
  const failed = results.filter((r) => !r.passed).length;
  const totalDuration = results.reduce((sum, r) => sum + r.duration, 0);

  console.log(`\nTotal Test Suites: ${results.length}`);
  console.log(`Passed: ${passed}`);
  console.log(`Failed: ${failed}`);
  console.log(`Total Duration: ${(totalDuration / 1000).toFixed(2)}s`);

  // Detailed results
  console.log('\nDetailed Results:');
  console.log('-'.repeat(70));

  results.forEach((result) => {
    const icon = result.passed ? '✓' : '✗';
    const color = result.passed ? '\x1b[32m' : '\x1b[31m';
    const reset = '\x1b[0m';
    const duration = (result.duration / 1000).toFixed(2);

    console.log(`${color}${icon}${reset} ${result.suite.padEnd(20)} ${duration}s`);
    if (result.error) {
      console.log(`  Error: ${result.error}`);
    }
  });

  // Failed tests details
  if (failed > 0) {
    console.log('\n' + '='.repeat(70));
    console.log('FAILED TEST SUITES');
    console.log('='.repeat(70));

    results
      .filter((r) => !r.passed)
      .forEach((r) => {
        console.log(`\n• ${r.suite}`);
        console.log(`  Error: ${r.error || 'Unknown error'}`);
        console.log(`  Recommendation: Run individually with GEN_DEBUG=2 for details:`);
        console.log(`  GEN_DEBUG=2 npx tsx ${testSuites.find(s => s.name === r.suite)?.script}`);
      });
  }

  // Success message
  if (failed === 0) {
    console.log('\n' + '='.repeat(70));
    console.log('✓ All functional tests passed successfully!');
    console.log('='.repeat(70));
  }

  console.log('\n');

  // Exit with appropriate code
  process.exit(failed > 0 ? 1 : 0);
}

main().catch((error) => {
  console.error('Fatal error during test execution:', error);
  process.exit(1);
});
