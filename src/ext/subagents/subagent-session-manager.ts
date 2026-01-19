/**
 * SubagentSessionManager - Manages session persistence for subagents
 *
 * Responsibilities:
 * - Create subagent sessions with metadata
 * - Validate sessions for resume (expiry, quota)
 * - List and filter subagent sessions
 * - Clean up expired sessions
 */

import { SessionManager } from '../../core/session/manager.js';
import type { Session, SessionListItem } from '../../core/session/types.js';
import type { SubagentType, TaskOutput } from './types.js';
import type { Agent } from '../../core/agent/agent.js';

/**
 * Default session retention period (7 days)
 */
const DEFAULT_RETENTION_DAYS = 7;

/**
 * Maximum resume count (5 resumes)
 */
const MAX_RESUME_COUNT = 5;

/**
 * Session validation result
 */
export interface SessionValidationResult {
  valid: boolean;
  reason?: string;
}

/**
 * Filter options for listing subagent sessions
 */
export interface SubagentSessionFilter {
  type?: SubagentType;
  expired?: boolean;
  parentId?: string;
}

/**
 * SubagentSessionManager - Wraps SessionManager with subagent-specific functionality
 */
export class SubagentSessionManager {
  private sessionManager: SessionManager;

  constructor(sessionManager?: SessionManager) {
    this.sessionManager = sessionManager ?? new SessionManager();
  }

  /**
   * Create a new subagent session with metadata
   */
  async createSubagentSession(
    subagentType: SubagentType,
    description: string,
    parentSessionId?: string
  ): Promise<string> {
    // Generate session ID
    const sessionId = this.generateSessionId();

    // Calculate expiry date (7 days from now)
    const expiresAt = new Date();
    expiresAt.setDate(expiresAt.getDate() + DEFAULT_RETENTION_DAYS);

    // Create session with subagent metadata
    const session: Session = {
      metadata: {
        id: sessionId,
        title: `Subagent: ${description}`,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        provider: 'anthropic', // Will be updated by agent
        model: '', // Will be updated by agent
        cwd: process.cwd(),
        messageCount: 0,
        isSubagentSession: true,
        subagentType,
        parentSessionId,
        resumeCount: 0,
        expiresAt: expiresAt.toISOString(),
        originalDescription: description,
      },
      messages: [],
    };

    // Save session
    await this.sessionManager.save(session);

    return sessionId;
  }

  /**
   * Save subagent session from agent
   */
  async saveSubagentSession(sessionId: string, agent: Agent): Promise<void> {
    // Load current session
    const session = await this.sessionManager.load(sessionId);
    if (!session) {
      throw new Error(`Session not found: ${sessionId}`);
    }

    // Update session from agent state
    session.messages = agent.getHistory();
    session.metadata.messageCount = session.messages.length;
    session.metadata.updatedAt = new Date().toISOString();

    // Save updated session
    await this.sessionManager.save(session);
  }

  /**
   * List subagent sessions with optional filters
   */
  async listSubagentSessions(filter?: SubagentSessionFilter): Promise<SessionListItem[]> {
    // Get all sessions
    const allSessions = await this.sessionManager.list();

    // Load full sessions to access subagent metadata
    const sessions: Session[] = [];
    for (const item of allSessions) {
      const session = await this.sessionManager.load(item.id);
      if (session) {
        sessions.push(session);
      }
    }

    // Filter subagent sessions
    let filtered = sessions.filter((s) => s.metadata.isSubagentSession === true);

    // Apply filters
    if (filter) {
      if (filter.type) {
        filtered = filtered.filter((s) => s.metadata.subagentType === filter.type);
      }

      if (filter.parentId) {
        filtered = filtered.filter((s) => s.metadata.parentSessionId === filter.parentId);
      }

      if (filter.expired !== undefined) {
        const now = new Date();
        filtered = filtered.filter((s) => {
          if (!s.metadata.expiresAt) return false;
          const expires = new Date(s.metadata.expiresAt);
          const isExpired = now > expires;
          return filter.expired ? isExpired : !isExpired;
        });
      }
    }

    // Convert to SessionListItem
    return filtered.map((s) => ({
      id: s.metadata.id,
      title: s.metadata.title,
      updatedAt: s.metadata.updatedAt,
      messageCount: s.metadata.messageCount,
      preview: s.metadata.originalDescription || 'Subagent session',
    }));
  }

  /**
   * Validate session for resume
   */
  async validateSubagentSession(sessionId: string): Promise<SessionValidationResult> {
    // Load session
    const session = await this.sessionManager.load(sessionId);
    if (!session) {
      return { valid: false, reason: 'Session not found' };
    }

    // Check if it's a subagent session
    if (!session.metadata.isSubagentSession) {
      return { valid: false, reason: 'Not a subagent session' };
    }

    // Check expiry (default: 7 days)
    if (session.metadata.expiresAt) {
      const expiresAt = new Date(session.metadata.expiresAt);
      if (Date.now() > expiresAt.getTime()) {
        return {
          valid: false,
          reason: `Session expired (>${DEFAULT_RETENTION_DAYS} days old)`,
        };
      }
    }

    // Check resume quota (max 5 resumes)
    const resumeCount = session.metadata.resumeCount || 0;
    if (resumeCount >= MAX_RESUME_COUNT) {
      return {
        valid: false,
        reason: `Resume quota exceeded (max ${MAX_RESUME_COUNT})`,
      };
    }

    return { valid: true };
  }

  /**
   * Increment resume count for a session
   */
  async incrementResumeCount(sessionId: string): Promise<void> {
    const session = await this.sessionManager.load(sessionId);
    if (!session) {
      throw new Error(`Session not found: ${sessionId}`);
    }

    session.metadata.resumeCount = (session.metadata.resumeCount || 0) + 1;
    session.metadata.lastResumedAt = new Date().toISOString();
    session.metadata.updatedAt = new Date().toISOString();

    await this.sessionManager.save(session);
  }

  /**
   * Clean up expired subagent sessions
   */
  async cleanupExpiredSessions(maxAgeDays: number = DEFAULT_RETENTION_DAYS): Promise<number> {
    // List all subagent sessions
    const sessions = await this.listSubagentSessions({ expired: true });

    // Delete expired sessions
    let cleaned = 0;
    for (const session of sessions) {
      await this.sessionManager.delete(session.id);
      cleaned++;
    }

    return cleaned;
  }

  /**
   * Generate unique session ID
   */
  private generateSessionId(): string {
    const timestamp = Date.now();
    const random = Math.random().toString(36).slice(2, 8);
    return `subagent-${timestamp}-${random}`;
  }

  /**
   * Get underlying SessionManager
   */
  getSessionManager(): SessionManager {
    return this.sessionManager;
  }
}
