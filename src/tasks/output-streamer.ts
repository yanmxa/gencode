/**
 * OutputStreamer - Stream agent events to NDJSON log file
 *
 * Responsibilities:
 * - Write agent events to NDJSON (newline-delimited JSON) format
 * - Update task metadata file
 * - Read events back from log
 * - Enforce output size limits
 */

import * as fs from 'node:fs/promises';
import { createWriteStream, type WriteStream } from 'node:fs';
import { createReadStream } from 'node:fs';
import { createInterface } from 'node:readline';
import type { AgentEvent } from '../agent/types.js';
import type { BackgroundTaskJson } from './types.js';

/**
 * Agent event log entry for NDJSON storage
 */
export interface AgentEventLog {
  timestamp: string;
  type: AgentEvent['type'];
  data: unknown;
}

/**
 * Maximum output file size (10MB)
 */
const MAX_OUTPUT_SIZE = 10 * 1024 * 1024;

/**
 * OutputStreamer - Manages event streaming and metadata updates
 */
export class OutputStreamer {
  private outputStream: WriteStream | null = null;
  private outputPath: string;
  private metadataPath: string;
  private bytesWritten: number = 0;
  private closed: boolean = false;

  constructor(outputPath: string, metadataPath: string) {
    this.outputPath = outputPath;
    this.metadataPath = metadataPath;
  }

  /**
   * Write an agent event to the NDJSON log
   */
  async writeEvent(event: AgentEvent): Promise<void> {
    if (this.closed) {
      throw new Error('OutputStreamer is closed');
    }

    // Check size limit
    if (this.bytesWritten >= MAX_OUTPUT_SIZE) {
      await this.writeEvent({
        type: 'error',
        error: new Error('Output size limit exceeded (10MB)'),
      });
      await this.close();
      return;
    }

    // Lazy initialize stream
    if (!this.outputStream) {
      this.outputStream = createWriteStream(this.outputPath, { flags: 'a' });
    }

    // Create event log entry
    const logEntry: AgentEventLog = {
      timestamp: new Date().toISOString(),
      type: event.type,
      data: this.serializeEventData(event),
    };

    // Write as NDJSON (newline-delimited JSON)
    const json = JSON.stringify(logEntry);
    const line = json + '\n';

    this.outputStream.write(line);
    this.bytesWritten += Buffer.byteLength(line, 'utf-8');
  }

  /**
   * Serialize event data for logging
   */
  private serializeEventData(event: AgentEvent): unknown {
    switch (event.type) {
      case 'text':
        return { text: event.text };

      case 'thinking':
        return { text: event.text };

      case 'reasoning_delta':
        return { text: event.text };

      case 'tool_start':
        return {
          id: event.id,
          name: event.name,
          input: event.input,
        };

      case 'tool_result':
        return {
          id: event.id,
          name: event.name,
          result: event.result,
        };

      case 'tool_input_delta':
        return {
          id: event.id,
          delta: event.delta,
        };

      case 'done':
        return {
          text: event.text,
          usage: event.usage
            ? {
                inputTokens: event.usage.inputTokens,
                outputTokens: event.usage.outputTokens,
              }
            : undefined,
          cost: event.cost,
        };

      case 'error':
        return {
          message: event.error.message,
          stack: event.error.stack,
        };

      case 'ask_user':
        return {
          id: event.id,
          questions: event.questions,
        };

      default:
        return {};
    }
  }

  /**
   * Update task metadata file
   */
  async updateMetadata(updates: Partial<BackgroundTaskJson>): Promise<void> {
    if (this.closed) return;

    try {
      // Read current metadata
      let current: BackgroundTaskJson;
      try {
        const data = await fs.readFile(this.metadataPath, 'utf-8');
        current = JSON.parse(data);
      } catch {
        // Metadata doesn't exist yet, skip update
        return;
      }

      // Merge updates
      const updated: BackgroundTaskJson = {
        ...current,
        ...updates,
      };

      // Write back
      await fs.writeFile(this.metadataPath, JSON.stringify(updated, null, 2), 'utf-8');
    } catch (error) {
      // Ignore metadata update errors to prevent blocking execution
      console.error('Failed to update task metadata:', error);
    }
  }

  /**
   * Close the output stream
   */
  async close(): Promise<void> {
    if (this.closed) return;

    this.closed = true;

    if (this.outputStream) {
      return new Promise<void>((resolve, reject) => {
        this.outputStream!.end((err: Error | undefined) => {
          if (err) reject(err);
          else resolve();
        });
      });
    }
  }

  /**
   * Read all events from a log file
   */
  static async *readEvents(logPath: string): AsyncGenerator<AgentEventLog> {
    const fileStream = createReadStream(logPath, { encoding: 'utf-8' });
    const rl = createInterface({
      input: fileStream,
      crlfDelay: Infinity,
    });

    for await (const line of rl) {
      if (line.trim().length === 0) continue;

      try {
        const event: AgentEventLog = JSON.parse(line);
        yield event;
      } catch (error) {
        // Skip malformed lines
        console.error('Failed to parse event log line:', error);
      }
    }
  }

  /**
   * Get current task status from metadata file
   */
  static async getStatus(metadataPath: string): Promise<BackgroundTaskJson | null> {
    try {
      const data = await fs.readFile(metadataPath, 'utf-8');
      return JSON.parse(data);
    } catch {
      return null;
    }
  }

  /**
   * Get number of bytes written
   */
  getBytesWritten(): number {
    return this.bytesWritten;
  }

  /**
   * Check if stream is closed
   */
  isClosed(): boolean {
    return this.closed;
  }
}
