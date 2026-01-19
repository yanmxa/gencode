/**
 * Memory System Types
 *
 * Hierarchical memory loading compatible with Claude Code:
 * At each level, both .gen and .claude are loaded and merged (gen has higher priority).
 *
 * Levels:
 * - Enterprise: System-wide managed memory (enforced)
 * - User: ~/.gen/ + ~/.claude/ (both loaded, gen content appears later)
 * - User Rules: ~/.gen/rules/ + ~/.claude/rules/
 * - Extra: GEN_CONFIG_DIRS directories
 * - Project: ./GEN.md, .gen/, ./CLAUDE.md, .claude/ (recursive upward search)
 * - Project Rules: .gen/rules/ + .claude/rules/
 * - Local: *.local.md files (gitignored)
 */

export type MemoryLevel =
  | 'enterprise'
  | 'user'
  | 'user-rules'
  | 'extra'
  | 'project'
  | 'project-rules'
  | 'local';

export type MemoryNamespace = 'gen' | 'claude' | 'extra';

/**
 * Memory merge strategy - how to combine CLAUDE.md and GEN.md at each level
 * - fallback: Load GEN.md if exists, else CLAUDE.md (reduces context, default)
 * - both: Load both CLAUDE.md and GEN.md (current behavior, max context)
 * - gen-only: Only load .gen/GEN.md files
 * - claude-only: Only load .claude/CLAUDE.md files
 */
export type MemoryMergeStrategy = 'fallback' | 'both' | 'gen-only' | 'claude-only';

export interface MemoryFile {
  path: string;
  content: string;
  level: MemoryLevel;
  namespace: MemoryNamespace;
  loadedAt: Date;
  resolvedImports: string[];
  enforced?: boolean; // For enterprise level
}

export interface MemoryRule {
  path: string;
  content: string;
  patterns: string[]; // Glob patterns from 'paths:' frontmatter
  isActive: boolean; // Whether current file context matches
  level: 'user-rules' | 'project-rules';
  namespace: MemoryNamespace;
}

export interface MemoryConfig {
  // GenCode file names (higher priority)
  genFilename: string;
  genLocalFilename: string;
  genDir: string;

  // Claude file names (lower priority, loaded first)
  claudeFilename: string;
  claudeLocalFilename: string;
  claudeDir: string;

  // Common settings
  rulesDir: string;
  maxFileSize: number;
  maxTotalSize: number;
  maxImportDepth: number;
}

export const DEFAULT_MEMORY_CONFIG: MemoryConfig = {
  // GenCode
  genFilename: 'GEN.md',
  genLocalFilename: 'GEN.local.md',
  genDir: '.gen',

  // Claude
  claudeFilename: 'CLAUDE.md',
  claudeLocalFilename: 'CLAUDE.local.md',
  claudeDir: '.claude',

  // Common
  rulesDir: 'rules',
  maxFileSize: 100 * 1024, // 100KB
  maxTotalSize: 500 * 1024, // 500KB
  maxImportDepth: 5,
};

// Legacy compatibility
export const LEGACY_MEMORY_CONFIG = {
  primaryFilename: 'GEN.md',
  fallbackFilename: 'CLAUDE.md',
  localFilename: 'GEN.local.md',
  localFallbackFilename: 'CLAUDE.local.md',
  primaryUserDir: '.gen',
  fallbackUserDir: '.claude',
  primaryLocalDir: '.gen',
  fallbackLocalDir: '.claude',
  rulesDir: 'rules',
  maxFileSize: 100 * 1024,
  maxTotalSize: 500 * 1024,
  maxImportDepth: 5,
};

export interface LoadedMemory {
  files: MemoryFile[];
  rules: MemoryRule[];
  totalSize: number;
  context: string;
  errors: string[];
  sources: MemorySource[]; // For debugging
  skippedFiles: string[]; // Files skipped due to merge strategy
}

export interface MemorySource {
  level: MemoryLevel;
  namespace: MemoryNamespace;
  path: string;
  type: 'file' | 'rule';
  size: number;
}

export interface MemoryLoadOptions {
  cwd: string;
  currentFile?: string; // For activating path-scoped rules
  strategy?: MemoryMergeStrategy; // How to merge CLAUDE.md and AGENT.md (default: 'fallback')
}
