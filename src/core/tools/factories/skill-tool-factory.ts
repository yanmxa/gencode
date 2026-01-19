/**
 * Skill Tool - Dynamically generated tool for activating skills
 *
 * The Skill tool provides access to domain-specific knowledge through SKILL.md files.
 * Tool description is dynamically generated to list all available skills.
 */

import { z } from 'zod';
import type { Tool, ToolContext, ToolResult } from '../types.js';
import { SkillDiscovery } from '../../../extensions/skills/manager.js';
import type { SkillInput, SkillDefinition } from '../../../extensions/skills/types.js';
import { formatBoxedMessage } from '../../../base/utils/format-utils.js';
import { isVerboseDebugEnabled } from '../../../base/utils/debug.js';
import { logger } from '../../../base/utils/logger.js';

// Module-level discovery instance (shared across tool invocations in production)
// This is a singleton for performance, but can be reset for testing
let sharedDiscovery = new SkillDiscovery();

/**
 * Reset the shared discovery instance (for testing purposes)
 * This allows tests to start with a clean state
 */
export function resetSkillDiscovery(): void {
  sharedDiscovery = new SkillDiscovery();
}

/**
 * Create the Skill tool with dynamically generated description
 *
 * Discovers all skills from filesystem and builds tool with:
 * - Description listing all available skills
 * - Execute function that injects skill content
 *
 * @param projectRoot - Project root directory for skill discovery
 * @param options - Optional configuration (for testing)
 * @returns Tool instance ready for registration
 */
export async function createSkillTool(
  projectRoot: string,
  options?: { projectOnly?: boolean }
): Promise<Tool<SkillInput>> {
  // Use a new discovery instance with options if provided, otherwise use shared instance
  const discovery = options ? new SkillDiscovery(options) : sharedDiscovery;

  // Discover all skills from hierarchical directories
  await discovery.discover(projectRoot);
  const skills = discovery.getAll();

  // Build dynamic description with skills list
  const description = buildSkillDescription(skills);

  return {
    name: 'Skill',
    description,
    parameters: z.object({
      skill: z.string().describe('Skill name to activate'),
      args: z.string().optional().describe('Optional arguments for the skill'),
    }),

    async execute(input: SkillInput, context: ToolContext): Promise<ToolResult> {
      // Verbose debug: Log skill activation attempt
      if (isVerboseDebugEnabled('skills')) {
        logger.debug('Skill', `Activating skill: ${input.skill}`, {
          args: input.args || 'none',
          sessionId: context.sessionId,
        });
      }

      const skill = discovery.get(input.skill);

      if (!skill) {
        if (isVerboseDebugEnabled('skills')) {
          logger.debug('Skill', `Skill not found: ${input.skill}`, {
            availableSkills: discovery.getAll().map((s) => s.name).join(', ') || 'none',
          });
        }
        return {
          success: false,
          output: '',
          error: `Skill not found: ${input.skill}\n\nAvailable skills: ${discovery.getAll().map((s) => s.name).join(', ') || 'none'}`,
        };
      }

      // Verbose debug: Log skill found
      if (isVerboseDebugEnabled('skills')) {
        logger.debug('Skill', `Found skill definition`, {
          name: skill.name,
          hasContent: !!skill.content,
          contentLength: skill.content.length,
          source: `${skill.source.level}/${skill.source.namespace}`,
        });
      }

      // Format skill content as tool_result
      const output = formatSkillActivation(skill, input.args);

      // Verbose debug: Log activation complete
      if (isVerboseDebugEnabled('skills')) {
        logger.debug('Skill', `Skill activation complete`, {
          skillName: skill.name,
          outputLength: output.length,
        });
      }

      return {
        success: true,
        output,
        metadata: {
          title: `Skill: ${skill.name}`,
          subtitle: skill.description,
        },
      };
    },
  };
}

/**
 * Build tool description with list of available skills
 */
function buildSkillDescription(skills: SkillDefinition[]): string {
  const lines = [];

  lines.push('Execute a skill to gain specialized domain knowledge.');
  lines.push('');
  lines.push('Skills inject expertise into your context for specific tasks.');
  lines.push('Invoke a skill when the task matches its description.');
  lines.push('');
  lines.push('**Available Skills:**');
  lines.push('');

  if (skills.length === 0) {
    lines.push('(No skills available. Create skills in ~/.gen/skills/)');
  } else {
    for (const skill of skills) {
      lines.push(`• **${skill.name}** - ${skill.description}`);
    }
  }

  lines.push('');
  lines.push('**Usage:** Skill({ skill: "skill-name", args: "optional-args" })');
  lines.push('');
  lines.push('**Skills are loaded from:**');
  lines.push('- ~/.gen/skills/ (user, highest priority)');
  lines.push('- ~/.claude/skills/ (user, fallback)');
  lines.push('- .gen/skills/ (project)');
  lines.push('- .claude/skills/ (project)');

  return lines.join('\n');
}

/**
 * Format skill activation output
 *
 * Returns formatted markdown with skill content for injection into context
 */
function formatSkillActivation(skill: SkillDefinition, args?: string): string {
  const lines = [];

  // Build fields for boxed message
  const fields: Record<string, string> = {
    'Name': skill.name,
    'Description': skill.description,
    'Source': `${skill.source.level}/${skill.source.namespace}`,
  };

  if (args) {
    fields['Arguments'] = args;
  }

  // Use shared formatter for boxed message
  lines.push(formatBoxedMessage('Skill Activated', fields));
  lines.push('');
  lines.push(skill.content);
  lines.push('');
  lines.push('---');
  lines.push('');
  lines.push('Follow the instructions above to complete the task.');

  return lines.join('\n');
}
