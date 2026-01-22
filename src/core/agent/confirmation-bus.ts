/**
 * ConfirmationBus - EventEmitter-based confirmation system
 *
 * This module implements the Gemini CLI approach to solving the Ink rendering deadlock.
 * The problem: When awaiting a callback inside an async generator, Ink's render cycle
 * is blocked because the `for await` loop in the consumer is waiting for the next yield.
 *
 * Solution: Use EventEmitter to completely decouple tool execution from UI:
 * 1. Agent EMITS confirmation request event (non-blocking, returns immediately)
 * 2. Agent AWAITS using Node.js `on()` function (yields to event loop!)
 * 3. UI SUBSCRIBES to events and shows dialog
 * 4. UI EMITS response event
 * 5. Agent's `on()` listener receives response and continues
 *
 * The key insight is that `on()` from 'node:events' returns an async iterator
 * that yields to the event loop between events, unlike a Promise which blocks.
 */

import { EventEmitter, on } from 'node:events';
import type { ApprovalAction } from '../permissions/types.js';
import type { PermissionRequest } from '../tools/types.js';

export interface ConfirmationRequestEvent {
  id: string;
  request: PermissionRequest;
}

export interface ConfirmationResponseEvent {
  id: string;
  action: ApprovalAction;
}

/**
 * Singleton EventEmitter for confirmation requests/responses.
 * Used to decouple the agent's tool execution from the UI's render cycle.
 */
export class ConfirmationBus extends EventEmitter {
  private static instance: ConfirmationBus | null = null;

  private constructor() {
    super();
    // Increase max listeners since we might have many concurrent requests
    this.setMaxListeners(100);
  }

  /**
   * Get the singleton instance of ConfirmationBus.
   */
  static getInstance(): ConfirmationBus {
    if (!ConfirmationBus.instance) {
      ConfirmationBus.instance = new ConfirmationBus();
    }
    return ConfirmationBus.instance;
  }

  /**
   * Reset the singleton (useful for testing).
   */
  static reset(): void {
    if (ConfirmationBus.instance) {
      ConfirmationBus.instance.removeAllListeners();
      ConfirmationBus.instance = null;
    }
  }

  /**
   * Agent calls this to request confirmation.
   * This is non-blocking - it emits an event and returns immediately.
   *
   * @param id Unique request ID
   * @param request The permission request details
   */
  requestConfirmation(id: string, request: PermissionRequest): void {
    const event: ConfirmationRequestEvent = { id, request };
    this.emit('confirmation_request', event);
  }

  /**
   * Agent calls this to wait for a response.
   * Uses Node.js `on()` which returns an async iterator that yields to the event loop.
   * This is the key to avoiding the Ink rendering deadlock!
   *
   * @param id Request ID to wait for
   * @param signal Optional AbortSignal for cancellation
   * @returns The user's approval action
   */
  async waitForResponse(id: string, signal?: AbortSignal): Promise<ApprovalAction> {
    const ac = new AbortController();

    // Link external signal to our controller
    if (signal) {
      if (signal.aborted) {
        return 'deny';
      }
      signal.addEventListener('abort', () => ac.abort(), { once: true });
    }

    try {
      // `on()` returns an async iterator that yields to the event loop
      // This is fundamentally different from a Promise - it allows other
      // async operations (like Ink's render cycle) to run between events
      for await (const [event] of on(this, 'confirmation_response', { signal: ac.signal })) {
        const response = event as ConfirmationResponseEvent;
        if (response.id === id) {
          return response.action;
        }
        // Keep waiting for our specific ID
      }
    } catch (err) {
      if ((err as Error).name === 'AbortError') {
        return 'deny';
      }
      throw err;
    }

    // Should not reach here, but return deny as fallback
    return 'deny';
  }

  /**
   * UI calls this to send a response to a confirmation request.
   * This emits an event that will be received by waitForResponse().
   *
   * @param id Request ID
   * @param action User's approval action
   */
  respondToConfirmation(id: string, action: ApprovalAction): void {
    const event: ConfirmationResponseEvent = { id, action };
    this.emit('confirmation_response', event);
  }
}
