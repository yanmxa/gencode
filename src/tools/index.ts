/**
 * Tools System - Built-in tools and registry
 */

export * from './types.js';
export { ToolRegistry } from './registry.js';

// Built-in tools
export { readTool } from './builtin/read.js';
export { writeTool } from './builtin/write.js';
export { editTool } from './builtin/edit.js';
export { bashTool } from './builtin/bash.js';
export { globTool } from './builtin/glob.js';
export { grepTool } from './builtin/grep.js';

import { ToolRegistry } from './registry.js';
import { readTool } from './builtin/read.js';
import { writeTool } from './builtin/write.js';
import { editTool } from './builtin/edit.js';
import { bashTool } from './builtin/bash.js';
import { globTool } from './builtin/glob.js';
import { grepTool } from './builtin/grep.js';

/**
 * Create a registry with all built-in tools
 */
export function createDefaultRegistry(): ToolRegistry {
  const registry = new ToolRegistry();
  registry.registerAll([readTool, writeTool, editTool, bashTool, globTool, grepTool]);
  return registry;
}

/**
 * All built-in tools
 */
export const builtinTools = [readTool, writeTool, editTool, bashTool, globTool, grepTool];
