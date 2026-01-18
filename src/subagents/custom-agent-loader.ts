/**
 * CustomAgentLoader - Load user-defined custom agent configurations
 *
 * Uses the unified resource discovery system to load custom agents from:
 * - User level: ~/.claude/agents/ and ~/.gen/agents/
 * - Project level: .claude/agents/ and .gen/agents/ (NEW!)
 *
 * Priority: project gen > project claude > user gen > user claude
 *
 * Supports both JSON and Markdown formats:
 * - JSON: ~/.gen/agents/code-reviewer.json
 * - Markdown: ~/.gen/agents/code-reviewer.md
 *
 * Example JSON:
 * {
 *   "name": "code-reviewer",
 *   "description": "Expert code review specialist",
 *   "allowedTools": ["Read", "Grep", "Glob", "WebFetch"],
 *   "defaultModel": "claude-sonnet-4",
 *   "maxTurns": 15,
 *   "permissionMode": "permissive",
 *   "systemPrompt": "You are a senior code reviewer..."
 * }
 */

import * as fs from 'node:fs/promises';
import * as path from 'node:path';
import { homedir } from 'node:os';
import { discoverResources } from '../discovery/index.js';
import { CustomAgentParser } from './parser.js';
import type { SubagentConfig, CustomAgentDefinition, customAgentToConfig } from './types.js';
import { customAgentToConfig as convertToConfig } from './types.js';

/**
 * CustomAgentLoader - Manages custom agent configurations
 *
 * Now uses the unified discovery system for consistent loading across user and project levels.
 */
export class CustomAgentLoader {
  private customAgents: Map<string, CustomAgentDefinition> = new Map();
  private initialized: boolean = false;
  private gencodeAgentsDir: string;

  constructor(gencodeDir?: string) {
    const genDir = gencodeDir ?? path.join(homedir(), '.gen');
    this.gencodeAgentsDir = path.join(genDir, 'agents');
  }

  /**
   * Initialize loader (create directory, load configs)
   */
  private async initialize(projectRoot: string = process.cwd()): Promise<void> {
    if (this.initialized) return;

    // Ensure GenCode agents directory exists (we manage this one)
    await fs.mkdir(this.gencodeAgentsDir, { recursive: true });

    // Load all custom agents using unified discovery system
    this.customAgents = await discoverResources(projectRoot, {
      resourceType: 'Custom Agent',
      subdirectory: 'agents',
      filePattern: { type: 'multiple', extensions: ['.json', '.md'] },
      parser: new CustomAgentParser(),
      levels: ['user', 'project'], // Now supports both user and project levels!
    });

    this.initialized = true;
  }

  /**
   * Get a custom agent configuration by name
   */
  async getAgentConfig(name: string, projectRoot: string = process.cwd()): Promise<SubagentConfig | null> {
    await this.initialize(projectRoot);
    const agent = this.customAgents.get(name);
    return agent ? convertToConfig(agent) : null;
  }

  /**
   * Get agent source (gen or claude, user or project)
   * @param name - Agent name
   * @returns Source information or null if not found
   */
  async getAgentSource(
    name: string,
    projectRoot: string = process.cwd()
  ): Promise<{ level: string; namespace: string } | null> {
    await this.initialize(projectRoot);
    const agent = this.customAgents.get(name);
    if (!agent) return null;

    return {
      level: agent.source.level,
      namespace: agent.source.namespace,
    };
  }

  /**
   * Check if a custom agent exists
   */
  async hasAgent(name: string, projectRoot: string = process.cwd()): Promise<boolean> {
    await this.initialize(projectRoot);
    return this.customAgents.has(name);
  }

  /**
   * List all available custom agents
   */
  async listAgents(projectRoot: string = process.cwd()): Promise<CustomAgentDefinition[]> {
    await this.initialize(projectRoot);
    return Array.from(this.customAgents.values());
  }

  /**
   * Reload all custom agents (for hot-reload support)
   */
  async reload(projectRoot: string = process.cwd()): Promise<void> {
    this.initialized = false;
    await this.initialize(projectRoot);
  }
}
