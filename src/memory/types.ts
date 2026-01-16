/**
 * Memory System Types
 *
 * Hierarchical memory loading compatible with Claude Code:
 * - User: ~/.gencode/AGENT.md → fallback ~/.claude/CLAUDE.md
 * - User Rules: ~/.gencode/rules/*.md → fallback ~/.claude/rules/*.md
 * - Project: ./AGENT.md → fallback ./CLAUDE.md
 * - Project Rules: ./.gencode/rules/*.md → fallback ./.claude/rules/*.md
 * - Local: ./.gencode/AGENT.local.md → fallback ./.claude/CLAUDE.local.md
 */

export type MemoryLevel = 'user' | 'user-rules' | 'project' | 'project-rules' | 'local';

export interface MemoryFile {
  path: string;
  content: string;
  level: MemoryLevel;
  loadedAt: Date;
  resolvedImports: string[];
}

export interface MemoryRule {
  path: string;
  content: string;
  patterns: string[]; // Glob patterns from 'paths:' frontmatter
  isActive: boolean; // Whether current file context matches
  level: 'user-rules' | 'project-rules';
}

export interface MemoryConfig {
  primaryFilename: string;
  fallbackFilename: string;
  localFilename: string;
  localFallbackFilename: string;
  primaryUserDir: string;
  fallbackUserDir: string;
  primaryLocalDir: string;
  fallbackLocalDir: string;
  rulesDir: string;
  maxFileSize: number;
  maxTotalSize: number;
  maxImportDepth: number;
}

export const DEFAULT_MEMORY_CONFIG: MemoryConfig = {
  primaryFilename: 'AGENT.md',
  fallbackFilename: 'CLAUDE.md',
  localFilename: 'AGENT.local.md',
  localFallbackFilename: 'CLAUDE.local.md',
  primaryUserDir: '.gencode',
  fallbackUserDir: '.claude',
  primaryLocalDir: '.gencode',
  fallbackLocalDir: '.claude',
  rulesDir: 'rules',
  maxFileSize: 100 * 1024, // 100KB
  maxTotalSize: 500 * 1024, // 500KB
  maxImportDepth: 5,
};

export interface LoadedMemory {
  files: MemoryFile[];
  rules: MemoryRule[];
  totalSize: number;
  context: string;
  errors: string[];
}

export interface MemoryLoadOptions {
  cwd: string;
  currentFile?: string; // For activating path-scoped rules
}
