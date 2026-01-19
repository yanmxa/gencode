/**
 * Tool System Type Definitions
 */

import * as path from 'path';
import { z } from 'zod';

// ============================================================================
// Tool Definition Types
// ============================================================================

// Forward declaration for Question type (used by AskUserQuestion tool)
export interface Question {
  question: string;
  header: string;
  options: Array<{ label: string; description: string }>;
  multiSelect: boolean;
}

// Forward declaration for QuestionAnswer type (used by AskUserQuestion tool)
export interface QuestionAnswer {
  question: string;
  header: string;
  selectedOptions: string[];
  customInput?: string;
}

export interface ToolContext {
  cwd: string;
  sessionId?: string;
  abortSignal?: AbortSignal;
  /** Callback for AskUserQuestion tool to interact with user */
  askUser?: (questions: Question[]) => Promise<QuestionAnswer[]>;
  /** Current agent's provider (for Task tool to inherit) */
  currentProvider?: string;
  /** Current agent's model (for Task tool to inherit) */
  currentModel?: string;
  /** Current agent's auth method (for Task tool to inherit) */
  currentAuthMethod?: string;
}

export interface ToolResultMetadata {
  title?: string; // Short title, e.g., "Fetch(url)"
  subtitle?: string; // Subtitle, e.g., "Received 540.3KB (200 OK)"
  size?: number; // Response size in bytes
  statusCode?: number; // HTTP status code
  contentType?: string; // Content-Type header
  duration?: number; // Duration in milliseconds
}

export interface ToolResult {
  success: boolean;
  output: string;
  error?: string;
  metadata?: ToolResultMetadata;
}

export interface Tool<TInput = unknown> {
  name: string;
  description: string;
  parameters: z.ZodSchema<TInput>;
  execute(input: TInput, context: ToolContext): Promise<ToolResult>;
}

// ============================================================================
// Helper Functions
// ============================================================================

/**
 * Resolve a file path relative to the context's working directory
 */
export function resolvePath(filePath: string, cwd: string): string {
  return path.isAbsolute(filePath) ? filePath : path.resolve(cwd, filePath);
}

/**
 * Extract error message from unknown error
 */
export function getErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

// ============================================================================
// Built-in Tool Input Types
// ============================================================================

export const ReadInputSchema = z.object({
  file_path: z.string().describe('The absolute path to the file to read'),
  offset: z.number().optional().describe('Line number to start reading from (1-based)'),
  limit: z.number().optional().describe('Number of lines to read'),
});
export type ReadInput = z.infer<typeof ReadInputSchema>;

export const WriteInputSchema = z.object({
  file_path: z.string().describe('The absolute path to the file to write'),
  content: z.string().describe('The content to write to the file'),
});
export type WriteInput = z.infer<typeof WriteInputSchema>;

export const EditInputSchema = z.object({
  file_path: z.string().describe('The absolute path to the file to modify'),
  old_string: z.string().describe('The text to replace'),
  new_string: z.string().describe('The replacement text'),
});
export type EditInput = z.infer<typeof EditInputSchema>;

export const BashInputSchema = z.object({
  command: z.string().describe('The bash command to execute'),
  timeout: z.number().optional().describe('Timeout in milliseconds (default: 30000)'),
  run_in_background: z.boolean().optional().describe('Run command in background and return task ID'),
  description: z.string().optional().describe('Description of the task (used for background tasks)'),
});
export type BashInput = z.infer<typeof BashInputSchema>;

export const GlobInputSchema = z.object({
  pattern: z.string().describe('The glob pattern to match files'),
  path: z.string().optional().describe('The directory to search in'),
});
export type GlobInput = z.infer<typeof GlobInputSchema>;

export const GrepInputSchema = z.object({
  pattern: z.string().describe('The regex pattern to search for'),
  path: z.string().optional().describe('The file or directory to search in'),
  include: z.string().optional().describe('File pattern to include (e.g., "*.ts")'),
});
export type GrepInput = z.infer<typeof GrepInputSchema>;

export const WebFetchInputSchema = z.object({
  url: z.string().describe('The URL to fetch content from (http:// or https://)'),
  format: z
    .enum(['text', 'markdown', 'html'])
    .optional()
    .describe('Output format: markdown (default), text, or html'),
  timeout: z.number().optional().describe('Timeout in seconds (default: 30, max: 120)'),
});
export type WebFetchInput = z.infer<typeof WebFetchInputSchema>;

