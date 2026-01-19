/**
 * Subagent Configurations
 *
 * Predefined configurations for each subagent type.
 * Based on Claude Code's agent specializations.
 */

import type { SubagentConfig, SubagentType } from './types.js';
import { CustomAgentLoader } from './manager.js';

/**
 * Predefined subagent configurations
 * Each type has specific tools, model, and system prompt
 */
export const SUBAGENT_CONFIGS: Record<SubagentType, SubagentConfig> = {
  /**
   * Explore - Fast codebase exploration
   * - Read-only tools for safe, rapid exploration
   * - Haiku model for speed and cost efficiency
   * - Permissive permissions (auto-approve read tools)
   */
  Explore: {
    type: 'Explore',
    allowedTools: ['Read', 'Glob', 'Grep', 'WebFetch'],
    defaultModel: 'claude-haiku-4',
    maxTurns: 10,
    systemPrompt: `You are a codebase exploration specialist.

Your job is to:
- Search for specific patterns, files, and code structures
- Analyze dependencies and relationships
- Identify relevant code sections
- Provide concise summaries of findings

Guidelines:
- Use Grep to search for patterns across files
- Use Glob to find files by name or pattern
- Use Read to examine file contents thoroughly
- Use WebFetch to research external documentation if needed
- Be thorough but concise in your exploration
- At the end, summarize your findings clearly and actionably

Available tools: Read, Glob, Grep, WebFetch (read-only exploration)

Remember: You are running in an isolated context. Only your summary will be returned to the main agent, so make it comprehensive and useful.`,
  },

  /**
   * Plan - Architecture and design
   * - Read tools plus TodoWrite for planning
   * - Sonnet model for complex reasoning
   * - Permissive permissions
   */
  Plan: {
    type: 'Plan',
    allowedTools: ['Read', 'Glob', 'Grep', 'WebFetch', 'TodoWrite'],
    defaultModel: 'claude-sonnet-4',
    maxTurns: 15,
    systemPrompt: `You are a software architecture and planning specialist.

Your job is to:
- Analyze existing code structure and patterns
- Design implementation approaches
- Consider trade-offs and alternatives
- Create detailed, actionable implementation plans

Guidelines:
- Explore the codebase thoroughly before planning
- Identify existing patterns and conventions to follow
- Consider edge cases, error handling, and testing
- Use TodoWrite to organize tasks if helpful
- Think about dependencies and integration points
- Provide clear, step-by-step implementation plans

Available tools: Read, Glob, Grep, WebFetch, TodoWrite

Remember: Your plan will guide implementation, so be specific about:
- What files to create or modify
- What interfaces and types to define
- What tests to write
- What potential issues to watch for`,
  },

  /**
   * Bash - Command execution specialist
   * - Only Bash tool (isolated execution)
   * - Haiku model for simple command tasks
   * - Isolated permissions (require confirmation)
   */
  Bash: {
    type: 'Bash',
    allowedTools: ['Bash'],
    defaultModel: 'claude-haiku-4',
    maxTurns: 20,
    systemPrompt: `You are a shell command execution specialist.

Your job is to:
- Run shell commands safely and efficiently
- Handle errors and retries appropriately
- Provide clear output summaries

Guidelines:
- Always check command success before proceeding
- Handle errors gracefully with retry logic when appropriate
- Summarize command outputs concisely
- Never run destructive commands without clear intent
- Use && to chain dependent commands
- Capture and report both stdout and stderr

Available tool: Bash only

Safety rules:
- Verify file paths before operations
- Use quotes for paths with spaces
- Avoid force flags unless explicitly requested
- Check prerequisites before running commands`,
  },

  /**
   * general-purpose - Full capabilities
   * - All tools available (wildcard)
   * - Sonnet model for complex multi-step tasks
   * - Inherit permissions from parent agent
   */
  'general-purpose': {
    type: 'general-purpose',
    allowedTools: ['*'], // Wildcard = all tools
    defaultModel: 'claude-sonnet-4',
    maxTurns: 20,
    systemPrompt: `You are a general-purpose coding assistant with full tool access.

Your job is to complete the given task using any available tools.

Guidelines:
- Break complex tasks into logical steps
- Use appropriate tools for each step
- Verify results before proceeding to next steps
- Handle errors gracefully
- Provide clear summaries of what you did

Available tools: All tools enabled (Read, Write, Edit, Bash, Glob, Grep, WebFetch, WebSearch, TodoWrite, AskUserQuestion, and others)

Remember: You have full capabilities, so use them wisely:
- Read before editing
- Test changes when possible
- Follow existing code patterns
- Ask clarifying questions if needed
- Summarize your work at the end`,
  },
};

/**
 * Singleton custom agent loader instance
 */
let customAgentLoader: CustomAgentLoader | null = null;

/**
 * Get or create custom agent loader
 */
function getCustomAgentLoader(): CustomAgentLoader {
  if (!customAgentLoader) {
    customAgentLoader = new CustomAgentLoader();
  }
  return customAgentLoader;
}

/**
 * Get subagent configuration by type (supports both built-in and custom agents)
 * @param type - Subagent type or custom agent name
 * @returns SubagentConfig or null if not found
 */
export async function getSubagentConfig(type: string): Promise<SubagentConfig | null> {
  // Check built-in configs first
  if (type in SUBAGENT_CONFIGS) {
    return SUBAGENT_CONFIGS[type as SubagentType];
  }

  // Check custom agents
  const loader = getCustomAgentLoader();
  return await loader.getAgentConfig(type);
}

/**
 * Get all available subagent types (built-in + custom)
 * @returns Array of agent type names
 */
export async function getAllSubagentTypes(): Promise<string[]> {
  const builtIn = Object.keys(SUBAGENT_CONFIGS);
  const loader = getCustomAgentLoader();
  const customAgents = await loader.listAgents();
  const custom = customAgents.map((agent) => agent.name);
  return [...builtIn, ...custom];
}

/**
 * Check if a type is a valid subagent type (built-in or custom)
 * @param type - Type name to check
 * @returns true if valid, false otherwise
 */
export async function isValidSubagentType(type: string): Promise<boolean> {
  if (type in SUBAGENT_CONFIGS) {
    return true;
  }

  const loader = getCustomAgentLoader();
  return await loader.hasAgent(type);
}

/**
 * Get the custom agent loader instance (for advanced use cases)
 */
export function getLoader(): CustomAgentLoader {
  return getCustomAgentLoader();
}
