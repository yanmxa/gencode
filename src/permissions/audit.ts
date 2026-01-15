/**
 * Permission Audit - Log permission decisions for transparency
 *
 * Maintains an in-memory audit log with optional file persistence.
 * Useful for debugging, security review, and compliance.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import type { PermissionAuditEntry, AuditDecision } from './types.js';

const AUDIT_FILE = 'permission-audit.json';
const MAX_MEMORY_ENTRIES = 1000;
const MAX_FILE_ENTRIES = 10000;
const GLOBAL_DIR = path.join(os.homedir(), '.gencode');

/**
 * Summarize tool input for audit (avoid storing sensitive data)
 */
function summarizeInput(tool: string, input: unknown): string {
  if (input === null || input === undefined) return '';

  if (typeof input === 'string') {
    return input.slice(0, 100);
  }

  if (typeof input !== 'object') {
    return String(input).slice(0, 100);
  }

  const obj = input as Record<string, unknown>;

  // Tool-specific summaries
  switch (tool) {
    case 'Bash':
      return (obj.command as string)?.slice(0, 100) ?? '';

    case 'Read':
    case 'Write':
    case 'Edit':
    case 'Glob':
      return (obj.file_path as string) ?? (obj.path as string) ?? '';

    case 'Grep':
      return `${obj.pattern ?? ''} in ${obj.path ?? '.'}`;

    case 'WebFetch':
      return (obj.url as string)?.slice(0, 100) ?? '';

    case 'WebSearch':
      return (obj.query as string)?.slice(0, 100) ?? '';

    default:
      // Generic summary
      const keys = Object.keys(obj).slice(0, 3);
      return keys.map((k) => `${k}:${String(obj[k]).slice(0, 20)}`).join(', ');
  }
}

/**
 * Permission Audit Logger
 */
export class PermissionAudit {
  private entries: PermissionAuditEntry[] = [];
  private persistToFile: boolean;
  private filePath: string;

  constructor(options: { persistToFile?: boolean; auditDir?: string } = {}) {
    this.persistToFile = options.persistToFile ?? false;
    const dir = options.auditDir ?? GLOBAL_DIR;
    this.filePath = path.join(dir, AUDIT_FILE);
  }

  /**
   * Log a permission decision
   */
  async log(
    tool: string,
    input: unknown,
    decision: AuditDecision,
    reason: string,
    options: {
      matchedRule?: string;
      sessionId?: string;
    } = {}
  ): Promise<void> {
    const entry: PermissionAuditEntry = {
      timestamp: new Date(),
      tool,
      inputSummary: summarizeInput(tool, input),
      decision,
      reason,
      matchedRule: options.matchedRule,
      sessionId: options.sessionId,
    };

    // Add to memory
    this.entries.push(entry);

    // Trim if too large
    if (this.entries.length > MAX_MEMORY_ENTRIES) {
      this.entries = this.entries.slice(-MAX_MEMORY_ENTRIES);
    }

    // Persist if enabled
    if (this.persistToFile) {
      await this.appendToFile(entry);
    }
  }

  /**
   * Append entry to file
   */
  private async appendToFile(entry: PermissionAuditEntry): Promise<void> {
    try {
      // Ensure directory exists
      const dir = path.dirname(this.filePath);
      await fs.mkdir(dir, { recursive: true });

      // Load existing entries
      let fileEntries: PermissionAuditEntry[] = [];
      try {
        const content = await fs.readFile(this.filePath, 'utf-8');
        fileEntries = JSON.parse(content);
      } catch {
        // File doesn't exist
      }

      // Add new entry
      fileEntries.push(entry);

      // Trim if too large
      if (fileEntries.length > MAX_FILE_ENTRIES) {
        fileEntries = fileEntries.slice(-MAX_FILE_ENTRIES);
      }

      // Write back
      await fs.writeFile(
        this.filePath,
        JSON.stringify(fileEntries, null, 2),
        'utf-8'
      );
    } catch {
      // Silently fail - audit should not break the app
    }
  }

  /**
   * Get recent audit entries
   */
  getRecent(count: number = 50): PermissionAuditEntry[] {
    return this.entries.slice(-count);
  }

  /**
   * Get all entries in memory
   */
  getAll(): PermissionAuditEntry[] {
    return [...this.entries];
  }

  /**
   * Get entries by tool
   */
  getByTool(tool: string): PermissionAuditEntry[] {
    return this.entries.filter((e) => e.tool === tool);
  }

  /**
   * Get entries by decision
   */
  getByDecision(decision: AuditDecision): PermissionAuditEntry[] {
    return this.entries.filter((e) => e.decision === decision);
  }

  /**
   * Get entries by session
   */
  getBySession(sessionId: string): PermissionAuditEntry[] {
    return this.entries.filter((e) => e.sessionId === sessionId);
  }

  /**
   * Get statistics
   */
  getStats(): {
    total: number;
    allowed: number;
    denied: number;
    confirmed: number;
    rejected: number;
    byTool: Record<string, number>;
  } {
    const stats = {
      total: this.entries.length,
      allowed: 0,
      denied: 0,
      confirmed: 0,
      rejected: 0,
      byTool: {} as Record<string, number>,
    };

    for (const entry of this.entries) {
      // Count by decision
      switch (entry.decision) {
        case 'allowed':
          stats.allowed++;
          break;
        case 'denied':
          stats.denied++;
          break;
        case 'confirmed':
          stats.confirmed++;
          break;
        case 'rejected':
          stats.rejected++;
          break;
      }

      // Count by tool
      stats.byTool[entry.tool] = (stats.byTool[entry.tool] ?? 0) + 1;
    }

    return stats;
  }

  /**
   * Clear in-memory entries
   */
  clear(): void {
    this.entries = [];
  }

  /**
   * Load entries from file
   */
  async loadFromFile(): Promise<PermissionAuditEntry[]> {
    try {
      const content = await fs.readFile(this.filePath, 'utf-8');
      return JSON.parse(content);
    } catch {
      return [];
    }
  }

  /**
   * Clear file entries
   */
  async clearFile(): Promise<void> {
    try {
      await fs.writeFile(this.filePath, '[]', 'utf-8');
    } catch {
      // Silently fail
    }
  }

  /**
   * Format entry for display
   */
  formatEntry(entry: PermissionAuditEntry): string {
    const time = entry.timestamp.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
    });

    const decision = entry.decision.toUpperCase().padEnd(9);
    const tool = entry.tool.padEnd(10);
    const input = entry.inputSummary.slice(0, 40).padEnd(40);

    return `${time}  ${decision}  ${tool}  ${input}`;
  }

  /**
   * Format entries as table
   */
  formatTable(entries: PermissionAuditEntry[]): string {
    const header = 'Time      Decision   Tool        Input';
    const separator = '─'.repeat(header.length);
    const rows = entries.map((e) => this.formatEntry(e));

    return [header, separator, ...rows].join('\n');
  }
}
