/**
 * Memory Manager - Core memory system implementation
 *
 * Implements Claude Code compatible memory loading with merge semantics:
 * At each level, both .gencode and .claude directories are loaded.
 * Content from .gencode appears later in the context (higher priority for LLM).
 *
 * Loading order within each level:
 * 1. .claude files first (lower priority - LLM sees earlier)
 * 2. .gencode files second (higher priority - LLM sees later)
 *
 * Level loading order:
 * 1. Enterprise (system-wide managed, enforced)
 * 2. User (~/.gencode/ + ~/.claude/)
 * 3. User Rules
 * 4. Extra (GENCODE_CONFIG_DIRS)
 * 5. Project (recursive upward search)
 * 6. Project Rules
 * 7. Local (*.local.md files)
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { glob } from 'glob';
import { ImportResolver } from './import-resolver.js';
import { parseRuleFrontmatter, activateRules } from './rules-parser.js';
import type {
  MemoryConfig,
  MemoryFile,
  MemoryRule,
  LoadedMemory,
  MemoryLoadOptions,
  MemoryLevel,
  MemoryNamespace,
  MemorySource,
} from './types.js';
import { DEFAULT_MEMORY_CONFIG } from './types.js';
import { getManagedPaths, GENCODE_CONFIG_DIRS_ENV } from '../config/types.js';

export class MemoryManager {
  private config: MemoryConfig;
  private importResolver: ImportResolver;
  private loadedMemory: LoadedMemory | null = null;

  constructor(config?: Partial<MemoryConfig>) {
    this.config = { ...DEFAULT_MEMORY_CONFIG, ...config };
    // Create import resolver with the config
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
    const sources: MemorySource[] = [];
    let totalSize = 0;

    this.importResolver.reset();
    const projectRoot = await this.findProjectRoot(cwd);
    this.importResolver.setProjectRoot(projectRoot);

    // 1. Load enterprise memory (system-wide, enforced)
    const enterpriseFiles = await this.loadEnterpriseMemory();
    for (const file of enterpriseFiles) {
      files.push(file);
      totalSize += file.content.length;
      sources.push({
        level: 'enterprise',
        namespace: file.namespace,
        path: file.path,
        type: 'file',
        size: file.content.length,
      });
    }

    // 2. Load user-level memory (both claude and gencode)
    const userFiles = await this.loadUserMemory();
    for (const file of userFiles) {
      if (totalSize + file.content.length <= this.config.maxTotalSize) {
        files.push(file);
        totalSize += file.content.length;
        sources.push({
          level: 'user',
          namespace: file.namespace,
          path: file.path,
          type: 'file',
          size: file.content.length,
        });
      } else {
        errors.push(`Skipped ${file.path}: would exceed max total size`);
      }
    }

    // 3. Load user-level rules (both claude and gencode)
    const userRules = await this.loadUserRules();
    for (const rule of userRules) {
      rules.push(rule);
      sources.push({
        level: 'user-rules',
        namespace: rule.namespace,
        path: rule.path,
        type: 'rule',
        size: rule.content.length,
      });
    }

    // 4. Load extra config dirs memory
    const extraFiles = await this.loadExtraMemory();
    for (const file of extraFiles) {
      if (totalSize + file.content.length <= this.config.maxTotalSize) {
        files.push(file);
        totalSize += file.content.length;
        sources.push({
          level: 'extra',
          namespace: file.namespace,
          path: file.path,
          type: 'file',
          size: file.content.length,
        });
      } else {
        errors.push(`Skipped ${file.path}: would exceed max total size`);
      }
    }

    // 5. Load project-level memory (both claude and gencode, recursive upward)
    const projectFiles = await this.loadProjectMemory(cwd, projectRoot);
    for (const file of projectFiles) {
      if (totalSize + file.content.length <= this.config.maxTotalSize) {
        files.push(file);
        totalSize += file.content.length;
        sources.push({
          level: 'project',
          namespace: file.namespace,
          path: file.path,
          type: 'file',
          size: file.content.length,
        });
      } else {
        errors.push(`Skipped ${file.path}: would exceed max total size`);
      }
    }

    // 6. Load project-level rules (both claude and gencode)
    const projectRules = await this.loadProjectRules(projectRoot);
    for (const rule of projectRules) {
      rules.push(rule);
      sources.push({
        level: 'project-rules',
        namespace: rule.namespace,
        path: rule.path,
        type: 'rule',
        size: rule.content.length,
      });
    }

    // 7. Load local memory (both claude and gencode)
    const localFiles = await this.loadLocalMemory(projectRoot);
    for (const file of localFiles) {
      if (totalSize + file.content.length <= this.config.maxTotalSize) {
        files.push(file);
        totalSize += file.content.length;
        sources.push({
          level: 'local',
          namespace: file.namespace,
          path: file.path,
          type: 'file',
          size: file.content.length,
        });
      } else {
        errors.push(`Skipped ${file.path}: would exceed max total size`);
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
      sources,
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
    return (
      this.loadedMemory !== null &&
      (this.loadedMemory.files.length > 0 || this.loadedMemory.rules.length > 0)
    );
  }

  /**
   * Build combined context string for system prompt injection
   */
  private buildContext(files: MemoryFile[], rules: MemoryRule[]): string {
    const parts: string[] = [];

    // Add regular memory files
    for (const file of files) {
      const label = this.getLevelLabel(file.level, file.namespace);
      const enforced = file.enforced ? ' [ENFORCED]' : '';
      parts.push(`Contents of ${file.path} (${label}${enforced}):\n\n${file.content}`);
    }

    // Add active rules
    const activeRules = rules.filter((r) => r.isActive);
    for (const rule of activeRules) {
      const patterns =
        rule.patterns.length > 0 ? ` (applies to: ${rule.patterns.join(', ')})` : '';
      parts.push(`Rule from ${rule.path}${patterns}:\n\n${rule.content}`);
    }

    if (parts.length === 0) {
      return '';
    }

    return parts.join('\n\n---\n\n');
  }

  private getLevelLabel(level: MemoryLevel, namespace: MemoryNamespace): string {
    const levelLabels: Record<MemoryLevel, string> = {
      enterprise: 'enterprise policy',
      user: "user's private global instructions",
      'user-rules': 'user rules',
      extra: 'extra config',
      project: 'project instructions',
      'project-rules': 'project rules',
      local: 'local personal notes',
    };
    const namespaceLabel = namespace === 'gencode' ? 'gencode' : namespace === 'claude' ? 'claude' : 'extra';
    return `${levelLabels[level]} - ${namespaceLabel}`;
  }

  /**
   * Load enterprise-level memory files
   */
  private async loadEnterpriseMemory(): Promise<MemoryFile[]> {
    const files: MemoryFile[] = [];
    const managedPaths = getManagedPaths();

    // Load Claude first (lower priority)
    const claudeFile = await this.loadFile(
      path.join(managedPaths.claude, this.config.claudeFilename),
      'enterprise',
      'claude'
    );
    if (claudeFile) {
      claudeFile.enforced = true;
      files.push(claudeFile);
    }

    // Load GenCode second (higher priority)
    const gencodeFile = await this.loadFile(
      path.join(managedPaths.gencode, this.config.gencodeFilename),
      'enterprise',
      'gencode'
    );
    if (gencodeFile) {
      gencodeFile.enforced = true;
      files.push(gencodeFile);
    }

    return files;
  }

  /**
   * Load user-level memory files (both claude and gencode)
   */
  private async loadUserMemory(): Promise<MemoryFile[]> {
    const home = os.homedir();
    const files: MemoryFile[] = [];

    // Load Claude first (lower priority)
    const claudeFile = await this.loadFile(
      path.join(home, this.config.claudeDir, this.config.claudeFilename),
      'user',
      'claude'
    );
    if (claudeFile) files.push(claudeFile);

    // Load GenCode second (higher priority)
    const gencodeFile = await this.loadFile(
      path.join(home, this.config.gencodeDir, this.config.gencodeFilename),
      'user',
      'gencode'
    );
    if (gencodeFile) files.push(gencodeFile);

    return files;
  }

  /**
   * Load user-level rules (both claude and gencode)
   */
  private async loadUserRules(): Promise<MemoryRule[]> {
    const home = os.homedir();
    const rules: MemoryRule[] = [];

    // Load Claude rules first (lower priority)
    const claudeRulesDir = path.join(home, this.config.claudeDir, this.config.rulesDir);
    const claudeRules = await this.loadRulesFromDir(claudeRulesDir, 'user-rules', 'claude');
    rules.push(...claudeRules);

    // Load GenCode rules second (higher priority)
    const gencodeRulesDir = path.join(home, this.config.gencodeDir, this.config.rulesDir);
    const gencodeRules = await this.loadRulesFromDir(gencodeRulesDir, 'user-rules', 'gencode');
    rules.push(...gencodeRules);

    return rules;
  }

  /**
   * Load extra config dirs memory
   */
  private async loadExtraMemory(): Promise<MemoryFile[]> {
    const extraDirs = this.parseExtraConfigDirs();
    const files: MemoryFile[] = [];

    for (const dir of extraDirs) {
      // Try CLAUDE.md
      const claudeFile = await this.loadFile(
        path.join(dir, this.config.claudeFilename),
        'extra',
        'extra'
      );
      if (claudeFile) files.push(claudeFile);

      // Try AGENT.md
      const gencodeFile = await this.loadFile(
        path.join(dir, this.config.gencodeFilename),
        'extra',
        'extra'
      );
      if (gencodeFile) files.push(gencodeFile);
    }

    return files;
  }

  /**
   * Parse GENCODE_CONFIG_DIRS environment variable
   */
  private parseExtraConfigDirs(): string[] {
    const value = process.env[GENCODE_CONFIG_DIRS_ENV];
    if (!value) return [];

    return value
      .split(':')
      .map((dir) => dir.trim())
      .filter((dir) => dir.length > 0)
      .map((dir) => dir.replace(/^~/, os.homedir()));
  }

  /**
   * Load project-level memory files (both claude and gencode)
   */
  private async loadProjectMemory(cwd: string, projectRoot: string): Promise<MemoryFile[]> {
    const files: MemoryFile[] = [];

    // Load from project root - Claude files first
    const claudeCandidates = [
      path.join(projectRoot, this.config.claudeFilename),
      path.join(projectRoot, this.config.claudeDir, this.config.claudeFilename),
    ];

    for (const filePath of claudeCandidates) {
      const file = await this.loadFile(filePath, 'project', 'claude');
      if (file) {
        files.push(file);
        break; // Only load one claude file
      }
    }

    // Load from project root - GenCode files second
    const gencodeCandidates = [
      path.join(projectRoot, this.config.gencodeFilename),
      path.join(projectRoot, this.config.gencodeDir, this.config.gencodeFilename),
    ];

    for (const filePath of gencodeCandidates) {
      const file = await this.loadFile(filePath, 'project', 'gencode');
      if (file) {
        files.push(file);
        break; // Only load one gencode file
      }
    }

    return files;
  }

  /**
   * Load project-level rules (both claude and gencode)
   */
  private async loadProjectRules(projectRoot: string): Promise<MemoryRule[]> {
    const rules: MemoryRule[] = [];

    // Load Claude rules first (lower priority)
    const claudeRulesDir = path.join(projectRoot, this.config.claudeDir, this.config.rulesDir);
    const claudeRules = await this.loadRulesFromDir(claudeRulesDir, 'project-rules', 'claude');
    rules.push(...claudeRules);

    // Load GenCode rules second (higher priority)
    const gencodeRulesDir = path.join(projectRoot, this.config.gencodeDir, this.config.rulesDir);
    const gencodeRules = await this.loadRulesFromDir(gencodeRulesDir, 'project-rules', 'gencode');
    rules.push(...gencodeRules);

    return rules;
  }

  /**
   * Load local memory files (both claude and gencode)
   */
  private async loadLocalMemory(projectRoot: string): Promise<MemoryFile[]> {
    const files: MemoryFile[] = [];

    // Load Claude local files first
    const claudeCandidates = [
      path.join(projectRoot, this.config.claudeLocalFilename),
      path.join(projectRoot, this.config.claudeDir, this.config.claudeLocalFilename),
    ];

    for (const filePath of claudeCandidates) {
      const file = await this.loadFile(filePath, 'local', 'claude');
      if (file) {
        files.push(file);
        break;
      }
    }

    // Load GenCode local files second
    const gencodeCandidates = [
      path.join(projectRoot, this.config.gencodeLocalFilename),
      path.join(projectRoot, this.config.gencodeDir, this.config.gencodeLocalFilename),
    ];

    for (const filePath of gencodeCandidates) {
      const file = await this.loadFile(filePath, 'local', 'gencode');
      if (file) {
        files.push(file);
        break;
      }
    }

    return files;
  }

  /**
   * Load rules from a directory
   */
  private async loadRulesFromDir(
    rulesDir: string,
    level: 'user-rules' | 'project-rules',
    namespace: MemoryNamespace
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
            namespace,
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
  private async loadFile(
    filePath: string,
    level: MemoryLevel,
    namespace: MemoryNamespace
  ): Promise<MemoryFile | null> {
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
        namespace,
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
      const dir = path.join(home, this.config.gencodeDir);
      await fs.mkdir(dir, { recursive: true });
      filePath = path.join(dir, this.config.gencodeFilename);
    } else {
      const projectRoot = await this.findProjectRoot(cwd);
      filePath = path.join(projectRoot, this.config.gencodeFilename);
    }

    // Read existing content
    let existing = '';
    try {
      existing = await fs.readFile(filePath, 'utf-8');
    } catch {
      // File doesn't exist, create with header
      existing = `# ${this.config.gencodeFilename.replace('.md', '')}\n\nThis file provides guidance when working with code in this repository.\n\n`;
    }

    // Append new content
    const newContent = `${existing.trimEnd()}\n\n${content}\n`;
    await fs.writeFile(filePath, newContent, 'utf-8');

    return filePath;
  }

  /**
   * Get list of loaded files for /memory command
   */
  getLoadedFileList(): Array<{
    path: string;
    level: string;
    namespace: string;
    size: number;
    type: 'file' | 'rule';
  }> {
    if (!this.loadedMemory) return [];

    const list: Array<{
      path: string;
      level: string;
      namespace: string;
      size: number;
      type: 'file' | 'rule';
    }> = [];

    for (const f of this.loadedMemory.files) {
      list.push({
        path: f.path,
        level: f.level,
        namespace: f.namespace,
        size: f.content.length,
        type: 'file',
      });
    }

    for (const r of this.loadedMemory.rules) {
      list.push({
        path: r.path,
        level: r.level,
        namespace: r.namespace,
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
    return path.join(cwd, this.config.gencodeFilename);
  }

  /**
   * Check if project memory already exists
   */
  async hasProjectMemory(cwd: string): Promise<boolean> {
    const projectRoot = await this.findProjectRoot(cwd);
    const candidates = [
      path.join(projectRoot, this.config.gencodeFilename),
      path.join(projectRoot, this.config.gencodeDir, this.config.gencodeFilename),
      path.join(projectRoot, this.config.claudeFilename),
      path.join(projectRoot, this.config.claudeDir, this.config.claudeFilename),
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
      path.join(projectRoot, this.config.gencodeFilename),
      path.join(projectRoot, this.config.gencodeDir, this.config.gencodeFilename),
      path.join(projectRoot, this.config.claudeFilename),
      path.join(projectRoot, this.config.claudeDir, this.config.claudeFilename),
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

  /**
   * Get debug summary of loaded memory
   */
  getDebugSummary(): string {
    if (!this.loadedMemory) return 'Memory not loaded';

    const lines: string[] = ['Memory Sources (in load order):'];

    for (const source of this.loadedMemory.sources) {
      const marker = source.level === 'enterprise' ? ' [enforced]' : '';
      lines.push(`  ${source.level}:${source.namespace} - ${source.path} (${source.size} bytes)${marker}`);
    }

    if (this.loadedMemory.errors.length > 0) {
      lines.push('');
      lines.push('Errors:');
      for (const error of this.loadedMemory.errors) {
        lines.push(`  - ${error}`);
      }
    }

    return lines.join('\n');
  }
}
