/**
 * Agent - Core agent implementation with tool loop and session support
 */

import type { LLMProvider, Message, ToolResultContent, Provider, AuthMethod } from '../providers/types.js';
import { createProvider, inferProvider, inferAuthMethod } from '../providers/index.js';
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
import { buildSystemPromptForModel, debugPromptLoading } from '../../cli/prompts/index.js';
import type { Question, QuestionAnswer } from '../tools/types.js';
import {
  getPlanModeManager,
  type PlanModeManager,
  type ModeType,
  type AllowedPrompt,
} from '../../cli/planning/index.js';
import { initCheckpointManager } from '../session/checkpointing/index.js';
import { HooksManager } from '../../ext/hooks/index.js';
import type { HooksConfig } from '../../ext/hooks/index.js';
import { CommandManager } from '../../ext/commands/manager.js';
import type { ParsedCommand } from '../../ext/commands/types.js';

// Type for askUser callback
export type AskUserCallback = (questions: Question[]) => Promise<QuestionAnswer[]>;

export class Agent {
  private provider: LLMProvider;
  private registry: ToolRegistry | null = null; // Lazy-initialized for async skill discovery
  private registryPromise: Promise<ToolRegistry> | null = null; // Mutex for registry initialization
  private commandManager: CommandManager | null = null; // Lazy-initialized for async command discovery
  private commandManagerPromise: Promise<CommandManager> | null = null; // Mutex for command manager initialization
  private permissions: PermissionManager;
  private sessionManager: SessionManager;
  private memoryManager: MemoryManager;
  private planModeManager: PlanModeManager;
  private hooksManager: HooksManager;
  private config: AgentConfig;
  private sessionId: string | null = null;
  private loadedMemory: LoadedMemory | null = null;
  private askUserCallback: AskUserCallback | null = null;

  constructor(config: AgentConfig) {
    this.config = {
      maxTurns: 10,
      cwd: process.cwd(),
      ...config,
    };

    this.provider = createProvider({
      provider: config.provider,
      authMethod: config.authMethod,
    });
    // Registry is now initialized lazily in ensureRegistry()
    this.permissions = new PermissionManager({
      config: config.permissions,
      projectPath: config.cwd,
    });
    this.sessionManager = new SessionManager({
      compression: config.compression,
    });
    this.memoryManager = new MemoryManager();
    this.planModeManager = getPlanModeManager();
    this.hooksManager = new HooksManager();

    // Set compression engine with current model
    this.sessionManager.setCompressionEngine(this.provider, this.config.model);
  }

  /**
   * Ensure tool registry is initialized (lazy loading for async skill discovery)
   * Thread-safe: Prevents concurrent initialization using a mutex
   */
  private async ensureRegistry(): Promise<ToolRegistry> {
    // If already initialized, return immediately
    if (this.registry) {
      return this.registry;
    }

    // If initialization is in progress, wait for it
    if (this.registryPromise) {
      return this.registryPromise;
    }

    // Start new initialization
    this.registryPromise = createDefaultRegistry(this.config.cwd ?? process.cwd());

    try {
      this.registry = await this.registryPromise;
      return this.registry;
    } finally {
      // Clear the promise after initialization completes
      this.registryPromise = null;
    }
  }

  /**
   * Ensure command manager is initialized (lazy loading for async command discovery)
   * Thread-safe: Prevents concurrent initialization using a mutex
   */
  private async ensureCommandManager(): Promise<CommandManager> {
    // If already initialized, return immediately
    if (this.commandManager) {
      return this.commandManager;
    }

    // If initialization is in progress, wait for it
    if (this.commandManagerPromise) {
      return this.commandManagerPromise;
    }

    // Start new initialization
    const cwd = this.config.cwd ?? process.cwd();
    this.commandManagerPromise = (async () => {
      const manager = new CommandManager(cwd);
      await manager.initialize();
      return manager;
    })();

    try {
      this.commandManager = await this.commandManagerPromise;
      return this.commandManager;
    } finally {
      // Clear the promise after initialization completes
      this.commandManagerPromise = null;
    }
  }

  /**
   * Initialize permission system (load persisted rules)
   */
  async initializePermissions(settings?: PermissionSettings): Promise<void> {
    await this.permissions.initialize(settings);
  }

