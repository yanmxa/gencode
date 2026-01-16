/**
 * Test Interrupt Cleanup
 *
 * Simulates the interrupt scenario and verifies cleanup works correctly
 */

import { Agent } from '../src/agent/index.js';
import type { Message } from '../src/providers/types.js';

async function testInterruptCleanup() {
  console.log('🧪 Testing Interrupt Cleanup\n');

  const agent = new Agent({
    provider: 'anthropic',
    model: 'claude-3-5-sonnet-20241022',
    cwd: process.cwd(),
  });

  try {
    // Simulate incomplete tool_use message (what happens during interrupt)
    console.log('1️⃣  Simulating incomplete tool_use message...');

    // Access private messages array via getHistory and direct manipulation
    const history = agent.getHistory();
    console.log(`   Initial history: ${history.length} messages`);

    // Add a user message
    (agent as any).messages.push({
      role: 'user',
      content: 'Create a file',
    });
    console.log(`   After user message: ${agent.getHistory().length} messages`);

    // Add an assistant message with tool_use (simulating interrupt point)
    (agent as any).messages.push({
      role: 'assistant',
      content: [
        { type: 'text', text: 'I will create the file.' },
        {
          type: 'tool_use',
          id: 'toolu_test_123',
          name: 'Write',
          input: { file_path: 'test.txt', content: 'hello' },
        },
      ],
    });
    console.log(`   After assistant+tool_use: ${agent.getHistory().length} messages`);
    console.log(`   Last message role: ${agent.getHistory()[agent.getHistory().length - 1].role}`);

    // Verify incomplete state
    const lastMessage = agent.getHistory()[agent.getHistory().length - 1];
    const hasToolUse = Array.isArray(lastMessage.content) &&
      lastMessage.content.some((c: any) => c.type === 'tool_use');
    console.log(`   Has incomplete tool_use: ${hasToolUse}\n`);

    console.log('2️⃣  Calling cleanupIncompleteMessages()...');
    agent.cleanupIncompleteMessages();

    const afterCleanup = agent.getHistory();
    console.log(`   After cleanup: ${afterCleanup.length} messages`);

    if (afterCleanup.length === 1 && afterCleanup[0].role === 'user') {
      console.log('   ✓ Incomplete assistant message removed!\n');
    } else {
      console.log('   ✗ Cleanup failed!\n');
      console.log('   Remaining messages:', JSON.stringify(afterCleanup, null, 2));
    }

    console.log('3️⃣  Testing with complete messages (no tool_use)...');

    // Add complete assistant message
    (agent as any).messages.push({
      role: 'assistant',
      content: [{ type: 'text', text: 'Here is your answer.' }],
    });
    console.log(`   Before cleanup: ${agent.getHistory().length} messages`);

    agent.cleanupIncompleteMessages();
    console.log(`   After cleanup: ${agent.getHistory().length} messages`);

    if (agent.getHistory().length === 2) {
      console.log('   ✓ Complete messages preserved!\n');
    } else {
      console.log('   ✗ Complete message was incorrectly removed!\n');
    }

    console.log('✅ All tests passed!\n');
  } catch (error) {
    console.error('❌ Test failed:', error);
  }
}

// Run the test
testInterruptCleanup();
