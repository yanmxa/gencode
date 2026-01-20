/**
 * Configuration Validator
 *
 * Provides Zod schemas for validating various configuration files:
 * - SKILL.md frontmatter
 * - Command markdown frontmatter
 * - Custom agent JSON/MD files
 * - Hooks configuration
 */

import { z } from 'zod';
import { logger } from './logger.js';

/**
 * SKILL.md frontmatter schema
 */
export const SkillFrontmatterSchema = z.object({
  name: z.string().min(1, 'Skill name is required'),
  description: z.string().min(1, 'Skill description is required'),
  'allowed-tools': z
    .union([z.string(), z.array(z.string())])
    .optional()
    .describe('Tools allowed for this skill (string or array)'),
  version: z.string().optional(),
  author: z.string().optional(),
  tags: z.array(z.string()).optional(),
});

export type SkillFrontmatter = z.infer<typeof SkillFrontmatterSchema>;

/**
 * Command markdown frontmatter schema
 */
export const CommandFrontmatterSchema = z.object({
  name: z
    .string()
    .min(1)
    .optional()
    .describe('Command name (optional, defaults to filename)'),
  description: z.string().min(1, 'Command description is required'),
  'allowed-tools': z
    .union([z.string(), z.array(z.string())])
    .optional()
    .describe('Tools allowed for this command'),
  'argument-hint': z.string().optional().describe('Hint for command arguments'),
  model: z.string().optional().describe('Preferred model for this command'),
  includes: z
    .union([z.string(), z.array(z.string())])
    .optional()
    .describe('Files to include inline (relative paths)'),
});

export type CommandFrontmatter = z.infer<typeof CommandFrontmatterSchema>;

/**
 * Custom agent schema (JSON format)
 */
export const CustomAgentSchema = z.object({
  name: z.string().min(1, 'Agent name is required'),
  description: z.string().min(1, 'Agent description is required'),
  allowedTools: z
    .array(z.string())
    .min(1, 'At least one allowed tool is required'),
  defaultModel: z.string().default('claude-sonnet-4'),
  maxTurns: z.number().positive().default(20),
  permissionMode: z.enum(['strict', 'permissive']).default('strict').optional(),
  systemPrompt: z.string().min(1, 'System prompt is required'),
});

export type CustomAgent = z.infer<typeof CustomAgentSchema>;

/**
 * Hook configuration schema
 */
export const HookSchema = z.object({
  event: z.enum([
    'ToolCall',
    'AgentStart',
    'AgentEnd',
    'UserPromptSubmit',
    'Notification',
    'Stop',
  ]),
  matcher: z
    .object({
      tool: z.string().optional(),
      pattern: z.string().optional(),
    })
    .optional(),
  command: z.string().min(1, 'Hook command is required'),
  blocking: z.boolean().default(false).optional(),
  timeout: z.number().positive().default(5000).optional(),
});

export type Hook = z.infer<typeof HookSchema>;

/**
 * Hooks configuration schema
 */
export const HooksConfigSchema = z.object({
  hooks: z.array(HookSchema),
});

export type HooksConfig = z.infer<typeof HooksConfigSchema>;

/**
 * Validation result
 */
export interface ValidationResult<T> {
  valid: boolean;
  data?: T;
  errors?: string[];
}

/**
 * Validate configuration data against a Zod schema
 *
 * @param schema - Zod schema to validate against
 * @param data - Data to validate
 * @param context - Context for error messages (e.g., file path)
 * @returns Validation result with parsed data or errors
 */
export function validateConfig<T>(
  schema: z.ZodSchema<T>,
  data: unknown,
  context: string
): ValidationResult<T> {
  try {
    const validated = schema.parse(data);
    return { valid: true, data: validated };
  } catch (error) {
    if (error instanceof z.ZodError) {
      const errors = error.issues.map((e: z.ZodIssue) => {
        const path = e.path.join('.');
        return path ? `${path}: ${e.message}` : e.message;
      });

      logger.error('Validator', `Invalid configuration in ${context}`, { errors });

      return { valid: false, errors };
    }

    // Unknown error
    const errorMsg = error instanceof Error ? error.message : String(error);
    logger.error('Validator', `Validation failed for ${context}`, {
      error: errorMsg,
    });

    return { valid: false, errors: [errorMsg] };
  }
}

/**
 * Validate skill frontmatter
 */
export function validateSkillFrontmatter(
  data: unknown,
  filePath: string
): ValidationResult<SkillFrontmatter> {
  return validateConfig(SkillFrontmatterSchema, data, filePath);
}

/**
 * Validate command frontmatter
 */
export function validateCommandFrontmatter(
  data: unknown,
  filePath: string
): ValidationResult<CommandFrontmatter> {
  return validateConfig(CommandFrontmatterSchema, data, filePath);
}

/**
 * Validate custom agent configuration
 */
export function validateCustomAgent(
  data: unknown,
  filePath: string
): ValidationResult<CustomAgent> {
  return validateConfig(CustomAgentSchema, data, filePath);
}

/**
 * Validate hooks configuration
 */
export function validateHooksConfig(
  data: unknown,
  filePath: string
): ValidationResult<HooksConfig> {
  return validateConfig(HooksConfigSchema, data, filePath);
}