  /**
   * Initialize hooks system (load hooks configuration)
   */
  initializeHooks(hooksConfig?: HooksConfig): void {
    if (hooksConfig) {
      this.hooksManager.setConfig(hooksConfig);
    }
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

    // Determine memory merge strategy (priority: env var > config > default)
    const envStrategy = process.env.GEN_MEMORY_STRATEGY as
      | 'fallback'
      | 'both'
      | 'gen-only'
      | 'claude-only'
      | undefined;
    const strategy =
      envStrategy ?? this.config.memoryMergeStrategy ?? 'fallback';

    this.loadedMemory = await this.memoryManager.load({ cwd, strategy });

    // Log verbose summary if verbose mode is enabled
    if (this.config.verbose) {
      console.log(this.memoryManager.getVerboseSummary(strategy));
    }

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
    const { createPlanFile } = await import('../../cli/planning/index.js');
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
   * @param model Model ID to use
   * @param provider Optional: explicit provider (otherwise inferred from model name)
   * @param authMethod Optional: explicit auth method (otherwise inferred or use current)
   */
  setModel(model: string, provider?: string, authMethod?: string): void {
    this.config.model = model;

    // Determine new provider and authMethod
    const newProvider = (provider as Provider | undefined) ?? inferProvider(model);
    const newAuthMethod = (authMethod as AuthMethod | undefined) ??
                         inferAuthMethod(model) ??
                         this.config.authMethod;

    // Recreate provider if either provider or authMethod changed
    const providerChanged = newProvider !== this.config.provider;
    const authMethodChanged = newAuthMethod !== this.config.authMethod;

    if (providerChanged || authMethodChanged) {
      this.config.provider = newProvider;
      this.config.authMethod = newAuthMethod;
      this.provider = createProvider({
        provider: newProvider,
        authMethod: newAuthMethod,
      });
      // Update compression engine with new provider and model
      this.sessionManager.setCompressionEngine(this.provider, model);
    }
  }

  /**
   * Get current model
   */
  getModel(): string {
    return this.config.model;
  }

  /**
   * Get current provider
   */
  getProvider(): Provider {
    return this.config.provider;
  }

  /**
   * Get model information for compression
   */
  getModelInfo(): { contextWindow: number; outputLimit?: number } {
    // Try to get from provider if available
    if (this.provider.getModelInfo) {
      const info = this.provider.getModelInfo(this.config.model);
      if (info.contextWindow) {
        return { contextWindow: info.contextWindow, outputLimit: info.outputLimit };
      }
    }

    // Fallback: rough estimates based on model name
    // These should eventually be moved to provider implementations
    const model = this.config.model.toLowerCase();

    if (model.includes('claude')) {
      return { contextWindow: 200_000, outputLimit: 8192 };
    }

    if (model.includes('gpt-4') || model.includes('gpt-3.5')) {
      return { contextWindow: 128_000, outputLimit: 4096 };
    }

    if (model.includes('gemini')) {
      return { contextWindow: 1_000_000, outputLimit: 8192 };
    }

    // Default fallback
    return { contextWindow: 128_000, outputLimit: 4096 };
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

    // Initialize checkpoint manager for this session
    initCheckpointManager(this.sessionId);

    // Trigger SessionStart hooks
    await this.hooksManager.trigger('SessionStart', {
      event: 'SessionStart',
      cwd: this.config.cwd ?? process.cwd(),
      sessionId: this.sessionId,
      timestamp: new Date(),
    });

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

    // CheckpointManager already restored by SessionManager.load()
    // No need to call initCheckpointManager again

    // Trigger SessionStart hooks (for resumed sessions)
    await this.hooksManager.trigger('SessionStart', {
      event: 'SessionStart',
      cwd: this.config.cwd ?? process.cwd(),
      sessionId: this.sessionId,
      timestamp: new Date(),
    });

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

    // CheckpointManager already restored by SessionManager.load()
    // No need to call initCheckpointManager again

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
      await this.sessionManager.save(current);
    }
  }

