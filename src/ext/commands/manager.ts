/**
 * Command Manager
 *
 * Manages the lifecycle of custom commands including discovery,
 * retrieval, and parsing with variable expansion.
 */

import { discoverCommands } from './discovery.js';
import { expandTemplate, parseArguments } from './expander.js';
import type { CommandDefinition, ParsedCommand, ExpansionContext } from './types.js';
import { isVerboseDebugEnabled } from '../../base/utils/debug.js';
import { logger } from '../../base/utils/logger.js';

export class CommandManager {
  private commands: Map<string, CommandDefinition> = new Map();
  private projectRoot: string;
  private initialized: boolean = false;

  constructor(projectRoot: string) {
    this.projectRoot = projectRoot;
  }

  /**
   * Initialize the command manager by discovering all commands
   */
  async initialize(): Promise<void> {
    if (this.initialized) {
      return;
    }

    this.commands = await discoverCommands(this.projectRoot);
    this.initialized = true;
  }

  /**
   * Ensure initialization before any operation
   */
  private async ensureInitialized(): Promise<void> {
    if (!this.initialized) {
      await this.initialize();
    }
  }

  /**
   * List all available commands
   */
  async listCommands(): Promise<CommandDefinition[]> {
    await this.ensureInitialized();
    return Array.from(this.commands.values()).sort((a, b) =>
      a.name.localeCompare(b.name)
    );
  }

  /**
   * Get a command by name
   */
  async getCommand(name: string): Promise<CommandDefinition | undefined> {
    await this.ensureInitialized();
    return this.commands.get(name);
  }

  /**
   * Check if a command exists
   */
  async hasCommand(name: string): Promise<boolean> {
    await this.ensureInitialized();
    return this.commands.has(name);
  }

  /**
   * Parse a command with arguments and expand variables/file includes
   *
   * @param name - Command name
   * @param args - Argument string from user input
   * @returns Parsed command ready for execution, or null if command not found
   */
  async parseCommand(name: string, args: string): Promise<ParsedCommand | null> {
    await this.ensureInitialized();

    // Verbose debug: Log command parsing attempt
    if (isVerboseDebugEnabled('commands')) {
      logger.debug('Command', `Parsing command: /${name}`, {
        args: args || 'none',
        projectRoot: this.projectRoot,
      });
    }

    const definition = this.commands.get(name);
    if (!definition) {
      if (isVerboseDebugEnabled('commands')) {
        logger.debug('Command', `Command not found: /${name}`, {
          availableCommands: Array.from(this.commands.keys()).join(', ') || 'none',
        });
      }
      return null;
    }

    // Parse arguments into positional array
    const positionalArgs = parseArguments(args);

    // Verbose debug: Log template expansion
    if (isVerboseDebugEnabled('commands')) {
      logger.debug('Command', `Expanding template for /${name}`, {
        templateLength: definition.content.length,
        arguments: args,
        positionalArgsCount: positionalArgs.length,
        projectRoot: this.projectRoot,
      });
    }

    // Create expansion context
    const context: ExpansionContext = {
      arguments: args,
      positionalArgs,
      projectRoot: this.projectRoot,
    };

    // Expand template
    const expandedPrompt = await expandTemplate(definition.content, context);

    // Verbose debug: Log parsing complete
    if (isVerboseDebugEnabled('commands')) {
      logger.debug('Command', `Command parsed successfully: /${name}`, {
        expandedPromptLength: expandedPrompt.length,
        preAuthorizedTools: definition.allowedTools?.join(', ') || 'none',
        modelOverride: definition.model || 'none',
      });
    }

    return {
      definition,
      expandedPrompt,
      preAuthorizedTools: definition.allowedTools || [],
      modelOverride: definition.model,
    };
  }

  /**
   * Reload all commands (useful for development)
   */
  async reload(): Promise<void> {
    this.initialized = false;
    await this.initialize();
  }
}

/**
 * Global command manager instance (lazy initialized per project)
 */
let globalManager: CommandManager | null = null;
let currentProjectRoot: string | null = null;

/**
 * Get or create the global command manager
 */
export async function getCommandManager(projectRoot: string): Promise<CommandManager> {
  // Recreate manager if project root changed
  if (!globalManager || currentProjectRoot !== projectRoot) {
    globalManager = new CommandManager(projectRoot);
    currentProjectRoot = projectRoot;
    await globalManager.initialize();
  }

  return globalManager;
}