export const TodoItemSchema = z.object({
  content: z.string().min(1).describe('The task description'),
  status: z.enum(['pending', 'in_progress', 'completed']).describe('Current status of the task'),
  id: z.string().optional().describe('Unique task identifier'),
});
export type TodoItem = z.infer<typeof TodoItemSchema>;

export const TodoWriteInputSchema = z.object({
  todos: z.array(TodoItemSchema).describe('The complete todo list to write'),
});
export type TodoWriteInput = z.infer<typeof TodoWriteInputSchema>;

// ============================================================================
// JSON Schema Conversion
// ============================================================================

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export function zodToJsonSchema(schema: z.ZodSchema<any>): Record<string, unknown> {
  // Use Zod's built-in JSON schema support or manual conversion
  try {
    // Check if toJSONSchema method exists on the schema itself
    if ('toJSONSchema' in schema && typeof (schema as any).toJSONSchema === 'function') {
      return (schema as any).toJSONSchema() as Record<string, unknown>;
    }
  } catch {
    // Fall through to manual conversion
  }

  // Manual conversion for object schemas
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const def = (schema as any)._zod ?? (schema as any)._def;
  if (def?.typeName === 'ZodObject' || def?.def?.typeName === 'object') {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const shape = (schema as any).shape ?? (schema as any)._zod?.def?.shape;
    if (shape) {
      const properties: Record<string, unknown> = {};
      const required: string[] = [];

      for (const [key, value] of Object.entries(shape)) {
        properties[key] = zodFieldToJsonSchema(value);
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const valDef = (value as any)._zod ?? (value as any)._def;
        const isOptional = valDef?.typeName === 'ZodOptional' || valDef?.def?.typeName === 'optional';
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
  }

  return { type: 'object' };
}

function zodFieldToJsonSchema(field: unknown): Record<string, unknown> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const f = field as any;
  const def = f._zod ?? f._def;
  const description = def?.def?.description ?? def?.description;

  // Get the inner type for optionals
  let typeName = def?.typeName ?? def?.def?.typeName ?? 'string';
  let innerField = field;

  // Unwrap optional
  if (typeName === 'ZodOptional' || typeName === 'optional') {
    const inner = def?.def?.innerType ?? def?.innerType;
    if (inner) {
      innerField = inner;
      const innerDef = inner._zod ?? inner._def;
      typeName = innerDef?.typeName ?? innerDef?.def?.typeName ?? 'string';
    }
  }

  // Handle ZodObject - nested objects
  if (typeName === 'ZodObject' || typeName === 'object') {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const shape = (innerField as any).shape ?? (innerField as any)._zod?.def?.shape ?? (innerField as any)._def?.shape;
    if (shape) {
      const properties: Record<string, unknown> = {};
      const required: string[] = [];

      for (const [key, value] of Object.entries(shape)) {
        properties[key] = zodFieldToJsonSchema(value);
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const valDef = (value as any)._zod ?? (value as any)._def;
        const isOptional = valDef?.typeName === 'ZodOptional' || valDef?.def?.typeName === 'optional';
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
  }

  // Handle ZodEnum - enum types
  if (typeName === 'ZodEnum' || typeName === 'enum') {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const values = (innerField as any)._def?.values ?? (innerField as any)._zod?.def?.values ?? def?.def?.values ?? def?.values;
    if (values && Array.isArray(values)) {
      return {
        type: 'string',
        enum: values,
        description,
      };
    }
  }

  // Handle ZodArray - arrays
  if (typeName === 'ZodArray' || typeName === 'array') {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const items = (innerField as any)._def?.type ?? (innerField as any)._zod?.def?.type ?? def?.def?.type ?? def?.type ?? def?.def?.element ?? def?.element;
    return {
      type: 'array',
      items: items ? zodFieldToJsonSchema(items) : { type: 'string' },
      description,
    };
  }

  // Map Zod types to JSON Schema types
  let type = 'string';
  if (typeName === 'ZodNumber' || typeName === 'number') {
    type = 'number';
  } else if (typeName === 'ZodBoolean' || typeName === 'boolean') {
    type = 'boolean';
  }

  return { type, description };
}
