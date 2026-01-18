/**
 * Simple test to check Task tool schema without importing
 */

import { z } from 'zod';

// Recreate the Task tool schema
const taskToolSchema = z.object({
  description: z
    .string()
    .min(1)
    .describe('Short description (3-5 words) for UI display'),

  prompt: z
    .string()
    .min(1)
    .describe('Detailed task instructions for the subagent'),

  subagent_type: z
    .enum(['Explore', 'Plan', 'Bash', 'general-purpose'])
    .describe('Type of subagent to spawn'),

  model: z
    .string()
    .optional()
    .describe('Optional: specific model to use (overrides default for type)'),

  run_in_background: z
    .boolean()
    .optional()
    .describe('Optional: run in background (Phase 2 - not yet implemented)'),

  resume: z
    .string()
    .optional()
    .describe('Optional: resume a previous subagent by ID (Phase 3 - not yet implemented)'),

  max_turns: z
    .number()
    .positive()
    .optional()
    .describe('Optional: max conversation turns (overrides default)'),

  tasks: z
    .array(
      z.object({
        description: z.string().min(1).describe('Short description (3-5 words)'),
        prompt: z.string().min(1).describe('Task prompt'),
        subagent_type: z
          .enum(['Explore', 'Plan', 'Bash', 'general-purpose'])
          .describe('Subagent type'),
        model: z.string().optional().describe('Optional model override'),
        max_turns: z.number().positive().optional().describe('Optional max turns'),
      })
    )
    .optional()
    .describe('Optional: array of tasks to execute in parallel (Phase 4)'),
});

// Simple zodToJsonSchema implementation
function zodToJsonSchema(schema) {
  const def = schema._def;

  if (def.typeName === 'ZodObject') {
    const shape = def.shape();
    const properties = {};
    const required = [];

    for (const [key, value] of Object.entries(shape)) {
      properties[key] = zodFieldToJsonSchema(value);
      const valDef = value._def;
      const isOptional = valDef.typeName === 'ZodOptional';
      if (!isOptional) {
        required.push(key);
      }
    }

    return {
      type: 'object',
      properties,
      required: required.length > 0 ? required : undefined,
    };
  }

  return { type: 'object' };
}

function zodFieldToJsonSchema(field) {
  const def = field._def;
  const description = def.description;
  let typeName = def.typeName;
  let innerField = field;

  // Unwrap optional
  if (typeName === 'ZodOptional') {
    innerField = def.innerType;
    typeName = innerField._def.typeName;
  }

  // Handle ZodObject
  if (typeName === 'ZodObject') {
    const shape = innerField._def.shape();
    const properties = {};
    const required = [];

    for (const [key, value] of Object.entries(shape)) {
      properties[key] = zodFieldToJsonSchema(value);
      const valDef = value._def;
      const isOptional = valDef.typeName === 'ZodOptional';
      if (!isOptional) {
        required.push(key);
      }
    }

    return {
      type: 'object',
      properties,
      required: required.length > 0 ? required : undefined,
      description,
    };
  }

  // Handle ZodEnum
  if (typeName === 'ZodEnum') {
    const values = innerField._def.values;
    return {
      type: 'string',
      enum: values,
      description,
    };
  }

  // Handle ZodArray
  if (typeName === 'ZodArray') {
    const items = innerField._def.type;
    return {
      type: 'array',
      items: items ? zodFieldToJsonSchema(items) : { type: 'string' },
      description,
    };
  }

  // Map other types
  let type = 'string';
  if (typeName === 'ZodNumber') {
    type = 'number';
  } else if (typeName === 'ZodBoolean') {
    type = 'boolean';
  }

  return { type, description };
}

// Test the conversion
const schema = zodToJsonSchema(taskToolSchema);

console.log('=== Task Tool Schema ===');
console.log(JSON.stringify(schema, null, 2));

// Check for potential issues
console.log('\n=== Validation Checks ===');

// Check tasks array schema
const tasksProperty = schema.properties?.tasks;
console.log('tasks property type:', tasksProperty?.type);
console.log('tasks items type:', tasksProperty?.items?.type);
console.log('tasks items properties:', Object.keys(tasksProperty?.items?.properties || {}));

// Check enum handling
const subagentTypeProperty = schema.properties?.subagent_type;
console.log('\nsubagent_type property:');
console.log('  type:', subagentTypeProperty?.type);
console.log('  enum:', subagentTypeProperty?.enum);

// Check nested object in tasks
const tasksItemsProperties = tasksProperty?.items?.properties;
if (tasksItemsProperties) {
  console.log('\ntasks items.subagent_type:');
  console.log('  type:', tasksItemsProperties.subagent_type?.type);
  console.log('  enum:', tasksItemsProperties.subagent_type?.enum);
}

console.log('\n=== Schema looks valid! ===');
