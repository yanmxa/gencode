#!/usr/bin/env node
/**
 * Skills Loading Test
 *
 * Tests skills discovery from all configured directories:
 * - ~/.claude/skills/
 * - ~/.gen/skills/
 * - .claude/skills/
 * - .gen/skills/
 *
 * Usage:
 *   npm run test:skills                  # Normal mode
 *   GEN_DEBUG_SKILLS=1 npm run test:skills  # Debug mode
 *   npm run test:skills -- --verbose     # Show all skills
 */

import { SkillDiscovery } from '../src/extensions/skills/discovery.js';
import { logger } from '../src/infrastructure/utils/logger.js';
import * as path from 'node:path';

const args = process.argv.slice(2);
const verbose = args.includes('--verbose');

async function main(): Promise<void> {
  console.log('Skills Discovery Test\n');

  const cwd = process.cwd();
  const discovery = new SkillDiscovery();

  try {
    logger.info('SkillsTest', 'Starting skills discovery', { cwd });

    const startTime = Date.now();
    const skills = await discovery.loadAll(cwd);
    const duration = Date.now() - startTime;

    console.log(`✓ Loaded ${skills.size} skills in ${duration}ms\n`);

    if (verbose || skills.size <= 20) {
      console.log('Discovered skills:');
      for (const [name, skill] of skills) {
        console.log(`  - ${name}`);
        console.log(`    Description: ${skill.description}`);
        if (skill.allowedTools) {
          const tools = Array.isArray(skill.allowedTools)
            ? skill.allowedTools.join(', ')
            : skill.allowedTools;
          console.log(`    Allowed tools: ${tools}`);
        }
        console.log();
      }
    } else {
      console.log('Top 10 skills (use --verbose to see all):');
      let count = 0;
      for (const [name, skill] of skills) {
        if (count++ >= 10) break;
        console.log(`  - ${name}: ${skill.description}`);
      }
      console.log();
    }

    process.exit(0);
  } catch (error) {
    logger.error('SkillsTest', 'Skills discovery failed', {
      error: error instanceof Error ? error.message : String(error),
    });
    console.error('✗ Failed to load skills');
    if (error instanceof Error && error.stack) {
      console.error(error.stack);
    }
    process.exit(1);
  }
}

main();
