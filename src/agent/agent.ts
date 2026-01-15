/**
 * Agent - Core agent implementation with tool loop and session support
 */

import type { LLMProvider, Message, ToolResultContent } from '../providers/types.js';
import { createProvider, inferProvider } from '../providers/index.js';
import { ToolRegistry, createDefaultRegistry } from '../tools/index.js';
import {
  PermissionManager,
  type ApprovalAction,
  type ApprovalSuggestion,
  type PromptPermission,
  type PermissionSettings,
} from '../permissions/index.js';
import { SessionManager } from '../session/index.js';
import type { AgentConfig, AgentEvent } from './types.js';
import { buildSystemPrompt, mapProviderToPromptType } from '../prompts/index.js';

export class Agent {
  private provider: LLMProvider;
  private registry: ToolRegistry;
  private permissions: PermissionManager;
  private sessionManager: SessionManager;
  private config: AgentConfig;
  private messages: Message[] = [];
  private sessionId: string | null = null;

  constructor(config: AgentConfig) {
    this.config = {
      maxTurns: 10,
      cwd: process.cwd(),
      ...config,
    };

    this.provider = createProvider({ provider: config.provider });
    this.registry = createDefaultRegistry();
    this.permissions = new PermissionManager({
      config: config.permissions,
      projectPath: config.cwd,
    });
    this.sessionManager = new SessionManager();
  }

  /**
   * Initialize permission system (load persisted rules)
   */
  async initializePermissions(settings?: PermissionSettings): Promise<void> {
    await this.permissions.initialize(settings);
  }

  /**
   * Set simple permission confirmation callback (backward compatible)
   */
  setConfirmCallback(callback: (tool: string, input: unknown) => Promise<boolean>): void {
    this.permissions.setSimpleConfirmCallback(callback);
  }

  /**
   * Set enhanced permission confirmation callback with approval options
   */
  setEnhancedConfirmCallback(
    callback: (
      tool: string,
      input: unknown,
      suggestions: ApprovalSuggestion[]
    ) => Promise<ApprovalAction>
  ): void {
    this.permissions.setConfirmCallback(callback);
  }

  /**
   * Add prompt-based permissions (Claude Code ExitPlanMode style)
   */
  addAllowedPrompts(prompts: PromptPermission[]): void {
    this.permissions.addAllowedPrompts(prompts);
  }

  /**
   * Clear prompt-based permissions
   */
  clearAllowedPrompts(): void {
    this.permissions.clearAllowedPrompts();
  }

  /**
   * Set callback to save permission rules to settings
   * This enables saving rules to settings.local.json instead of permissions.json
   */
  setSaveRuleCallback(callback: (tool: string, pattern?: string) => Promise<void>): void {
    this.permissions.setSaveRuleCallback(callback);
  }

  /**
   * Get permission manager for direct access
   */
  getPermissionManager(): PermissionManager {
    return this.permissions;
  }

  /**
   * Get current session ID
   */
  getSessionId(): string | null {
    return this.sessionId;
  }

  /**
   * Get session manager for external access
   */
  getSessionManager(): SessionManager {
    return this.sessionManager;
  }

  /**
   * Set the model to use (auto-switches provider if needed)
   */
  setModel(model: string): void {
    this.config.model = model;

    // Auto-switch provider based on model name
    const newProvider = inferProvider(model);
    if (newProvider !== this.config.provider) {
      this.config.provider = newProvider;
      this.provider = createProvider({ provider: newProvider });
    }
  }

  /**
   * Get current model
   */
  getModel(): string {
    return this.config.model;
  }

  /**
   * List available models from the provider API
   */
  async listModels(): Promise<{ id: string; name: string }[]> {
    return this.provider.listModels();
  }

  /**
   * Start a new session
   */
  async startSession(title?: string): Promise<string> {
    const session = await this.sessionManager.create({
      provider: this.config.provider,
      model: this.config.model,
      cwd: this.config.cwd,
      title,
    });

    this.sessionId = session.metadata.id;
    this.messages = [];

    return this.sessionId;
  }

  /**
   * Resume an existing session
   */
  async resumeSession(sessionId: string): Promise<boolean> {
    const session = await this.sessionManager.load(sessionId);
    if (!session) {
      return false;
    }

    this.sessionId = session.metadata.id;
    this.messages = session.messages;

    return true;
  }

