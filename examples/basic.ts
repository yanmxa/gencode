/**
 * Basic example demonstrating multi-LLM support
 *
 * Run with: npx tsx examples/basic.ts
 *
 * Create a .env file with your API keys (see .env.example)
 */

import 'dotenv/config';

// Setup proxy if configured
const proxyUrl = process.env.HTTPS_PROXY || process.env.HTTP_PROXY;
if (proxyUrl) {
  const { setGlobalDispatcher, ProxyAgent } = await import('undici');
  setGlobalDispatcher(new ProxyAgent(proxyUrl));
  console.log(`Using proxy: ${proxyUrl}\n`);
}

import {
  OpenAIProvider,
  AnthropicProvider,
  GeminiProvider,
  createProvider,
  inferProvider,
  type LLMProvider,
  type Message,
  type ToolDefinition,
} from '../src/index.js';

// Simple tool definition for testing
const tools: ToolDefinition[] = [
  {
    name: 'get_weather',
    description: 'Get the current weather for a location',
    parameters: {
      type: 'object',
      properties: {
        location: {
          type: 'string',
          description: 'The city and state, e.g., San Francisco, CA',
        },
        unit: {
          type: 'string',
          enum: ['celsius', 'fahrenheit'],
          description: 'Temperature unit',
        },
      },
      required: ['location'],
    },
  },
];

async function testProvider(provider: LLMProvider, model: string) {
  console.log(`\n${'='.repeat(60)}`);
  console.log(`Testing ${provider.name} with model: ${model}`);
  console.log('='.repeat(60));

  const messages: Message[] = [{ role: 'user', content: 'What is the weather in Tokyo?' }];

  try {
    // Test non-streaming completion
    console.log('\n--- Non-streaming completion ---');
    const response = await provider.complete({
      model,
      messages,
      tools,
      systemPrompt: 'You are a helpful assistant. Use tools when appropriate.',
      maxTokens: 1024,
    });

    console.log('Stop reason:', response.stopReason);
    console.log('Content:');
    for (const content of response.content) {
      if (content.type === 'text') {
        console.log(`  [text] ${content.text}`);
      } else if (content.type === 'tool_use') {
        console.log(`  [tool_use] ${content.name}(${JSON.stringify(content.input)})`);
      }
    }
    if (response.usage) {
      console.log(`Usage: ${response.usage.inputTokens} input, ${response.usage.outputTokens} output`);
    }

    // Test streaming completion
    console.log('\n--- Streaming completion ---');
    process.stdout.write('Response: ');
    for await (const chunk of provider.stream({
      model,
      messages: [{ role: 'user', content: 'Say "hello world" in Japanese.' }],
      maxTokens: 100,
    })) {
      if (chunk.type === 'text') {
        process.stdout.write(chunk.text);
      } else if (chunk.type === 'tool_start') {
        process.stdout.write(`\n[Tool: ${chunk.name}]`);
      } else if (chunk.type === 'done') {
        console.log(`\n(Stop: ${chunk.response.stopReason})`);
      }
    }
  } catch (error) {
    console.error(`Error: ${error instanceof Error ? error.message : error}`);
  }
}

async function main() {
  console.log('Recode - Multi-LLM Agent SDK Demo\n');

  // Test each provider if API key is available
  const tests: Array<{ provider: LLMProvider; model: string; envKey: string }> = [];

  if (process.env.OPENAI_API_KEY) {
    tests.push({
      provider: new OpenAIProvider(),
      model: 'gpt-4o-mini',
      envKey: 'OPENAI_API_KEY',
    });
  }

  if (process.env.ANTHROPIC_API_KEY) {
    tests.push({
      provider: new AnthropicProvider(),
      model: 'claude-3-5-sonnet-20241022',
      envKey: 'ANTHROPIC_API_KEY',
    });
  }

  if (process.env.GOOGLE_API_KEY || process.env.GEMINI_API_KEY) {
    tests.push({
      provider: new GeminiProvider(),
      model: 'gemini-2.0-flash',
      envKey: 'GOOGLE_API_KEY',
    });
  }

  if (tests.length === 0) {
    console.log('No API keys found. Please set at least one of:');
    console.log('  - OPENAI_API_KEY');
    console.log('  - ANTHROPIC_API_KEY');
    console.log('  - GOOGLE_API_KEY (or GEMINI_API_KEY)');
    process.exit(1);
  }

  console.log(`Found ${tests.length} provider(s) with API keys.\n`);

  // Test provider inference
  console.log('--- Provider Inference Test ---');
  console.log(`gpt-4o -> ${inferProvider('gpt-4o')}`);
  console.log(`claude-3-opus -> ${inferProvider('claude-3-opus')}`);
  console.log(`gemini-1.5-pro -> ${inferProvider('gemini-1.5-pro')}`);

  // Test createProvider factory (use first available provider)
  console.log('\n--- Factory Test ---');
  const firstProvider = tests[0];
  const providerName = firstProvider.provider.name as 'openai' | 'anthropic' | 'gemini';
  const factoryProvider = createProvider({ provider: providerName });
  console.log(`Created provider via factory: ${factoryProvider.name}`);

  // Run provider tests
  for (const test of tests) {
    await testProvider(test.provider, test.model);
  }

  console.log('\n\nAll tests completed!');
}

main().catch(console.error);
