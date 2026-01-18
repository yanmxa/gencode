/**
 * Test Task tool schema generation for Google provider
 */

import { taskTool } from './dist/subagents/task-tool.js';
import { zodToJsonSchema } from './dist/tools/types.js';
import { GoogleProvider } from './dist/providers/google.js';

// Test 1: Check Task tool schema
console.log('=== Task Tool Schema ===');
const schema = zodToJsonSchema(taskTool.parameters);
console.log(JSON.stringify(schema, null, 2));

// Test 2: Check if Google provider can handle the schema
console.log('\n=== Testing Google Provider ===');

const toolDef = {
  name: taskTool.name,
  description: taskTool.description,
  parameters: schema,
};

console.log('\nTool Definition:');
console.log(JSON.stringify(toolDef, null, 2));

// Test 3: Try to convert to Google's format
try {
  const provider = new GoogleProvider();

  // Use reflection to access private method
  const convertTools = (provider as any).convertTools.bind(provider);
  const googleTools = convertTools([toolDef]);

  console.log('\n=== Google Tools Format ===');
  console.log(JSON.stringify(googleTools, null, 2));
} catch (error) {
  console.error('\n=== Error ===');
  console.error(error);
}

// Test 4: Make actual API call with simplified prompt
console.log('\n=== Making API Call ===');

async function testApiCall() {
  try {
    const provider = new GoogleProvider();

    const result = await provider.complete({
      model: 'gemini-2.0-flash-exp',
      systemPrompt: 'You are a helpful assistant.',
      messages: [
        {
          role: 'user',
          content: 'Use the Task tool to explore authentication patterns.',
        },
      ],
      tools: [toolDef],
      maxTokens: 1000,
    });

    console.log('\nAPI call successful!');
    console.log('Stop reason:', result.stopReason);
    console.log('Content:', JSON.stringify(result.content, null, 2));
  } catch (error) {
    console.error('\nAPI call failed:');
    console.error(error);
  }
}

testApiCall();
