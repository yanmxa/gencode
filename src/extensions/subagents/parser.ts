/**
 * Custom Agent Parser - Parse agent configuration files
 *
 * Supports two formats:
 * 1. JSON: ~/.gen/agents/my-agent.json
 * 2. Markdown with frontmatter: ~/.gen/agents/my-agent.md
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import { z } from 'zod';
import type { ResourceParser, ResourceLevel, ResourceNamespace } from '../../base/discovery/types.js';
import type { CustomAgentDefinition } from './types.js';
import { logger } from '../../base/utils/logger.js';

/**
 * Custom agent configuration schema
 */
const CustomAgentConfigSchema = z.object({
  name: z.string().min(1).describe('Agent name (used as identifier)'),
  type: z.literal('custom').optional().describe('Must be "custom" if provided'),
  description: z.string().min(1).describe('Agent description'),
  allowedTools: z.array(z.string()).min(1).describe('Tools this agent can use'),
  defaultModel: z.string().describe('Default model to use'),
  maxTurns: z.number().positive().describe('Maximum conversation turns'),
  permissionMode: z
    .enum(['inherit', 'isolated', 'permissive'])
    .optional()
    .describe('Permission mode'),
  systemPrompt: z.string().min(1).describe('System prompt for the agent'),
});

type RawAgentConfig = z.infer<typeof CustomAgentConfigSchema>;

/**
 * Parse markdown agent file with YAML frontmatter
 */
function parseMarkdownAgent(content: string, filename: string): unknown {
  // Extract frontmatter from markdown
  const frontmatterMatch = content.match(/^---\n([\s\S]*?)\n---/);
  if (!frontmatterMatch) {
    // Try to parse as JSON if no frontmatter
    try {
      return JSON.parse(content);
    } catch {
      throw new Error('No valid frontmatter or JSON found in markdown agent file');
    }
  }

  const frontmatter = frontmatterMatch[1];
  const config: Record<string, unknown> = {};

  // Parse YAML-like frontmatter (simple key: value pairs)
  const lines = frontmatter.split('\n');
  for (const line of lines) {
    const match = line.match(/^(\w+):\s*(.+)$/);
    if (match) {
      const [, key, value] = match;
      // Try to parse value as JSON, otherwise keep as string
      try {
        config[key] = JSON.parse(value);
      } catch {
        config[key] = value.trim();
      }
    }
  }

  // Extract system prompt from markdown body (after frontmatter)
  const bodyMatch = content.match(/^---\n[\s\S]*?\n---\n([\s\S]*)/);
  if (bodyMatch && !config.systemPrompt) {
    config.systemPrompt = bodyMatch[1].trim();
  }

  // Set defaults if not specified
  if (!config.name) {
    config.name = path.basename(filename, path.extname(filename));
  }
  if (!config.type) {
    config.type = 'custom';
  }

  return config;
}

/**
 * Custom Agent Parser - implements ResourceParser interface
 */
export class CustomAgentParser implements ResourceParser<CustomAgentDefinition> {
  async parse(
    filePath: string,
    level: ResourceLevel,
    namespace: ResourceNamespace
  ): Promise<CustomAgentDefinition | null> {
    try {
      const content = await fs.readFile(filePath, 'utf-8');
      const ext = path.extname(filePath);

      // Parse based on file extension
      let rawConfig: unknown;
      if (ext === '.json') {
        rawConfig = JSON.parse(content);
      } else if (ext === '.md') {
        rawConfig = parseMarkdownAgent(content, path.basename(filePath));
      } else {
        logger.warn('CustomAgent', `Unsupported agent file extension`, {
          file: filePath,
          extension: ext,
          hint: 'Use .json or .md format',
        });
        return null;
      }

      // Validate schema
      try {
        const validated = CustomAgentConfigSchema.parse(rawConfig);
      } catch (validationError) {
        if (validationError instanceof z.ZodError) {
          const errors = validationError.issues.map((e: z.ZodIssue) => `${e.path.join('.')}: ${e.message}`);
          logger.error('CustomAgent', `Invalid agent configuration`, {
            file: filePath,
            errors: errors.join(', '),
          });
        }
        throw validationError;
      }

      const validated = CustomAgentConfigSchema.parse(rawConfig);

      // Build CustomAgentDefinition
      const agent: CustomAgentDefinition = {
        name: validated.name,
        description: validated.description,
        allowedTools: validated.allowedTools,
        defaultModel: validated.defaultModel,
        maxTurns: validated.maxTurns,
        systemPrompt: validated.systemPrompt,
        source: {
          path: filePath,
          level,
          namespace,
        },
      };

      return agent;
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : String(error);
      logger.warn('CustomAgent', `Failed to parse custom agent`, {
        file: filePath,
        error: errorMsg,
        hint: 'Check agent configuration format and required fields',
      });
      return null;
    }
  }

  isValidName(name: string): boolean {
    // Agent names come from filenames, allow dash and underscore
    return /^[a-zA-Z0-9_-]+$/.test(name);
  }
}
