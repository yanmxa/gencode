/**
 * Direct test of Task tool with Google provider
 */

process.env.DEBUG_SCHEMA = '1';
process.env.DEBUG_TOKENS = '1';

import { Agent } from './dist/agent/agent.js';
import { createDefaultRegistry } from './dist/tools/index.js';

async function test() {
  try {
    console.log('Creating agent...');

    const registry = await createDefaultRegistry(process.cwd());

    const agent = new Agent({
      provider: 'google',
      model: 'gemini-2.0-flash-exp',
      systemPrompt: 'You are a helpful assistant.',
      toolRegistry: registry,
      cwd: process.cwd(),
    });

    console.log('\nSending prompt...');

    const stream = agent.run([
      {
        role: 'user',
        content: 'Use the Task tool to explore authentication patterns in this codebase',
      },
    ]);

    for await (const event of stream) {
      if (event.type === 'text') {
        process.stdout.write(event.text);
      } else if (event.type === 'tool_start') {
        console.log(`\n[Tool: ${event.name}]`);
      } else if (event.type === 'error') {
        console.error('\n[Error]', event.error);
      } else if (event.type === 'done') {
        console.log('\n\n[Done]');
        console.log('Stop reason:', event.response.stopReason);
        if (event.response.usage) {
          console.log('Usage:', event.response.usage);
        }
      }
    }

    console.log('\nTest completed successfully!');
  } catch (error) {
    console.error('\nTest failed:');
    console.error(error);
    process.exit(1);
  }
}

test();
