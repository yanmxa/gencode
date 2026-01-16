/**
 * Memory System Types
 *
 * Hierarchical memory loading compatible with Claude Code:
 * At each level, both .gencode and .claude are loaded and merged (gencode has higher priority).
 *
 * Levels:
 * - Enterprise: System-wide managed memory (enforced)
 * - User: ~/.gencode/ + ~/.claude/ (both loaded, gencode content appears later)
 * - User Rules: ~/.gencode/rules/ + ~/.claude/rules/
 * - Extra: GENCODE_CONFIG_DIRS directories
 * - Project: ./AGENT.md, .gencode/, ./CLAUDE.md, .claude/ (recursive upward search)
 * - Project Rules: .gencode/rules/ + .claude/rules/
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

export type MemoryNamespace = 'gencode' | 'claude' | 'extra';

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
  gencodeFilename: string;
  gencodeLocalFilename: string;
  gencodeDir: string;

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
  gencodeFilename: 'AGENT.md',
  gencodeLocalFilename: 'AGENT.local.md',
  gencodeDir: '.gencode',

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
  primaryFilename: 'AGENT.md',
  fallbackFilename: 'CLAUDE.md',
  localFilename: 'AGENT.local.md',
  localFallbackFilename: 'CLAUDE.local.md',
  primaryUserDir: '.gencode',
  fallbackUserDir: '.claude',
  primaryLocalDir: '.gencode',
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
}
