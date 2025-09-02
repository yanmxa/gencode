/**
 * Session Management Types
 */

import type { Message } from '../providers/types.js';

export interface SessionMetadata {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  provider: string;
  model: string;
  cwd: string;
  messageCount: number;
  tokenUsage?: {
    input: number;
    output: number;
  };
  parentId?: string; // For forked sessions
}

export interface Session {
  metadata: SessionMetadata;
  messages: Message[];
  systemPrompt?: string;
}

export interface SessionListItem {
  id: string;
  title: string;
  updatedAt: string;
  messageCount: number;
  preview: string; // First user message preview
}

export interface SessionConfig {
  storageDir: string;
  maxSessions: number;
  maxAge: number; // Days
  autoSave: boolean;
}

export const DEFAULT_SESSION_CONFIG: SessionConfig = {
  storageDir: '~/.mycode/sessions',
  maxSessions: 50,
  maxAge: 30,
  autoSave: true,
};
