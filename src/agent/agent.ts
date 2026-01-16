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
import { MemoryManager, type LoadedMemory } from '../memory/index.js';
import type { AgentConfig, AgentEvent } from './types.js';
import { buildSystemPromptForModel, debugPromptLoading } from '../prompts/index.js';
import type { Question, QuestionAnswer } from '../tools/types.js';
import {
  getPlanModeManager,
  type PlanModeManager,
  type ModeType,
  type AllowedPrompt,
} from '../planning/index.js';

// Type for askUser callback
export type AskUserCallback = (questions: Question[]) => Promise<QuestionAnswer[]>;

export class Agent {
  private provider: LLMProvider;
  private registry: ToolRegistry;
  private permissions: PermissionManager;
  private sessionManager: SessionManager;
  private memoryManager: MemoryManager;
  private planModeManager: PlanModeManager;
  private config: AgentConfig;
  private messages: Message[] = [];
  private sessionId: string | null = null;
  private loadedMemory: LoadedMemory | null = null;
  private askUserCallback: AskUserCallback | null = null;

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
    this.memoryManager = new MemoryManager();
    this.planModeManager = getPlanModeManager();
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
   * Set callback for AskUserQuestion tool
   * This allows the CLI to handle user questioning
   */
  setAskUserCallback(callback: AskUserCallback): void {
    this.askUserCallback = callback;
  }

  /**
   * Get memory manager for direct access
   */
  getMemoryManager(): MemoryManager {
    return this.memoryManager;
  }

  /**
   * Get loaded memory (null if not loaded yet)
   */
  getLoadedMemory(): LoadedMemory | null {
    return this.loadedMemory;
  }

  /**
   * Load memory for the current working directory
   */
  async loadMemory(): Promise<LoadedMemory> {
    const cwd = this.config.cwd ?? process.cwd();
    this.loadedMemory = await this.memoryManager.load({ cwd });
    return this.loadedMemory;
  }

  // ============================================================================
  // Plan Mode
  // ============================================================================

  /**
   * Get plan mode manager for external access
   */
  getPlanModeManager(): PlanModeManager {
    return this.planModeManager;
  }

  /**
   * Check if plan mode is active
   */
  isPlanModeActive(): boolean {
    return this.planModeManager.isActive();
  }

  /**
   * Get current mode (build or plan)
   */
  getCurrentMode(): ModeType {
    return this.planModeManager.getCurrentMode();
  }

  /**
   * Enter plan mode programmatically
   */
  async enterPlanMode(taskDescription?: string): Promise<string> {
    const { createPlanFile } = await import('../planning/index.js');
    const cwd = this.config.cwd ?? process.cwd();
    const planFile = await createPlanFile(cwd, taskDescription);
    this.planModeManager.enter(planFile.path, taskDescription);
    return planFile.path;
  }

  /**
   * Exit plan mode
   */
  exitPlanMode(approved: boolean = false): void {
    if (approved) {
      // Add allowed prompts from plan mode to permissions
      const allowedPrompts = this.planModeManager.getRequestedPermissions();
      if (allowedPrompts.length > 0) {
        this.permissions.addAllowedPrompts(allowedPrompts);
      }
    }
    this.planModeManager.exit(approved);
  }

  /**
   * Toggle plan mode
   */
  async togglePlanMode(): Promise<void> {
    if (this.planModeManager.isActive()) {
      this.planModeManager.exit(false);
    } else {
      await this.enterPlanMode();
    }
  }

  /**
   * Get plan mode requested permissions
   */
  getPlanModePermissions(): AllowedPrompt[] {
    return this.planModeManager.getRequestedPermissions();
  }

  // ============================================================================
  // Session Management
  // ============================================================================

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

    // Load memory if not already loaded
    if (!this.loadedMemory) {
      await this.loadMemory();
    }

    // Add user message
    const userMessage: Message = { role: 'user', content: prompt };
    this.messages.push(userMessage);
    await this.sessionManager.addMessage(userMessage);

    let turns = 0;
    const maxTurns = this.config.maxTurns ?? 10;

    while (turns < maxTurns) {
      turns++;

      // Get tool definitions (filtered by plan mode if active)
      const toolDefs = this.registry.getFilteredDefinitions(this.config.tools);

      // Call LLM
      let response;
      try {
        // Debug prompt loading (enabled with GENCODE_DEBUG_PROMPTS=1)
        debugPromptLoading(this.config.model, this.config.provider);

        // Build system prompt based on model → provider → prompt flow
        // Looks up provider from ~/.gencode/providers.json, falls back to config.provider
        const systemPrompt =
          this.config.systemPrompt ??
          buildSystemPromptForModel(
            this.config.model,
            this.config.cwd ?? process.cwd(),
            true, // Assume git repo for now
            this.loadedMemory?.context,
            this.config.provider // Fallback provider if model lookup fails
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
        yield { type: 'done', text: textContent, usage: response.usage, cost: response.cost };
        return;
      }

      // Execute tool calls
      const toolResults: ToolResultContent[] = [];
      const cwd = this.config.cwd ?? process.cwd();

      // Build tool context with askUser callback
      const toolContext = {
        cwd,
        askUser: this.askUserCallback ?? undefined,
      };

      for (const call of toolCalls) {
        yield { type: 'tool_start', id: call.id, name: call.name, input: call.input };

        const allowed = await this.permissions.requestPermission(call.name, call.input);
        const result = allowed
          ? await this.registry.execute(call.name, call.input, toolContext)
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
   * Clean up incomplete tool use messages (after interruption)
   * Removes the last assistant message if it contains tool_use without corresponding tool_result
   */
  cleanupIncompleteMessages(): void {
    if (this.messages.length === 0) return;

    const lastMessage = this.messages[this.messages.length - 1];

    // Check if last message is an assistant message with tool_use
    if (lastMessage.role === 'assistant' && Array.isArray(lastMessage.content)) {
      const hasToolUse = lastMessage.content.some((c) => c.type === 'tool_use');

      if (hasToolUse) {
        // Remove the incomplete assistant message
        this.messages.pop();

        // Also remove from session manager
        // Note: SessionManager should have corresponding cleanup method
        const messages = this.sessionManager.getMessages();
        if (messages.length > 0 && messages[messages.length - 1].role === 'assistant') {
          this.sessionManager.removeLastMessage();
        }
      }
    }
  }

  /**
   * Get conversation history
   */
  getHistory(): Message[] {
    return [...this.messages];
  }
}
