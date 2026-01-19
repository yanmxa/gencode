/**
 * Skills Functional Verification Test
 *
 * Verifies:
 * 1. Skill tool creation succeeds
 * 2. Activating existing skill works
 * 3. Arguments are correctly passed
 * 4. Non-existent skill returns error
 * 5. GEN_DEBUG=2 shows detailed information
 *
 * Usage:
 *   npm run test:skills:func              # Normal mode
 *   GEN_DEBUG=2 npm run test:skills:func  # Verbose debug mode
 */

import { createSkillTool, resetSkillDiscovery } from '../src/extensions/skills/skill-tool.js';
import { isVerboseDebugEnabled } from '../src/infrastructure/utils/debug.js';
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
  if (details && isVerboseDebugEnabled('skills')) {
    console.log(`  ${details}`);
  }
  if (error) {
    console.log(`  Error: ${error}`);
  }
}

async function setupTestSkill(cwd: string): Promise<string> {
  const testDir = path.join(cwd, '.gen', 'skills', 'test-skill');
  await fs.mkdir(testDir, { recursive: true });

  const skillContent = `---
name: test-skill
description: A test skill for functional verification
allowed-tools: [Read, Grep]
---

# Test Skill

This is a test skill used for functional verification.

## Instructions

When activated with arguments: {{args}}

This skill should demonstrate:
1. Proper activation
2. Argument substitution
3. Content injection

## Example

Use this skill to verify the Skills system is working correctly.
`;

  const skillPath = path.join(testDir, 'SKILL.md');
  await fs.writeFile(skillPath, skillContent, 'utf-8');

  return skillPath;
}

async function cleanupTestSkill(cwd: string) {
  const testDir = path.join(cwd, '.gen', 'skills', 'test-skill');
  await fs.rm(testDir, { recursive: true, force: true });
}

async function testSkillActivation() {
  console.log('Skills Functional Verification Test');
  console.log('====================================\n');

  const cwd = process.cwd();
  let skillPath: string | undefined;

  try {
    // Setup: Create test skill
    console.log('Setup: Creating test skill...');
    skillPath = await setupTestSkill(cwd);
    console.log(`Created test skill at ${skillPath}\n`);

    // Reset discovery to pick up new test skill
    resetSkillDiscovery();

    // Test 1: Create Skill tool
    console.log('Test 1: Creating Skill tool...');
    let skillTool;
    try {
      skillTool = await createSkillTool(cwd);
      logTest('Skill tool created successfully', true);
    } catch (error) {
      logTest(
        'Skill tool creation failed',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
      return;
    }

    // Test 2: Activate existing skill
    console.log('\nTest 2: Activating existing skill...');
    try {
      const result1 = await skillTool.execute(
        { skill: 'test-skill', args: 'test-argument' },
        { cwd, sessionId: 'test' }
      );

      if (result1.error) {
        logTest('Skill activation returned error', false, undefined, result1.error);
      } else {
        const hasContent = result1.output && result1.output.length > 0;
        const hasMetadata = result1.metadata?.title === 'Skill: test-skill';
        logTest(
          'Skill activated successfully',
          hasContent && hasMetadata,
          `Output length: ${result1.output?.length || 0}, Has metadata: ${hasMetadata}`
        );
      }
    } catch (error) {
      logTest(
        'Skill activation threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 3: Verify argument passing
    console.log('\nTest 3: Verifying argument passing...');
    try {
      const result2 = await skillTool.execute(
        { skill: 'test-skill', args: 'custom-argument-123' },
        { cwd, sessionId: 'test' }
      );

      const hasArgs = result2.output?.includes('custom-argument-123');
      logTest(
        'Arguments correctly passed to skill',
        hasArgs || false,
        hasArgs ? 'Found argument in output' : 'Argument not found in output'
      );
    } catch (error) {
      logTest(
        'Argument passing test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 4: Activate non-existent skill
    console.log('\nTest 4: Activating non-existent skill...');
    try {
      const result3 = await skillTool.execute(
        { skill: 'non-existent-skill' },
        { cwd, sessionId: 'test' }
      );

      if (result3.error) {
        const hasNotFoundMsg = result3.error.includes('Skill not found');
        logTest(
          'Non-existent skill returns appropriate error',
          hasNotFoundMsg,
          hasNotFoundMsg ? 'Correct error message' : 'Unexpected error format'
        );
      } else {
        logTest('Non-existent skill should return error but succeeded', false);
      }
    } catch (error) {
      logTest(
        'Non-existent skill test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

    // Test 5: Verify skill metadata
    console.log('\nTest 5: Verifying skill metadata...');
    try {
      const result4 = await skillTool.execute(
        { skill: 'test-skill' },
        { cwd, sessionId: 'test' }
      );

      const hasTitle = result4.metadata?.title === 'Skill: test-skill';
      const hasSubtitle = result4.metadata?.subtitle === 'A test skill for functional verification';
      logTest(
        'Skill metadata correctly populated',
        hasTitle && hasSubtitle,
        `Title: ${result4.metadata?.title}, Subtitle: ${result4.metadata?.subtitle}`
      );
    } catch (error) {
      logTest(
        'Metadata test threw exception',
        false,
        undefined,
        error instanceof Error ? error.message : String(error)
      );
    }

  } finally {
    // Cleanup
    if (skillPath) {
      console.log('\nCleanup: Removing test skill...');
      await cleanupTestSkill(cwd);
    }
  }

  // Print summary
  console.log('\n====================================');
  console.log('Test Summary');
  console.log('====================================');
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

testSkillActivation().catch((error) => {
  console.error('Fatal error during Skills functional test:', error);
  process.exit(1);
});
