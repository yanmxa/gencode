/**
 * Memory System - Claude Code compatible memory management
 *
 * Provides hierarchical memory loading with AGENT.md (primary) and CLAUDE.md (fallback)
 */

export * from './types.js';
export { MemoryManager } from './memory-manager.js';
export { ImportResolver } from './import-resolver.js';
export { parseRuleFrontmatter, matchesPatterns, activateRules, getActiveRules } from './rules-parser.js';
export { gatherContextFiles, buildInitPrompt, getContextSummary } from './init-prompt.js';
