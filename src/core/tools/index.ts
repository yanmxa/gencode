/**
 * Tools System - Built-in tools and registry
 */

import { logger } from '../../base/utils/logger.js';
import { isDebugEnabled } from '../../base/utils/debug.js';

export * from './types.js';
export { ToolRegistry } from './registry.js';

// Tool Factories
export { createSkillTool, resetSkillDiscovery } from './factories/skill-tool-factory.js';
export { bridgeMCPTool } from './factories/mcp-tool-factory.js';

// Built-in tools
export { readTool } from './builtin/read.js';
export { writeTool } from './builtin/write.js';
export { editTool } from './builtin/edit.js';
export { bashTool } from './builtin/bash.js';
export { globTool } from './builtin/glob.js';
export { grepTool } from './builtin/grep.js';
export { webfetchTool } from './builtin/webfetch.js';
export { websearchTool } from './builtin/websearch.js';
export { todowriteTool, getTodos, clearTodos } from './builtin/todowrite.js';
export {
  askUserQuestionTool,
  formatAnswersForAgent,
  formatAnswersForDisplay,
} from './builtin/ask-user.js';
export type {
  Question as AskUserQuestion,
  QuestionOption as AskUserQuestionOption,
  QuestionAnswer as AskUserQuestionAnswer,
  AskUserQuestionInput,
  AskUserQuestionResult,
} from './builtin/ask-user.js';
export { taskoutputTool } from './builtin/taskoutput.js';

// Plan mode tools
export { enterPlanModeTool, exitPlanModeTool } from '../../cli/planning/index.js';

// Subagent tools
export { taskTool } from './factories/task-tool-factory.js';

import { ToolRegistry } from './registry.js';
import { readTool } from './builtin/read.js';
import { writeTool } from './builtin/write.js';
import { editTool } from './builtin/edit.js';
import { bashTool } from './builtin/bash.js';
import { globTool } from './builtin/glob.js';
import { grepTool } from './builtin/grep.js';
import { webfetchTool } from './builtin/webfetch.js';
import { websearchTool } from './builtin/websearch.js';
import { todowriteTool } from './builtin/todowrite.js';
import { askUserQuestionTool } from './builtin/ask-user.js';
import { taskoutputTool } from './builtin/taskoutput.js';
import { enterPlanModeTool, exitPlanModeTool } from '../../cli/planning/index.js';
import { taskTool } from './factories/task-tool-factory.js';
import { createSkillTool } from './factories/skill-tool-factory.js';

// Mutex for preventing concurrent registry initialization
let registryInitPromise: Promise<ToolRegistry> | null = null;

/**
 * Create a registry with all built-in tools
 * Now async to support dynamic skill discovery and MCP tools
 *
 * Thread-safe: Uses a mutex to prevent concurrent initialization
 *
 * @param cwd - Current working directory for skill discovery
 */
export async function createDefaultRegistry(cwd: string): Promise<ToolRegistry> {
  // If initialization is already in progress, wait for it
  if (registryInitPromise) {
    return registryInitPromise;
  }

  // Start new initialization
  registryInitPromise = (async () => {
  const registry = new ToolRegistry();
  registry.registerAll([
    readTool,
    writeTool,
    editTool,
    bashTool,
    globTool,
    grepTool,
    webfetchTool,
    websearchTool,
    todowriteTool,
    askUserQuestionTool,
    taskoutputTool,
    enterPlanModeTool,
    exitPlanModeTool,
    taskTool,
  ]);

  // Dynamically create and register Skill tool
  const skillTool = await createSkillTool(cwd);
  registry.register(skillTool);

  // Load MCP tools if available
  try {
    const { getMCPManager, loadMCPConfig } = await import('../../extensions/mcp/index.js');
    const mcpConfig = await loadMCPConfig(cwd);
    const mcpManager = getMCPManager();

    // Initialize MCP manager if not already initialized
    if (!mcpManager.isInitialized()) {
      await mcpManager.initialize(mcpConfig);
    }

    // Get all MCP tools
    const mcpTools = await mcpManager.getAllTools();
    if (mcpTools.length > 0) {
      registry.registerAll(mcpTools);
      if (isDebugEnabled('mcp')) {
        logger.debug('MCP', `Loaded ${mcpTools.length} tools from MCP servers`);
      }
    }
  } catch (error) {
    const errorMsg = error instanceof Error ? error.message : String(error);
    logger.warn('MCP', 'MCP integration failed to load', {
      error: errorMsg,
      hint: 'Check .mcp.json configuration. MCP is optional - system will continue without it.',
    });
  }

  return registry;
  })();

  try {
    return await registryInitPromise;
  } finally {
    // Clear the promise after initialization completes
    registryInitPromise = null;
  }
}

/**
 * All built-in tools
 */
export const builtinTools = [
  readTool,
  writeTool,
  editTool,
  bashTool,
  globTool,
  grepTool,
  webfetchTool,
  websearchTool,
  todowriteTool,
  askUserQuestionTool,
  taskoutputTool,
  enterPlanModeTool,
  exitPlanModeTool,
  taskTool,
];