  /**
   * Resume the most recent session
   */
  async resumeLatest(): Promise<boolean> {
    const session = await this.sessionManager.resumeLatest();
    if (!session) {
      return false;
    }

    this.sessionId = session.metadata.id;
    this.messages = session.messages;

    return true;
  }

  /**
   * Fork current session
   */
  async forkSession(title?: string): Promise<string | null> {
    if (!this.sessionId) {
      return null;
    }

    const forked = await this.sessionManager.fork(this.sessionId, title);
    this.sessionId = forked.metadata.id;

    return this.sessionId;
  }

  /**
   * List all sessions
   */
  async listSessions() {
    return this.sessionManager.list();
  }

  /**
   * Delete a session
   */
  async deleteSession(sessionId: string): Promise<boolean> {
    return this.sessionManager.delete(sessionId);
  }

  /**
   * Save current session
   */
  async saveSession(): Promise<void> {
    const current = this.sessionManager.getCurrent();
    if (current) {
      current.messages = this.messages;
      await this.sessionManager.save(current);
    }
  }

  /**
   * Run a single query through the agent
   */
  async *run(prompt: string): AsyncGenerator<AgentEvent, void, unknown> {
    // Auto-create session if none exists
    if (!this.sessionId) {
      await this.startSession();
    }

    // Add user message
    const userMessage: Message = { role: 'user', content: prompt };
    this.messages.push(userMessage);
    await this.sessionManager.addMessage(userMessage);

    let turns = 0;
    const maxTurns = this.config.maxTurns ?? 10;

    while (turns < maxTurns) {
      turns++;

      // Get tool definitions
      const toolDefs = this.registry.getDefinitions(this.config.tools);

      // Call LLM
      let response;
      try {
        // Build provider-specific system prompt if not overridden
        const systemPrompt =
          this.config.systemPrompt ??
          buildSystemPrompt(
            mapProviderToPromptType(this.config.provider),
            this.config.cwd ?? process.cwd(),
            true // Assume git repo for now
          );

        response = await this.provider.complete({
          model: this.config.model,
          messages: this.messages,
          tools: toolDefs,
          systemPrompt,
          maxTokens: 4096,
        });
      } catch (error) {
        yield { type: 'error', error: error as Error };
        return;
      }

      // Process response content
      const toolCalls: Array<{ id: string; name: string; input: Record<string, unknown> }> = [];
      let textContent = '';

      for (const content of response.content) {
        if (content.type === 'text') {
          textContent += content.text;
          yield { type: 'text', text: content.text };
        } else if (content.type === 'tool_use') {
          toolCalls.push({ id: content.id, name: content.name, input: content.input });
        }
      }

      // Add assistant message and check if done
      this.messages.push({ role: 'assistant', content: response.content });
      await this.sessionManager.addMessage({ role: 'assistant', content: response.content });

      if (response.stopReason !== 'tool_use' || toolCalls.length === 0) {
        yield { type: 'done', text: textContent };
        return;
      }

      // Execute tool calls
      const toolResults: ToolResultContent[] = [];
      const cwd = this.config.cwd ?? process.cwd();

      for (const call of toolCalls) {
        yield { type: 'tool_start', id: call.id, name: call.name, input: call.input };

        const allowed = await this.permissions.requestPermission(call.name, call.input);
        const result = allowed
          ? await this.registry.execute(call.name, call.input, { cwd })
          : { success: false, output: '', error: 'Permission denied by user' };

        yield { type: 'tool_result', id: call.id, name: call.name, result };
        toolResults.push({
          type: 'tool_result',
          toolUseId: call.id,
          content: result.success ? result.output : (result.error ?? 'Unknown error'),
          isError: !result.success,
        });
      }

      // Add tool results as user message
      this.messages.push({ role: 'user', content: toolResults });
      await this.sessionManager.addMessage({ role: 'user', content: toolResults });
    }

    yield { type: 'error', error: new Error(`Max turns (${maxTurns}) exceeded`) };
  }

  /**
   * Clear conversation history
   */
  clearHistory(): void {
    this.messages = [];
    this.sessionManager.clearMessages();
  }

  /**
   * Get conversation history
   */
  getHistory(): Message[] {
    return [...this.messages];
  }
}
