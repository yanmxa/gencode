/**
 * Session Manager - Handles session persistence and retrieval
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import * as crypto from 'crypto';
import type { Message } from '../providers/types.js';
import type {
  Session,
  SessionMetadata,
  SessionListItem,
  SessionConfig,
} from './types.js';
import { DEFAULT_SESSION_CONFIG } from './types.js';

export class SessionManager {
  private config: SessionConfig;
  private storageDir: string;
  private currentSession: Session | null = null;

  constructor(config?: Partial<SessionConfig>) {
    this.config = { ...DEFAULT_SESSION_CONFIG, ...config };
    this.storageDir = this.config.storageDir.replace('~', os.homedir());
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
    };

    this.currentSession = forked;

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
   * List all sessions
   */
  async list(): Promise<SessionListItem[]> {
    await this.init();

    try {
      const files = await fs.readdir(this.storageDir);
      const sessions: SessionListItem[] = [];

      for (const file of files) {
        if (!file.endsWith('.json')) continue;

        try {
          const filePath = path.join(this.storageDir, file);
          const content = await fs.readFile(filePath, 'utf-8');
          const session = JSON.parse(content) as Session;

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
  async addMessage(message: Message): Promise<void> {
    if (!this.currentSession) {
      throw new Error('No active session');
    }

    this.currentSession.messages.push(message);

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
}
