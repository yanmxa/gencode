/**
 * Memory Manager - Core memory system implementation
 *
 * Implements Claude Code compatible memory loading:
 * 1. User: ~/.gencode/AGENT.md → fallback ~/.claude/CLAUDE.md
 * 2. User Rules: ~/.gencode/rules/*.md → fallback ~/.claude/rules/*.md
 * 3. Project: ./AGENT.md or ./.gencode/AGENT.md → fallback ./CLAUDE.md or ./.claude/CLAUDE.md
 * 4. Project Rules: ./.gencode/rules/*.md → fallback ./.claude/rules/*.md
 * 5. Local: ./.gencode/AGENT.local.md → fallback ./.claude/CLAUDE.local.md
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { glob } from 'glob';
import { ImportResolver } from './import-resolver.js';
import { parseRuleFrontmatter, activateRules } from './rules-parser.js';
import type { MemoryConfig, MemoryFile, MemoryRule, LoadedMemory, MemoryLoadOptions } from './types.js';
import { DEFAULT_MEMORY_CONFIG } from './types.js';

export class MemoryManager {
  private config: MemoryConfig;
  private importResolver: ImportResolver;
  private loadedMemory: LoadedMemory | null = null;

  constructor(config?: Partial<MemoryConfig>) {
    this.config = { ...DEFAULT_MEMORY_CONFIG, ...config };
    this.importResolver = new ImportResolver(this.config);
  }

  /**
   * Load all memory files for the given working directory
   */
  async load(options: MemoryLoadOptions): Promise<LoadedMemory> {
    const { cwd, currentFile } = options;
    const files: MemoryFile[] = [];
    const rules: MemoryRule[] = [];
    const errors: string[] = [];
    let totalSize = 0;

    this.importResolver.reset();
    const projectRoot = await this.findProjectRoot(cwd);
    this.importResolver.setProjectRoot(projectRoot);

    // 1. Load user-level memory
    const userFile = await this.loadUserMemory();
    if (userFile) {
      files.push(userFile);
      totalSize += userFile.content.length;
    }

    // 2. Load user-level rules
    const userRules = await this.loadUserRules();
    rules.push(...userRules);

    // 3. Load project-level memory
    const projectFile = await this.loadProjectMemory(cwd, projectRoot);
    if (projectFile) {
      if (totalSize + projectFile.content.length <= this.config.maxTotalSize) {
        files.push(projectFile);
        totalSize += projectFile.content.length;
      } else {
        errors.push(`Skipped ${projectFile.path}: would exceed max total size`);
      }
    }

    // 4. Load project-level rules
    const projectRules = await this.loadProjectRules(projectRoot);
    rules.push(...projectRules);

    // 5. Load local memory
    const localFile = await this.loadLocalMemory(projectRoot);
    if (localFile) {
      if (totalSize + localFile.content.length <= this.config.maxTotalSize) {
        files.push(localFile);
        totalSize += localFile.content.length;
      } else {
        errors.push(`Skipped ${localFile.path}: would exceed max total size`);
      }
    }

    // Activate rules based on current file
    const activatedRules = activateRules(rules, currentFile);

    // Build combined context
    const context = this.buildContext(files, activatedRules);

    this.loadedMemory = {
      files,
      rules: activatedRules,
      totalSize,
      context,
      errors,
    };

    return this.loadedMemory;
  }

  /**
   * Get the current loaded memory
   */
  getLoaded(): LoadedMemory | null {
    return this.loadedMemory;
  }

  /**
   * Check if any memory is loaded
   */
  hasMemory(): boolean {
    return this.loadedMemory !== null && (this.loadedMemory.files.length > 0 || this.loadedMemory.rules.length > 0);
  }

  /**
   * Build combined context string for system prompt injection
   */
  private buildContext(files: MemoryFile[], rules: MemoryRule[]): string {
    const parts: string[] = [];

    // Add regular memory files
    for (const file of files) {
      const label = this.getLevelLabel(file.level);
      parts.push(`Contents of ${file.path} (${label}):\n\n${file.content}`);
    }

    // Add active rules
    const activeRules = rules.filter((r) => r.isActive);
    for (const rule of activeRules) {
      const patterns = rule.patterns.length > 0 ? ` (applies to: ${rule.patterns.join(', ')})` : '';
      parts.push(`Rule from ${rule.path}${patterns}:\n\n${rule.content}`);
    }

    if (parts.length === 0) {
      return '';
    }

    return parts.join('\n\n---\n\n');
  }

  private getLevelLabel(level: string): string {
    const labels: Record<string, string> = {
      user: "user's private global instructions for all projects",
      'user-rules': 'user rules',
      project: 'project instructions, checked into the codebase',
      'project-rules': 'project rules',
      local: 'local personal notes',
    };
    return labels[level] || level;
  }

  /**
   * Load user-level memory file
   */
  private async loadUserMemory(): Promise<MemoryFile | null> {
    const home = os.homedir();

    // Try primary location first, then fallback
    const candidates = [
      path.join(home, this.config.primaryUserDir, this.config.primaryFilename),
      path.join(home, this.config.fallbackUserDir, this.config.fallbackFilename),
    ];

    for (const filePath of candidates) {
      const file = await this.loadFile(filePath, 'user');
      if (file) return file;
    }

    return null;
  }

  /**
   * Load user-level rules
   */
  private async loadUserRules(): Promise<MemoryRule[]> {
    const home = os.homedir();
    const rules: MemoryRule[] = [];

    // Try primary location first, then fallback
    const rulesDirs = [
      path.join(home, this.config.primaryUserDir, this.config.rulesDir),
      path.join(home, this.config.fallbackUserDir, this.config.rulesDir),
    ];

    for (const rulesDir of rulesDirs) {
      const dirRules = await this.loadRulesFromDir(rulesDir, 'user-rules');
      if (dirRules.length > 0) {
        rules.push(...dirRules);
        break; // Only use first found location
      }
    }

    return rules;
  }

  /**
   * Load project-level memory file
   */
  private async loadProjectMemory(cwd: string, projectRoot: string): Promise<MemoryFile | null> {
    // Try multiple locations in order of priority
    const candidates = [
      path.join(projectRoot, this.config.primaryFilename),
      path.join(projectRoot, this.config.primaryLocalDir, this.config.primaryFilename),
      path.join(projectRoot, this.config.fallbackFilename),
      path.join(projectRoot, this.config.fallbackLocalDir, this.config.fallbackFilename),
    ];

    for (const filePath of candidates) {
      const file = await this.loadFile(filePath, 'project');
      if (file) return file;
    }

    return null;
  }

  /**
   * Load project-level rules
   */
  private async loadProjectRules(projectRoot: string): Promise<MemoryRule[]> {
    const rules: MemoryRule[] = [];

    // Try primary location first, then fallback
    const rulesDirs = [
      path.join(projectRoot, this.config.primaryLocalDir, this.config.rulesDir),
      path.join(projectRoot, this.config.fallbackLocalDir, this.config.rulesDir),
    ];

    for (const rulesDir of rulesDirs) {
      const dirRules = await this.loadRulesFromDir(rulesDir, 'project-rules');
      if (dirRules.length > 0) {
        rules.push(...dirRules);
        break; // Only use first found location
      }
    }

    return rules;
  }

  /**
   * Load local memory file
   */
  private async loadLocalMemory(projectRoot: string): Promise<MemoryFile | null> {
    // Try primary location first, then fallback
    const candidates = [
      path.join(projectRoot, this.config.primaryLocalDir, this.config.localFilename),
      path.join(projectRoot, this.config.fallbackLocalDir, this.config.localFallbackFilename),
    ];

    for (const filePath of candidates) {
      const file = await this.loadFile(filePath, 'local');
      if (file) return file;
    }

    return null;
  }

  /**
   * Load rules from a directory
   */
  private async loadRulesFromDir(
    rulesDir: string,
    level: 'user-rules' | 'project-rules'
  ): Promise<MemoryRule[]> {
    const rules: MemoryRule[] = [];

    try {
      const files = await glob('**/*.md', { cwd: rulesDir, absolute: true });

      for (const filePath of files) {
        try {
          const stat = await fs.stat(filePath);
          if (stat.size > this.config.maxFileSize) {
            continue; // Skip files that are too large
          }

          const content = await fs.readFile(filePath, 'utf-8');
          const parsed = parseRuleFrontmatter(content);

          rules.push({
            path: filePath,
            content: parsed.content,
            patterns: parsed.paths,
            isActive: false, // Will be set by activateRules
            level,
          });
        } catch {
          // Skip invalid rule files
        }
      }
    } catch {
      // Rules directory doesn't exist
    }

    return rules;
  }

  /**
   * Load a single file with import resolution
   */
  private async loadFile(filePath: string, level: 'user' | 'project' | 'local'): Promise<MemoryFile | null> {
    try {
      const stat = await fs.stat(filePath);

      if (stat.size > this.config.maxFileSize) {
        return null;
      }

      let content = await fs.readFile(filePath, 'utf-8');
      const result = await this.importResolver.resolve(content, path.dirname(filePath));
      content = result.content;

      return {
        path: filePath,
        content,
        level,
        loadedAt: new Date(),
        resolvedImports: result.importedPaths,
      };
    } catch {
      return null;
    }
  }

  /**
   * Find project root (git root or cwd)
   */
  private async findProjectRoot(cwd: string): Promise<string> {
    let current = cwd;

    while (current !== '/') {
      try {
        await fs.access(path.join(current, '.git'));
        return current;
      } catch {
        const parent = path.dirname(current);
        if (parent === current) break;
        current = parent;
      }
    }

    return cwd;
  }

  /**
   * Quick add content to memory file
   */
  async quickAdd(content: string, level: 'user' | 'project', cwd: string): Promise<string> {
    let filePath: string;
    const home = os.homedir();

    if (level === 'user') {
      const dir = path.join(home, this.config.primaryUserDir);
      await fs.mkdir(dir, { recursive: true });
      filePath = path.join(dir, this.config.primaryFilename);
    } else {
      const projectRoot = await this.findProjectRoot(cwd);
      filePath = path.join(projectRoot, this.config.primaryFilename);
    }

    // Read existing content
    let existing = '';
    try {
      existing = await fs.readFile(filePath, 'utf-8');
    } catch {
      // File doesn't exist, create with header
      existing = `# ${this.config.primaryFilename.replace('.md', '')}\n\nThis file provides guidance when working with code in this repository.\n\n`;
    }

    // Append new content
    const newContent = `${existing.trimEnd()}\n\n${content}\n`;
    await fs.writeFile(filePath, newContent, 'utf-8');

    return filePath;
  }

  /**
   * Get list of loaded files for /memory command
   */
  getLoadedFileList(): Array<{ path: string; level: string; size: number; type: 'file' | 'rule' }> {
    if (!this.loadedMemory) return [];

    const list: Array<{ path: string; level: string; size: number; type: 'file' | 'rule' }> = [];

    for (const f of this.loadedMemory.files) {
      list.push({
        path: f.path,
        level: f.level,
        size: f.content.length,
        type: 'file',
      });
    }

    for (const r of this.loadedMemory.rules) {
      list.push({
        path: r.path,
        level: r.level,
        size: r.content.length,
        type: 'rule',
      });
    }

    return list;
  }

  /**
   * Get the path where /init would create a file
   */
  getInitFilePath(cwd: string): string {
    return path.join(cwd, this.config.primaryFilename);
  }

  /**
   * Check if project memory already exists
   */
  async hasProjectMemory(cwd: string): Promise<boolean> {
    const projectRoot = await this.findProjectRoot(cwd);
    const candidates = [
      path.join(projectRoot, this.config.primaryFilename),
      path.join(projectRoot, this.config.primaryLocalDir, this.config.primaryFilename),
      path.join(projectRoot, this.config.fallbackFilename),
      path.join(projectRoot, this.config.fallbackLocalDir, this.config.fallbackFilename),
    ];

    for (const filePath of candidates) {
      try {
        await fs.access(filePath);
        return true;
      } catch {
        continue;
      }
    }

    return false;
  }

  /**
   * Get the path of existing project memory file
   */
  async getExistingProjectMemoryPath(cwd: string): Promise<string | null> {
    const projectRoot = await this.findProjectRoot(cwd);
    const candidates = [
      path.join(projectRoot, this.config.primaryFilename),
      path.join(projectRoot, this.config.primaryLocalDir, this.config.primaryFilename),
      path.join(projectRoot, this.config.fallbackFilename),
      path.join(projectRoot, this.config.fallbackLocalDir, this.config.fallbackFilename),
    ];

    for (const filePath of candidates) {
      try {
        await fs.access(filePath);
        return filePath;
      } catch {
        continue;
      }
    }

    return null;
  }
}