  /**
   * Run a single query through the agent
   */
  async *run(prompt: string, signal?: AbortSignal): AsyncGenerator<AgentEvent, void, unknown> {
    // Check for abort before starting
    if (signal?.aborted) {
      yield { type: 'error', error: new Error('Operation cancelled') };
      return;
    }

    // Auto-create session if none exists
    try {
      if (!this.sessionId) {
        await this.startSession();
      }

      // Load memory if not already loaded
      if (!this.loadedMemory) {
        await this.loadMemory();
      }
    } catch (error) {
      yield {
        type: 'error',
        error: error instanceof Error ? error : new Error(String(error))
      };
      return;
    }

    // ============================================================================
    // COMMAND DETECTION AND EXPANSION
    // ============================================================================
    let actualPrompt = prompt;
    let parsedCommand: ParsedCommand | null = null;

    // Check if input starts with / (command syntax)
    if (prompt.trim().startsWith('/')) {
      try {
        const commandManager = await this.ensureCommandManager();

        // Parse command: /name args
        const trimmed = prompt.trim().slice(1); // Remove leading /
        const firstSpaceIndex = trimmed.indexOf(' ');
        const commandName = firstSpaceIndex === -1 ? trimmed : trimmed.slice(0, firstSpaceIndex);
        const args = firstSpaceIndex === -1 ? '' : trimmed.slice(firstSpaceIndex + 1);

        // Try to parse the command
        parsedCommand = await commandManager.parseCommand(commandName, args);

        if (parsedCommand) {
          // Replace prompt with expanded template
          actualPrompt = parsedCommand.expandedPrompt;

          // Apply pre-authorized tools (add to permission manager)
          if (parsedCommand.preAuthorizedTools.length > 0) {
            for (const toolPattern of parsedCommand.preAuthorizedTools) {
              this.permissions.addAllowedPrompts([{
                tool: 'Bash',
                prompt: toolPattern,
              }]);
            }
          }

          // Apply model override if specified
          if (parsedCommand.modelOverride) {
            this.config.model = parsedCommand.modelOverride;
            // Recreate provider with new model if needed
            // (In practice, model is just passed to API calls, no need to recreate)
          }

          // Yield event to show command was recognized
          yield {
            type: 'text',
            text: `[Command: /${commandName}${args ? ' ' + args : ''}]\n\n`,
          };
        }
        // If command not found, continue with original prompt (LLM will handle it)
      } catch (error) {
        // Command parsing failed, continue with original prompt
        console.warn('Command parsing failed:', error);
      }
    }

    // Add user message (with expanded prompt if command was parsed)
    const userMessage: Message = { role: 'user', content: actualPrompt };
    try {
      await this.sessionManager.addMessage(userMessage, this.getModelInfo());
    } catch (error) {
      yield {
        type: 'error',
        error: new Error(`Failed to save user message: ${error instanceof Error ? error.message : String(error)}`)
      };
      return;
    }

    let turns = 0;
    const maxTurns = this.config.maxTurns ?? 10;

    while (turns < maxTurns) {
      turns++;

      // Ensure tool registry is initialized (lazy loading)
      const registry = await this.ensureRegistry();

      // Get tool definitions (filtered by plan mode if active)
      const toolDefs = registry.getFilteredDefinitions(this.config.tools);

      // Call LLM
      let response;
      const processingStartTime = Date.now();
      // Determine if streaming is enabled
      const useStreaming = process.env.GEN_STREAM === '1' || this.config.streaming;

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

        if (useStreaming) {
          // === STREAMING PATH ===
          // Build response incrementally from stream chunks
          const responseBuilder = {
            content: [] as Array<{ type: 'text'; text: string } | { type: 'tool_use'; id: string; name: string; input: Record<string, unknown> }>,
            textBuffer: '',
            toolCalls: new Map<string, { id: string; name: string; inputBuffer: string }>(),
            stopReason: 'end_turn' as 'end_turn' | 'max_tokens' | 'tool_use' | 'stop_sequence',
            usage: undefined as { inputTokens: number; outputTokens: number } | undefined,
            cost: undefined as { inputCost: number; outputCost: number; totalCost: number; currency: string } | undefined,
          };

          // Process stream chunks
          for await (const chunk of this.provider.stream({
            model: this.config.model,
            messages: this.sessionManager.getMessagesForLLM(),
            tools: toolDefs,
            systemPrompt,
            maxTokens: 4096,
            signal, // Pass abort signal for cancellation
          })) {
            // Check for abort
            if (signal?.aborted) {
              yield { type: 'error', error: new Error('Operation cancelled by user') };
              return;
            }
            switch (chunk.type) {
              case 'text':
                // Accumulate text and yield immediately for real-time display
                responseBuilder.textBuffer += chunk.text;
                yield { type: 'text', text: chunk.text };
                break;

              case 'reasoning':
                // Forward reasoning content (o1/o3/Gemini 3+ thinking)
                yield { type: 'reasoning_delta', text: chunk.text };
                break;

              case 'tool_start':
                // Initialize tool call tracking
                responseBuilder.toolCalls.set(chunk.id, {
                  id: chunk.id,
                  name: chunk.name,
                  inputBuffer: '',
                });
                break;

              case 'tool_input':
                // Accumulate incremental JSON input and forward delta
                const tool = responseBuilder.toolCalls.get(chunk.id);
                if (tool) {
                  tool.inputBuffer += chunk.input;
                  // Emit incremental tool input for progressive display
                  yield { type: 'tool_input_delta', id: chunk.id, delta: chunk.input };
                }
                break;

              case 'done':
                // Save final metadata
                responseBuilder.stopReason = chunk.response.stopReason;
                responseBuilder.usage = chunk.response.usage;
                responseBuilder.cost = chunk.response.cost;
                break;

              case 'error':
                yield { type: 'error', error: chunk.error };
                return;
            }
          }

          // Build complete response from accumulated data
          if (responseBuilder.textBuffer) {
            responseBuilder.content.push({
              type: 'text',
              text: responseBuilder.textBuffer,
            });
          }

          for (const [_id, tool] of responseBuilder.toolCalls) {
            try {
              responseBuilder.content.push({
                type: 'tool_use',
                id: tool.id,
                name: tool.name,
                input: JSON.parse(tool.inputBuffer || '{}'),
              });
            } catch (error) {
              // If JSON parsing fails, treat as malformed tool call
              yield {
                type: 'error',
                error: new Error(`Failed to parse tool input for ${tool.name}: ${error instanceof Error ? error.message : String(error)}`),
              };
              return;
            }
          }

          response = {
            content: responseBuilder.content,
            stopReason: responseBuilder.stopReason,
            usage: responseBuilder.usage,
            cost: responseBuilder.cost,
          };

        } else {
          // === TRADITIONAL PATH (COMPLETE) ===
          response = await this.provider.complete({
            model: this.config.model,
            messages: this.sessionManager.getMessagesForLLM(),
            tools: toolDefs,
            systemPrompt,
            maxTokens: 4096,
          });
        }
      } catch (error) {
        yield { type: 'error', error: error as Error };
        return;
      }

      // Validate response completeness
      if (!response || !response.content) {
        yield {
          type: 'error',
          error: new Error('Provider returned null or undefined response')
        };
        return;
      }

      // Validate content is not empty (excluding max_tokens case)
      if (response.content.length === 0 && response.stopReason !== 'max_tokens') {
        yield {
          type: 'error',
          error: new Error(
            `Provider returned empty content (stopReason: ${response.stopReason}, ` +
            `usage: ${JSON.stringify(response.usage)})`
          )
        };
        return;
      }

      // Process response content
      const toolCalls: Array<{ id: string; name: string; input: Record<string, unknown> }> = [];
      let textContent = '';

      for (const content of response.content) {
        if (content.type === 'text') {
          textContent += content.text;
          // Only yield text if not in streaming mode (streaming already yielded chunks)
          if (!useStreaming) {
            yield { type: 'text', text: content.text };
          }
        } else if (content.type === 'tool_use') {
          toolCalls.push({ id: content.id, name: content.name, input: content.input });
        }
      }

      // Add assistant message and check if done
      try {
        await this.sessionManager.addMessage(
          { role: 'assistant', content: response.content },
          this.getModelInfo()
        );
      } catch (error) {
        yield {
          type: 'error',
          error: new Error(`Failed to save assistant message: ${error instanceof Error ? error.message : String(error)}`)
        };
        return;
      }

      if (response.stopReason !== 'tool_use' || toolCalls.length === 0) {
        yield { type: 'done', text: textContent, usage: response.usage, cost: response.cost };

        // Save completion metadata for UI restoration
        if (response.usage || response.cost) {
          const current = this.sessionManager.getCurrent();
          if (current) {
            if (!current.metadata.completions) {
              current.metadata.completions = [];
            }
            current.metadata.completions.push({
              afterMessageIndex: current.messages.length - 1,
              durationMs: Date.now() - processingStartTime,
              usage: response.usage ? {
                inputTokens: response.usage.inputTokens,
                outputTokens: response.usage.outputTokens,
              } : undefined,
              cost: response.cost,
            });
          }
        }

        // Trigger Stop hooks when conversation ends
        await this.hooksManager.trigger('Stop', {
          event: 'Stop',
          cwd: this.config.cwd ?? process.cwd(),
          sessionId: this.sessionId ?? undefined,
          timestamp: new Date(),
        });

        return;
      }

      // Execute tool calls
      const toolResults: ToolResultContent[] = [];
      const cwd = this.config.cwd ?? process.cwd();

      // Build tool context with askUser callback and current agent info
      const toolContext = {
        cwd,
        askUser: this.askUserCallback ?? undefined,
        currentProvider: this.config.provider,
        currentModel: this.config.model,
        currentAuthMethod: this.config.authMethod,
      };

      for (const call of toolCalls) {
        yield { type: 'tool_start', id: call.id, name: call.name, input: call.input };

        // Trigger PreToolUse hooks
        const preHookResults = await this.hooksManager.trigger('PreToolUse', {
          event: 'PreToolUse',
          cwd,
          toolName: call.name,
          toolInput: call.input as Record<string, unknown>,
          sessionId: this.sessionId ?? undefined,
          timestamp: new Date(),
        });

        // Check if any PreToolUse hook blocked the action
        const preHookBlocked = preHookResults.some(r => r.blocked);
        if (preHookBlocked) {
          const blockingHook = preHookResults.find(r => r.blocked);
          const result = {
            success: false,
            output: '',
            error: `Blocked by PreToolUse hook: ${blockingHook?.error || 'Hook returned exit code 2'}`,
          };
          yield { type: 'tool_result', id: call.id, name: call.name, result };
          toolResults.push({
            type: 'tool_result',
            toolUseId: call.id,
            content: result.error,
            isError: true,
          });
          continue; // Skip to next tool
        }

        try {
          // Ensure tool registry is initialized
          const registry = await this.ensureRegistry();

          // Protect permission check and tool execution
          const allowed = await this.permissions.requestPermission(call.name, call.input);
          const result = allowed
            ? await registry.execute(call.name, call.input, toolContext)
            : { success: false, output: '', error: 'Permission denied by user' };

          yield { type: 'tool_result', id: call.id, name: call.name, result };
          toolResults.push({
            type: 'tool_result',
            toolUseId: call.id,
            content: result.success ? result.output : (result.error ?? 'Unknown error'),
            isError: !result.success,
          });

          // Trigger PostToolUse or PostToolUseFailure hooks
          const hookEvent = result.success ? 'PostToolUse' : 'PostToolUseFailure';
          await this.hooksManager.trigger(hookEvent, {
            event: hookEvent,
            cwd,
            toolName: call.name,
            toolInput: call.input as Record<string, unknown>,
            toolResult: result,
            sessionId: this.sessionId ?? undefined,
            timestamp: new Date(),
          });
        } catch (error) {
          // Catch permission check or tool execution errors
          const errorMsg = error instanceof Error ? error.message : String(error);
          const errorResult = {
            success: false,
            output: '',
            error: `Tool execution error: ${errorMsg}`
          };
          yield { type: 'tool_result', id: call.id, name: call.name, result: errorResult };
          toolResults.push({
            type: 'tool_result',
            toolUseId: call.id,
            content: errorMsg,
            isError: true,
          });

          // Trigger PostToolUseFailure hooks
          await this.hooksManager.trigger('PostToolUseFailure', {
            event: 'PostToolUseFailure',
            cwd,
            toolName: call.name,
            toolInput: call.input as Record<string, unknown>,
            toolResult: errorResult,
            sessionId: this.sessionId ?? undefined,
            timestamp: new Date(),
          });
        }
      }

      // Add tool results as user message
      try {
        await this.sessionManager.addMessage(
          { role: 'user', content: toolResults },
          this.getModelInfo()
        );
      } catch (error) {
        yield {
          type: 'error',
          error: new Error(`Failed to save tool results: ${error instanceof Error ? error.message : String(error)}`)
        };
        return;
      }
    }

    yield { type: 'error', error: new Error(`Max turns (${maxTurns}) exceeded`) };
  }

  /**
   * Clear conversation history
   */
  clearHistory(): void {
    this.sessionManager.clearMessages();
  }

  /**
   * Clean up incomplete tool use messages (after interruption)
   * Removes the last assistant message if it contains tool_use without corresponding tool_result
   */
  cleanupIncompleteMessages(): void {
    const messages = this.sessionManager.getMessages();
    if (messages.length === 0) return;

    const lastMessage = messages[messages.length - 1];

    // Check if last message is an assistant message with tool_use
    if (lastMessage.role === 'assistant' && Array.isArray(lastMessage.content)) {
      const hasToolUse = lastMessage.content.some((c) => c.type === 'tool_use');

      if (hasToolUse) {
        // Remove the incomplete assistant message from session manager
        this.sessionManager.removeLastMessage();
      }
    }
  }

  /**
   * Get conversation history
   */
  getHistory(): Message[] {
    return this.sessionManager.getMessages();
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
    return this.sessionManager.getCompressionStats();
  }
}
