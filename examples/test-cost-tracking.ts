/**
 * Test cost tracking functionality
 */

import { createProvider } from '../src/providers/index.js';
import { calculateCost, formatCost, formatTokens } from '../src/pricing/calculator.js';

async function testCostTracking() {
  console.log('='.repeat(60));
  console.log('Testing Cost Tracking Feature');
  console.log('='.repeat(60));
  console.log();

  // Test 1: Cost Calculation
  console.log('Test 1: Cost Calculation');
  console.log('-'.repeat(60));

  const testCases = [
    { provider: 'anthropic', model: 'claude-sonnet-4', input: 1000, output: 500 },
    { provider: 'openai', model: 'gpt-4o', input: 1000, output: 500 },
    { provider: 'gemini', model: 'gemini-2.0-flash', input: 1000, output: 500 },
  ];

  for (const test of testCases) {
    const cost = calculateCost(test.provider, test.model, {
      inputTokens: test.input,
      outputTokens: test.output,
    });

    console.log(`\n${test.provider}/${test.model}:`);
    console.log(`  Input: ${formatTokens(test.input)} tokens`);
    console.log(`  Output: ${formatTokens(test.output)} tokens`);
    console.log(`  Cost: ${formatCost(cost.totalCost)}`);
    console.log(`  Breakdown: ${formatCost(cost.inputCost)} (input) + ${formatCost(cost.outputCost)} (output)`);
  }

  console.log();
  console.log('='.repeat(60));
  console.log();

  // Test 2: Real API Call (if env var is set)
  if (process.env.ANTHROPIC_API_KEY) {
    console.log('Test 2: Real API Call with Anthropic');
    console.log('-'.repeat(60));

    const provider = createProvider({ provider: 'anthropic', model: 'claude-haiku-3-5' });

    const response = await provider.complete({
      model: 'claude-haiku-3-5',
      messages: [{ role: 'user', content: 'Say hello in one word' }],
      maxTokens: 10,
    });

    console.log('\nResponse:', response.content[0]?.type === 'text' ? response.content[0].text : '');

    if (response.usage) {
      console.log(`\nToken Usage:`);
      console.log(`  Input: ${formatTokens(response.usage.inputTokens)}`);
      console.log(`  Output: ${formatTokens(response.usage.outputTokens)}`);
    }

    if (response.cost) {
      console.log(`\nCost Estimate:`);
      console.log(`  Total: ${formatCost(response.cost.totalCost)}`);
      console.log(`  Input: ${formatCost(response.cost.inputCost)}`);
      console.log(`  Output: ${formatCost(response.cost.outputCost)}`);
    }

    console.log();
    console.log('='.repeat(60));
  } else {
    console.log('Skipping API test (no ANTHROPIC_API_KEY)');
    console.log('='.repeat(60));
  }
}

testCostTracking().catch(console.error);
