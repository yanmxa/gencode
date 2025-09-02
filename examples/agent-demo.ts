/**
 * Agent Demo - Demonstrates the full agent loop with tools
 *
 * Run with: npx tsx examples/agent-demo.ts
 */

import 'dotenv/config';
import { Agent } from '../src/agent/index.js';
import type { AgentEvent } from '../src/agent/types.js';
import chalk from 'chalk';

// Setup proxy if configured
const proxyUrl = process.env.HTTPS_PROXY || process.env.HTTP_PROXY;
if (proxyUrl) {
  const { setGlobalDispatcher, ProxyAgent } = await import('undici');
  setGlobalDispatcher(new ProxyAgent(proxyUrl));
  console.log(chalk.dim(`Using proxy: ${proxyUrl}\n`));
}

// Detect provider
function getConfig() {
  if (process.env.ANTHROPIC_API_KEY) {
    return { provider: 'anthropic' as const, model: 'claude-sonnet-4-20250514' };
  } else if (process.env.OPENAI_API_KEY) {
    return { provider: 'openai' as const, model: 'gpt-4o' };
  } else if (process.env.GOOGLE_API_KEY || process.env.GEMINI_API_KEY) {
    return { provider: 'gemini' as const, model: 'gemini-2.0-flash' };
  }
  throw new Error('No API key found. Set OPENAI_API_KEY, ANTHROPIC_API_KEY, or GOOGLE_API_KEY');
}

async function main() {
  console.log(chalk.cyan.bold('\n╭─────────────────────────────────────╮'));
  console.log(chalk.cyan.bold('│') + '  ' + chalk.white.bold('Recode Agent Demo') + '                  ' + chalk.cyan.bold('│'));
  console.log(chalk.cyan.bold('╰─────────────────────────────────────╯\n'));

  const config = getConfig();
  console.log(chalk.blue('ℹ') + ` Provider: ${chalk.bold(config.provider)} | Model: ${chalk.bold(config.model)}`);
  console.log(chalk.blue('ℹ') + ` Working directory: ${chalk.dim(process.cwd())}\n`);

  const agent = new Agent({
    ...config,
    cwd: process.cwd(),
    maxTurns: 10,
  });

  // Auto-approve all tools for demo (no confirmation prompts)
  agent.setConfirmCallback(async (tool, input) => {
    console.log(chalk.yellow('⚡') + ` Auto-approved: ${chalk.bold(tool)}`);
    return true;
  });

  // Demo prompt - ask agent to explore the project
  const prompt = `Please explore this project and tell me:
1. What is the project structure?
2. What are the main source files?
3. Give me a brief summary of what this project does.

Use the Glob and Read tools to explore.`;

  console.log(chalk.blue('▶ User:'));
  console.log(chalk.white('  ' + prompt.split('\n').join('\n  ')));
  console.log();

  let responseText = '';

  // Run the agent
  for await (const event of agent.run(prompt)) {
    switch (event.type) {
      case 'text':
        responseText += event.text;
        break;

      case 'tool_start':
        console.log(chalk.magenta('⚙ Tool:') + ` ${chalk.bold(event.name)}`);
        const inputStr = JSON.stringify(event.input);
        console.log(chalk.dim('  Input: ' + inputStr.slice(0, 80) + (inputStr.length > 80 ? '...' : '')));
        break;

      case 'tool_result':
        const status = event.result.success ? chalk.green('✓') : chalk.red('✗');
        console.log(status + chalk.dim(` ${event.name} completed`));
        // Show first few lines of output
        const lines = event.result.output.split('\n').slice(0, 5);
        for (const line of lines) {
          console.log(chalk.dim('  │ ') + line.slice(0, 80));
        }
        if (event.result.output.split('\n').length > 5) {
          console.log(chalk.dim('  │ ...'));
        }
        console.log();
        break;

      case 'error':
        console.log(chalk.red('✗ Error:') + ` ${event.error.message}`);
        break;

      case 'done':
        console.log(chalk.green('◀ Assistant:'));
        const respLines = responseText.split('\n');
        for (const line of respLines) {
          console.log('  ' + line);
        }
        break;
    }
  }

  console.log(chalk.dim('\n─'.repeat(50)));
  console.log(chalk.green('✓') + ' Demo completed!\n');
}

main().catch((error) => {
  console.error(chalk.red('Error:'), error.message);
  process.exit(1);
});
