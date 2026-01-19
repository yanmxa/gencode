/**
 * Memory Manager - Core memory system implementation
 *
 * Implements Claude Code compatible memory loading with merge semantics:
 * At each level, both .gen and .claude directories are loaded.
 * Content from .gen appears later in the context (higher priority for LLM).
 *
 * Loading order within each level:
 * 1. .claude files first (lower priority - LLM sees earlier)
 * 2. .gen files second (higher priority - LLM sees later)
 *
 * Level loading order:
 * 1. Enterprise (system-wide managed, enforced)
 * 2. User (~/.gen/ + ~/.claude/)
 * 3. User Rules
 * 4. Extra (GEN_CONFIG)
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
  MemoryMergeStrategy,
} from './types.js';
import { DEFAULT_MEMORY_CONFIG } from './types.js';
import { getManagedPaths, GEN_CONFIG_ENV } from '../../infrastructure/config/types.js';

/**
 * Result of loading files at a level with merge strategy
 */
interface LevelLoadResult {
  files: MemoryFile[];
  skipped: string[];
}

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
    const { cwd, currentFile, strategy = 'fallback' } = options;
    const files: MemoryFile[] = [];
    const rules: MemoryRule[] = [];
    const errors: string[] = [];
    const sources: MemorySource[] = [];
    const skippedFiles: string[] = [];
    let totalSize = 0;

    this.importResolver.reset();
    const projectRoot = await this.findProjectRoot(cwd);
    this.importResolver.setProjectRoot(projectRoot);

    // 1. Load enterprise memory (system-wide, enforced)
    const enterpriseResult = await this.loadEnterpriseMemory(strategy);
    for (const file of enterpriseResult.files) {
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
    skippedFiles.push(...enterpriseResult.skipped);

    // 2. Load user-level memory (both claude and gen)
    const userResult = await this.loadUserMemory(strategy);
    const userFiles = userResult.files;
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
    skippedFiles.push(...userResult.skipped);

    // 3. Load user-level rules (both claude and gen)
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
    const extraResult = await this.loadExtraMemory(strategy);
    const extraFiles = extraResult.files;
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
    skippedFiles.push(...extraResult.skipped);

    // 5. Load project-level memory (both claude and gen, recursive upward)
    const projectResult = await this.loadProjectMemory(cwd, projectRoot, strategy);
    const projectFiles = projectResult.files;
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
    skippedFiles.push(...projectResult.skipped);

    // 6. Load project-level rules (both claude and gen)
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

    // 7. Load local memory (both claude and gen)
    const localResult = await this.loadLocalMemory(projectRoot, strategy);
    const localFiles = localResult.files;
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
    skippedFiles.push(...localResult.skipped);

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
      skippedFiles,
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
    const namespaceLabel = namespace === 'gen' ? 'gen' : namespace === 'claude' ? 'claude' : 'extra';
    return `${levelLabels[level]} - ${namespaceLabel}`;
  }

  /**
   * Apply merge strategy to decide which files to load
   * Returns the files to load and the files to skip
   */
  private async applyMergeStrategy(
    claudeFilePath: string,
    genFilePath: string,
    level: MemoryLevel,
    strategy: MemoryMergeStrategy
  ): Promise<LevelLoadResult> {
    const files: MemoryFile[] = [];
    const skipped: string[] = [];

    switch (strategy) {
      case 'fallback': {
        // Try gen first, fallback to claude
        const genFile = await this.loadFile(genFilePath, level, 'gen');
        if (genFile) {
          files.push(genFile);
          // Only skip claude file if it exists
          const claudeExists = await this.fileExists(claudeFilePath);
          if (claudeExists) {
            skipped.push(claudeFilePath);
          }
        } else {
          const claudeFile = await this.loadFile(claudeFilePath, level, 'claude');
          if (claudeFile) {
            files.push(claudeFile);
          }
        }
        break;
      }
      case 'both': {
        // Load both (claude first for lower priority)
        const claudeFile = await this.loadFile(claudeFilePath, level, 'claude');
        if (claudeFile) files.push(claudeFile);

        const genFile = await this.loadFile(genFilePath, level, 'gen');
        if (genFile) files.push(genFile);
        break;
      }
      case 'gen-only': {
        // Only load gen
        const genFile = await this.loadFile(genFilePath, level, 'gen');
        if (genFile) {
          files.push(genFile);
        }
        // Only mark as skipped if claude file exists
        const claudeExists = await this.fileExists(claudeFilePath);
        if (claudeExists) {
          skipped.push(claudeFilePath);
        }
        break;
      }
      case 'claude-only': {
        // Only load claude
        const claudeFile = await this.loadFile(claudeFilePath, level, 'claude');
        if (claudeFile) {
          files.push(claudeFile);
        }
        // Only mark as skipped if gen file exists
        const genExists = await this.fileExists(genFilePath);
        if (genExists) {
          skipped.push(genFilePath);
        }
        break;
      }
    }

    return { files, skipped };
  }

  /**
   * Check if a file exists
   */
  private async fileExists(filePath: string): Promise<boolean> {
    try {
      await fs.stat(filePath);
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Load enterprise-level memory files
   */
  private async loadEnterpriseMemory(strategy: MemoryMergeStrategy): Promise<LevelLoadResult> {
    const managedPaths = getManagedPaths();
    const claudePath = path.join(managedPaths.claude, this.config.claudeFilename);
    const genPath = path.join(managedPaths.gen, this.config.genFilename);

    const result = await this.applyMergeStrategy(claudePath, genPath, 'enterprise', strategy);

    // Mark all enterprise files as enforced
    for (const file of result.files) {
      file.enforced = true;
    }

    return result;
  }

  /**
   * Load user-level memory files (both claude and gen)
   */
  private async loadUserMemory(strategy: MemoryMergeStrategy): Promise<LevelLoadResult> {
    const home = os.homedir();
    const claudePath = path.join(home, this.config.claudeDir, this.config.claudeFilename);
    const genPath = path.join(home, this.config.genDir, this.config.genFilename);

    return await this.applyMergeStrategy(claudePath, genPath, 'user', strategy);
  }

  /**
   * Load user-level rules (both claude and gen)
   */
  private async loadUserRules(): Promise<MemoryRule[]> {
    const home = os.homedir();
    const rules: MemoryRule[] = [];

    // Load Claude rules first (lower priority)
    const claudeRulesDir = path.join(home, this.config.claudeDir, this.config.rulesDir);
    const claudeRules = await this.loadRulesFromDir(claudeRulesDir, 'user-rules', 'claude');
    rules.push(...claudeRules);

    // Load GenCode rules second (higher priority)
    const genRulesDir = path.join(home, this.config.genDir, this.config.rulesDir);
    const genRules = await this.loadRulesFromDir(genRulesDir, 'user-rules', 'gen');
    rules.push(...genRules);

    return rules;
  }

  /**
   * Load extra config dirs memory
   */
  private async loadExtraMemory(strategy: MemoryMergeStrategy): Promise<LevelLoadResult> {
    const extraDirs = this.parseExtraConfigDirs();
    const files: MemoryFile[] = [];
    const skipped: string[] = [];

    for (const dir of extraDirs) {
      const claudePath = path.join(dir, this.config.claudeFilename);
      const genPath = path.join(dir, this.config.genFilename);
      const result = await this.applyMergeStrategy(claudePath, genPath, 'extra', strategy);

      files.push(...result.files);
      skipped.push(...result.skipped);
    }

    return { files, skipped };
  }

  /**
   * Parse GEN_CONFIG environment variable
   */
  private parseExtraConfigDirs(): string[] {
    const value = process.env[GEN_CONFIG_ENV];
    if (!value) return [];

    return value
      .split(':')
      .map((dir) => dir.trim())
      .filter((dir) => dir.length > 0)
      .map((dir) => dir.replace(/^~/, os.homedir()));
  }

  /**
   * Load project-level memory files (both claude and gen)
   */
  private async loadProjectMemory(
    cwd: string,
    projectRoot: string,
    strategy: MemoryMergeStrategy
  ): Promise<LevelLoadResult> {
    // Find first existing claude file
    const claudeCandidates = [
      path.join(projectRoot, this.config.claudeFilename),
      path.join(projectRoot, this.config.claudeDir, this.config.claudeFilename),
    ];
    let claudePath = claudeCandidates[0]; // Default for skipped tracking
    for (const candidate of claudeCandidates) {
      try {
        await fs.stat(candidate);
        claudePath = candidate;
        break;
      } catch {
        continue;
      }
    }

    // Find first existing gen file
    const genCandidates = [
      path.join(projectRoot, this.config.genFilename),
      path.join(projectRoot, this.config.genDir, this.config.genFilename),
    ];
    let genPath = genCandidates[0]; // Default for skipped tracking
    for (const candidate of genCandidates) {
      try {
        await fs.stat(candidate);
        genPath = candidate;
        break;
      } catch {
        continue;
      }
    }

    return await this.applyMergeStrategy(claudePath, genPath, 'project', strategy);
  }

  /**
   * Load project-level rules (both claude and gen)
   */
  private async loadProjectRules(projectRoot: string): Promise<MemoryRule[]> {
    const rules: MemoryRule[] = [];

    // Load Claude rules first (lower priority)
    const claudeRulesDir = path.join(projectRoot, this.config.claudeDir, this.config.rulesDir);
    const claudeRules = await this.loadRulesFromDir(claudeRulesDir, 'project-rules', 'claude');
    rules.push(...claudeRules);

    // Load GenCode rules second (higher priority)
    const genRulesDir = path.join(projectRoot, this.config.genDir, this.config.rulesDir);
    const genRules = await this.loadRulesFromDir(genRulesDir, 'project-rules', 'gen');
    rules.push(...genRules);

    return rules;
  }

  /**
   * Load local memory files (both claude and gen)
   */
  private async loadLocalMemory(
    projectRoot: string,
    strategy: MemoryMergeStrategy
  ): Promise<LevelLoadResult> {
    // Find first existing claude local file
    const claudeCandidates = [
      path.join(projectRoot, this.config.claudeLocalFilename),
      path.join(projectRoot, this.config.claudeDir, this.config.claudeLocalFilename),
    ];
    let claudePath = claudeCandidates[0];
    for (const candidate of claudeCandidates) {
      try {
        await fs.stat(candidate);
        claudePath = candidate;
        break;
      } catch {
        continue;
      }
    }

    // Find first existing gen local file
    const genCandidates = [
      path.join(projectRoot, this.config.genLocalFilename),
      path.join(projectRoot, this.config.genDir, this.config.genLocalFilename),
    ];
    let genPath = genCandidates[0];
    for (const candidate of genCandidates) {
      try {
        await fs.stat(candidate);
        genPath = candidate;
        break;
      } catch {
        continue;
      }
    }

    return await this.applyMergeStrategy(claudePath, genPath, 'local', strategy);
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
      const dir = path.join(home, this.config.genDir);
      await fs.mkdir(dir, { recursive: true });
      filePath = path.join(dir, this.config.genFilename);
    } else {
      const projectRoot = await this.findProjectRoot(cwd);
      filePath = path.join(projectRoot, this.config.genFilename);
    }

    // Read existing content
    let existing = '';
    try {
      existing = await fs.readFile(filePath, 'utf-8');
    } catch {
      // File doesn't exist, create with header
      existing = `# ${this.config.genFilename.replace('.md', '')}\n\nThis file provides guidance when working with code in this repository.\n\n`;
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
    return path.join(cwd, this.config.genFilename);
  }

  /**
   * Check if project memory already exists
   */
  async hasProjectMemory(cwd: string): Promise<boolean> {
    const projectRoot = await this.findProjectRoot(cwd);
    const candidates = [
      path.join(projectRoot, this.config.genFilename),
      path.join(projectRoot, this.config.genDir, this.config.genFilename),
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
      path.join(projectRoot, this.config.genFilename),
      path.join(projectRoot, this.config.genDir, this.config.genFilename),
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

  /**
   * Get verbose loading summary with strategy info
   */
  getVerboseSummary(strategy: MemoryMergeStrategy): string {
    if (!this.loadedMemory) return 'Memory not loaded';

    const lines: string[] = [];
    const kbLoaded = (this.loadedMemory.totalSize / 1024).toFixed(1);

    lines.push(`[Memory] Strategy: ${strategy}`);

    // Group sources by level
    const byLevel = new Map<string, typeof this.loadedMemory.sources>();
    for (const source of this.loadedMemory.sources) {
      const key = source.level;
      if (!byLevel.has(key)) {
        byLevel.set(key, []);
      }
      byLevel.get(key)!.push(source);
    }

    // Show what was loaded per level
    for (const [level, sources] of byLevel) {
      for (const source of sources) {
        const sizeKb = (source.size / 1024).toFixed(1);
        const marker = source.level === 'enterprise' ? ' [enforced]' : '';
        lines.push(`[Memory] ${level}: ${source.path} (${sizeKb} KB)${marker}`);
      }
    }

    // Show what was skipped
    if (this.loadedMemory.skippedFiles.length > 0) {
      for (const skipped of this.loadedMemory.skippedFiles) {
        lines.push(`[Memory] Skipped: ${skipped}`);
      }
    }

    lines.push(
      `[Memory] Total: ${kbLoaded} KB (${this.loadedMemory.files.length} files loaded, ${this.loadedMemory.skippedFiles.length} skipped)`
    );

    if (this.loadedMemory.errors.length > 0) {
      lines.push('[Memory] Errors:');
      for (const error of this.loadedMemory.errors) {
        lines.push(`  - ${error}`);
      }
    }

    return lines.join('\n');
  }
}
