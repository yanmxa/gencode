/**
 * Session Manager - Handles session persistence and retrieval
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import * as crypto from 'crypto';
import { EventEmitter } from 'events';
import type { Message, MessageContent } from '../providers/types.js';
import type {
  Session,
  SessionMetadata,
  SessionListItem,
  SessionConfig,
} from './types.js';
import { DEFAULT_SESSION_CONFIG } from './types.js';
import { CompressionEngine } from './compression/engine.js';
import type { ConversationSummary, ModelInfo } from './compression/types.js';
import { getCheckpointManager, initCheckpointManager } from './checkpointing/index.js';

// Provider interface (minimal subset needed for compression)
interface LLMProvider {
  complete(options: {
    model: string;
    messages: Message[];
    maxTokens?: number;
  }): Promise<{ content: string | MessageContent[] }>;
  getModel?(): string;
}

export class SessionManager extends EventEmitter {
  private config: SessionConfig;
  private storageDir: string;
  private currentSession: Session | null = null;
  private compressionEngine: CompressionEngine | null = null;
  private cumulativeTokens: {
    input: number;
    output: number;
    total: number;
  } = { input: 0, output: 0, total: 0 };

  constructor(config?: Partial<SessionConfig>) {
    super();
    this.config = { ...DEFAULT_SESSION_CONFIG, ...config };
    this.storageDir = this.config.storageDir.replace('~', os.homedir());
  }

  /**
   * Set compression engine (requires LLM provider)
   */
  setCompressionEngine(provider: LLMProvider, model?: string): void {
    const compressionConfig = {
      ...this.config.compression,
      model: model || this.config.compression?.model,
    };
    this.compressionEngine = new CompressionEngine(provider, compressionConfig);
  }

  /**
   * Calculate cumulative tokens from session metadata
   */
  private calculateCumulativeTokens(session: Session): void {
    let inputTotal = 0;
    let outputTotal = 0;

    // Sum from completions metadata
    if (session.metadata.completions) {
      for (const completion of session.metadata.completions) {
        if (completion.usage) {
          inputTotal += completion.usage.inputTokens;
          outputTotal += completion.usage.outputTokens;
        }
      }
    }

    // Also check session-level tokenUsage (for backwards compatibility)
    if (session.metadata.tokenUsage) {
      inputTotal = Math.max(inputTotal, session.metadata.tokenUsage.input);
      outputTotal = Math.max(outputTotal, session.metadata.tokenUsage.output);
    }

    this.cumulativeTokens = {
      input: inputTotal,
      output: outputTotal,
      total: inputTotal + outputTotal,
    };
  }

  /**
   * Update cumulative token usage from latest completion
   */
  private updateTokenUsageFromLatestCompletion(): void {
    if (!this.currentSession?.metadata.completions) return;

    const completions = this.currentSession.metadata.completions;
    if (completions.length === 0) return;

    // Get the latest completion
    const latest = completions[completions.length - 1];
    if (latest.usage) {
      this.cumulativeTokens.input += latest.usage.inputTokens;
      this.cumulativeTokens.output += latest.usage.outputTokens;
      this.cumulativeTokens.total = this.cumulativeTokens.input + this.cumulativeTokens.output;
    }
  }

  /**
   * Initialize storage directory
   */
  async init(): Promise<void> {
    await fs.mkdir(this.storageDir, { recursive: true });
  }

  /**
   * Generate a new session ID
   */
  private generateId(): string {
    const timestamp = Date.now().toString(36);
    const random = crypto.randomBytes(4).toString('hex');
    return `${timestamp}-${random}`;
  }

  /**
   * Get session file path
   */
  private getSessionPath(id: string): string {
    return path.join(this.storageDir, `${id}.json`);
  }

  /**
   * Create a new session
   */
  async create(options: {
    provider: string;
    model: string;
    cwd?: string;
    title?: string;
    parentId?: string;
  }): Promise<Session> {
    const id = this.generateId();
    const now = new Date().toISOString();

    const session: Session = {
      metadata: {
        id,
        title: options.title ?? `Session ${new Date().toLocaleString()}`,
        createdAt: now,
        updatedAt: now,
        provider: options.provider,
        model: options.model,
        cwd: options.cwd ?? process.cwd(),
        messageCount: 0,
        parentId: options.parentId,
      },
      messages: [],
    };

    this.currentSession = session;

    // Initialize cumulative tokens to zero for new session
    this.cumulativeTokens = { input: 0, output: 0, total: 0 };

    if (this.config.autoSave) {
      await this.save(session);
    }

    return session;
  }

  /**
   * Fork an existing session
   */
  async fork(sessionId: string, title?: string): Promise<Session> {
    const parent = await this.load(sessionId);
    if (!parent) {
      throw new Error(`Session not found: ${sessionId}`);
    }

    const id = this.generateId();
    const now = new Date().toISOString();

    const forked: Session = {
      metadata: {
        ...parent.metadata,
        id,
        title: title ?? `Fork of ${parent.metadata.title}`,
        createdAt: now,
        updatedAt: now,
        parentId: sessionId,
      },
      messages: [...parent.messages],
      systemPrompt: parent.systemPrompt,
      summaries: parent.summaries,
      fullMessageCount: parent.fullMessageCount,
      // Copy checkpoints from parent
      checkpoints: parent.checkpoints ? [...parent.checkpoints] : undefined,
    };

    this.currentSession = forked;

    // Inherit token usage from parent (already calculated by load())
    // Token counts remain the same since we copy the messages and completions
    this.calculateCumulativeTokens(forked);

    if (this.config.autoSave) {
      await this.save(forked);
    }

    return forked;
  }

  /**
   * Save a session to disk
   */
  async save(session: Session): Promise<void> {
    await this.init();
    session.metadata.updatedAt = new Date().toISOString();
    session.metadata.messageCount = session.messages.length;

    // Save cumulative token usage to metadata
    if (this.cumulativeTokens.total > 0) {
      session.metadata.tokenUsage = {
        input: this.cumulativeTokens.input,
        output: this.cumulativeTokens.output,
      };
    }

    // Save checkpoints from current session
    const checkpointManager = getCheckpointManager();
    if (checkpointManager && checkpointManager.getSessionId() === session.metadata.id) {
      session.checkpoints = checkpointManager.serialize() as any;
    }

    const filePath = this.getSessionPath(session.metadata.id);
    await fs.writeFile(filePath, JSON.stringify(session, null, 2), 'utf-8');
  }

  /**
   * Load a session from disk
   */
  async load(id: string): Promise<Session | null> {
    try {
      const filePath = this.getSessionPath(id);
      const content = await fs.readFile(filePath, 'utf-8');
      const session = JSON.parse(content) as Session;
      this.currentSession = session;

      // Calculate cumulative tokens from session metadata
      this.calculateCumulativeTokens(session);

      // Restore checkpoints to CheckpointManager
      if (session.checkpoints && session.checkpoints.length > 0) {
        const checkpointManager = initCheckpointManager(id);
        checkpointManager.deserialize(session.checkpoints as any);
      }

      return session;
    } catch {
      return null;
    }
  }

  /**
   * Resume the most recent session
   */
  async resumeLatest(): Promise<Session | null> {
    const sessions = await this.list();
    if (sessions.length === 0) {
      return null;
    }
    return this.load(sessions[0].id);
  }

  /**
   * Resume by index (1-based, most recent first)
   */
  async resumeByIndex(index: number): Promise<Session | null> {
    const sessions = await this.list();
    if (index < 1 || index > sessions.length) {
      return null;
    }
    return this.load(sessions[index - 1].id);
  }

  /**
   * List sessions, optionally filtered by project directory
   */
  async list(options?: { cwd?: string; all?: boolean }): Promise<SessionListItem[]> {
    await this.init();
    const filterCwd = options?.all ? undefined : (options?.cwd ?? process.cwd());

    try {
      const files = await fs.readdir(this.storageDir);
      const sessions: SessionListItem[] = [];

      for (const file of files) {
        if (!file.endsWith('.json')) continue;

        try {
          const filePath = path.join(this.storageDir, file);
          const content = await fs.readFile(filePath, 'utf-8');
          const session = JSON.parse(content) as Session;

          // Filter by cwd if specified
          if (filterCwd && session.metadata.cwd !== filterCwd) {
            continue;
          }

          // Get first user message as preview
          const firstUserMsg = session.messages.find((m) => m.role === 'user');
          let preview = '';
          if (firstUserMsg) {
            const text =
              typeof firstUserMsg.content === 'string'
                ? firstUserMsg.content
                : firstUserMsg.content
                    .filter((c) => c.type === 'text')
                    .map((c) => (c as { text: string }).text)
                    .join(' ');
            preview = text.slice(0, 80) + (text.length > 80 ? '...' : '');
          }

          sessions.push({
            id: session.metadata.id,
            title: session.metadata.title,
            updatedAt: session.metadata.updatedAt,
            messageCount: session.metadata.messageCount,
            preview,
          });
        } catch {
          // Skip invalid files
        }
      }

      // Sort by updated time, newest first
      sessions.sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime());

      return sessions;
    } catch {
      return [];
    }
  }

  /**
   * Delete a session
   */
  async delete(id: string): Promise<boolean> {
    try {
      const filePath = this.getSessionPath(id);
      await fs.unlink(filePath);

      if (this.currentSession?.metadata.id === id) {
        this.currentSession = null;
      }

      return true;
    } catch {
      return false;
    }
  }

  /**
   * Clear old sessions based on config
   */
  async cleanup(): Promise<number> {
    const sessions = await this.list();
    let deleted = 0;

    const now = Date.now();
    const maxAge = this.config.maxAge * 24 * 60 * 60 * 1000;

    for (let i = 0; i < sessions.length; i++) {
      const session = sessions[i];
      const age = now - new Date(session.updatedAt).getTime();

      // Delete if too old or exceeds max count
      if (age > maxAge || i >= this.config.maxSessions) {
        if (await this.delete(session.id)) {
          deleted++;
        }
      }
    }

    return deleted;
  }

  /**
   * Get current session
   */
  getCurrent(): Session | null {
    return this.currentSession;
  }

  /**
   * Set current session
   */
  setCurrent(session: Session): void {
    this.currentSession = session;
  }

  /**
   * Add message to current session
   */
  async addMessage(message: Message, modelInfo?: ModelInfo): Promise<void> {
    if (!this.currentSession) {
      throw new Error('No active session');
    }

    this.currentSession.messages.push(message);

    // Update full message count
    if (!this.currentSession.fullMessageCount) {
      this.currentSession.fullMessageCount = this.currentSession.messages.length;
    } else {
      this.currentSession.fullMessageCount++;
    }

    // Update cumulative token usage from latest completion
    this.updateTokenUsageFromLatestCompletion();

    // Check if compression is needed
    if (this.compressionEngine && modelInfo) {
      // Pass actual token usage to compression engine
      const tokenUsage = this.cumulativeTokens.total > 0
        ? { input: this.cumulativeTokens.input, output: this.cumulativeTokens.output }
        : undefined;

      const { needed, strategy, usagePercent, shouldWarn } =
        this.compressionEngine.needsCompression(
          this.currentSession.messages,
          modelInfo,
          tokenUsage
        );

      // Emit warning event at 80% usage
      if (shouldWarn && !needed && usagePercent) {
        this.emit('context-warning', {
          usagePercent,
          totalTokens: this.cumulativeTokens.total,
          maxTokens: modelInfo.contextWindow,
        });
      }

      // Handle auto-compaction at 90%
      if (needed) {
        // Emit before auto-compact
        if (usagePercent) {
          this.emit('auto-compacting', { strategy, usagePercent });
        }

        if (strategy === 'prune') {
          await this.pruneToolOutputs();
        } else if (strategy === 'compact') {
          await this.performCompaction(modelInfo);
        }

        // Emit after compression complete
        this.emit('compaction-complete', { strategy });
      }
    }

    if (this.config.autoSave) {
      await this.save(this.currentSession);
    }
  }

  /**
   * Update session title
   */
  async updateTitle(id: string, title: string): Promise<void> {
    const session = await this.load(id);
    if (session) {
      session.metadata.title = title;
      await this.save(session);
    }
  }

  /**
   * Get messages from current session
   */
  getMessages(): Message[] {
    return this.currentSession?.messages ?? [];
  }

  /**
   * Clear current session messages
   */
  clearMessages(): void {
    if (this.currentSession) {
      this.currentSession.messages = [];
    }
  }

  /**
   * Remove the last message from current session
   */
  removeLastMessage(): void {
    if (this.currentSession && this.currentSession.messages.length > 0) {
      this.currentSession.messages.pop();
    }
  }

  /**
   * Export session to JSON
   */
  async export(id: string): Promise<string> {
    const session = await this.load(id);
    if (!session) {
      throw new Error(`Session not found: ${id}`);
    }
    return JSON.stringify(session, null, 2);
  }

  /**
   * Import session from JSON
   */
  async import(json: string): Promise<Session> {
    const session = JSON.parse(json) as Session;

    // Generate new ID to avoid conflicts
    session.metadata.id = this.generateId();
    session.metadata.createdAt = new Date().toISOString();
    session.metadata.updatedAt = new Date().toISOString();

    await this.save(session);
    return session;
  }

  // ===== Compression Methods =====

  /**
   * Layer 1: Prune tool outputs
   */
  private async pruneToolOutputs(): Promise<void> {
    if (!this.currentSession || !this.compressionEngine) return;

    const result = await this.compressionEngine.pruneToolOutputs(
      this.currentSession.messages
    );

    if (result.pruned) {
      // Optionally log the pruning event
      // console.log(`Pruned ${result.prunedCount} tool outputs, saved ${result.savedTokens} tokens`);
    }
  }

  /**
   * Layer 2: Perform compaction (summarization)
   * Public for manual compaction via /compact command
   */
  async performCompaction(modelInfo: ModelInfo): Promise<void> {
    if (!this.currentSession || !this.compressionEngine) return;

    const session = this.currentSession;

    // Determine range to summarize
    const preserveRecent = 10; // Default, could be configurable
    const summarizeEnd = session.messages.length - preserveRecent - 1;

    // Find start point: after last summary or beginning
    let summarizeStart = 0;
    if (session.summaries && session.summaries.length > 0) {
      const lastSummary = session.summaries[session.summaries.length - 1];
      summarizeStart = lastSummary.coveringMessages[1] + 1;
    } else {
      // Skip system prompt
      const firstSystemIdx = session.messages.findIndex((m) => m.role === 'system');
      if (firstSystemIdx >= 0) {
        summarizeStart = firstSystemIdx + 1;
      }
    }

    if (summarizeEnd <= summarizeStart) return;

    // Generate summary
    const summary = await this.compressionEngine.compact(session.messages, [
      summarizeStart,
      summarizeEnd,
    ]);

    // Store summary
    session.summaries = session.summaries || [];
    session.summaries.push(summary);

    // Replace summarized messages
    const preserved: Message[] = [];

    // Preserve system prompt
    const systemMsg = session.messages.find((m) => m.role === 'system');
    if (systemMsg) {
      preserved.push(systemMsg);
    }

    // Add summary as system message
    const summaryMessage: Message = {
      role: 'system',
      content: this.formatSummaryAsContext(summary),
    };
    preserved.push(summaryMessage);

    // Add recent messages
    const recentMessages = session.messages.slice(summarizeEnd + 1);
    session.messages = [...preserved, ...recentMessages];
  }

  /**
   * Format summary as context message
   */
  private formatSummaryAsContext(summary: ConversationSummary): string {
    const lines: string[] = [];

    lines.push(
      `[Earlier conversation - ${summary.coveringMessages[1] - summary.coveringMessages[0] + 1} messages summarized]`
    );
    lines.push('');
    lines.push(summary.content);

    if (summary.filesModified.length > 0) {
      lines.push('');
      lines.push(`Files modified: ${summary.filesModified.join(', ')}`);
    }

    if (summary.keyDecisions.length > 0) {
      lines.push('');
      lines.push('Key decisions:');
      summary.keyDecisions.forEach((d) => lines.push(`- ${d}`));
    }

    if (summary.toolsUsed.length > 0) {
      lines.push('');
      lines.push('Tools used:');
      summary.toolsUsed.forEach((t) => {
        lines.push(`- ${t.tool}: ${t.count} times`);
      });
    }

    return lines.join('\n');
  }

  /**
   * Get messages for LLM (includes compression context)
   */
  getMessagesForLLM(): Message[] {
    if (!this.currentSession) return [];

    // If no summaries, return all messages
    if (!this.currentSession.summaries || this.currentSession.summaries.length === 0) {
      return this.currentSession.messages;
    }

    // Messages already include summary system message
    return this.currentSession.messages;
  }

  /**
   * Get compression statistics
   */
  getCompressionStats(): {
    totalMessages: number;
    activeMessages: number;
    summaryCount: number;
    compressionRatio: number;
  } | null {
    if (!this.currentSession) return null;

    const totalMessages =
      this.currentSession.fullMessageCount || this.currentSession.messages.length;
    const activeMessages = this.currentSession.messages.length;
    const summaryCount = this.currentSession.summaries?.length || 0;

    return {
      totalMessages,
      activeMessages,
      summaryCount,
      compressionRatio: activeMessages / totalMessages,
    };
  }

  /**
   * Get cumulative token usage for current session
   */
  getTokenUsage(): { input: number; output: number; total: number } {
    return { ...this.cumulativeTokens };
  }
}
