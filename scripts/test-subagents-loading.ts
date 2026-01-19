#!/usr/bin/env node
/**
 * Subagents Loading Test
 *
 * Tests custom agent discovery from hierarchical directories:
 * - ~/.claude/agents/ (JSON and MD formats)
 * - ~/.gen/agents/ (JSON and MD formats)
 * - .claude/agents/ (project-level)
 * - .gen/agents/ (project-level)
 *
 * Priority: project gen > project claude > user gen > user claude
 *
 * Usage:
 *   npm run test:subagents                      # Normal mode
 *   GEN_DEBUG_SUBAGENTS=1 npm run test:subagents  # Debug mode
 *   npm run test:subagents -- --verbose         # Show all agents
 */

import { CustomAgentLoader } from '../src/extensions/subagents/custom-agent-loader.js';
import { logger } from '../src/base/utils/logger.js';

const args = process.argv.slice(2);
const verbose = args.includes('--verbose');

async function main(): Promise<void> {
  console.log('Subagents Discovery Test\n');

  const cwd = process.cwd();
  const loader = new CustomAgentLoader();

  try {
    logger.info('SubagentsTest', 'Starting subagents discovery', { cwd });

    const startTime = Date.now();
    await loader.initialize(cwd);
    const agents = loader.getAllAgents();
    const duration = Date.now() - startTime;

    console.log(`✓ Loaded ${agents.length} custom agents in ${duration}ms\n`);

    if (verbose || agents.length <= 20) {
      console.log('Discovered custom agents:');
      for (const agent of agents) {
        console.log(`  - ${agent.name}`);
        console.log(`    Description: ${agent.description}`);
        console.log(`    Model: ${agent.defaultModel}`);
        console.log(`    Max turns: ${agent.maxTurns}`);
        console.log(`    Allowed tools: ${agent.allowedTools.join(', ')}`);
        console.log();
      }
    } else {
      console.log('Top 10 custom agents (use --verbose to see all):');
      for (let i = 0; i < Math.min(10, agents.length); i++) {
        const agent = agents[i];
        console.log(`  - ${agent.name}: ${agent.description}`);
      }
      console.log();
    }

    // Check for built-in agents
    const builtInCount = agents.filter(a =>
      ['Bash', 'general-purpose', 'Explore', 'Plan'].includes(a.name)
    ).length;
    console.log(`Built-in agents: ${builtInCount}`);
    console.log(`Custom agents: ${agents.length - builtInCount}\n`);

    process.exit(0);
  } catch (error) {
    logger.error('SubagentsTest', 'Subagents discovery failed', {
      error: error instanceof Error ? error.message : String(error),
    });
    console.error('✗ Failed to load subagents');
    if (error instanceof Error && error.stack) {
      console.error(error.stack);
    }
    process.exit(1);
  }
}

main();
