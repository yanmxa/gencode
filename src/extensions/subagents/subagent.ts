/**
 * Subagent - Isolated agent instance for specialized tasks
 *
 * Uses composition (wraps a new Agent instance) rather than inheritance.
 * Each subagent gets:
 * - Filtered tool access based on type
 * - Specialized system prompt
 * - Isolated conversation context
 */

import { Agent } from '../../core/agent/agent.js';
import type { AgentConfig } from '../../core/agent/types.js';
import type { Provider, AuthMethod } from '../../core/providers/types.js';
import type { Message } from '../../core/providers/types.js';
import { inferProvider, inferAuthMethod } from '../../core/providers/index.js';
import { SUBAGENT_CONFIGS } from './configs.js';
import type { SubagentType, SubagentConfig, TaskOutput } from './types.js';
import { SubagentSessionManager } from './subagent-session-manager.js';
import { isVerboseDebugEnabled } from '../../infrastructure/utils/debug.js';
import { logger } from '../../infrastructure/utils/logger.js';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

export interface SubagentOptions {
  /** Subagent type */
  type: SubagentType;

  /** Optional: override specific config fields */
  config?: Partial<SubagentConfig>;

  /** Optional: provider (defaults to inferred from model or anthropic) */
  provider?: Provider;

  /** Optional: authentication method (defaults to inferred from model) */
  authMethod?: AuthMethod;

  /** Optional: model override */
  model?: string;

  /** Optional: working directory */
  cwd?: string;

  /** Optional: persist session for resume capability (Phase 3) */
  persistSession?: boolean;

  /** Optional: parent session ID (for linking subagent to parent) */
  parentSessionId?: string;

  /** Optional: task description (for session title) */
  description?: string;

  /** Optional: current depth in subagent chain (Phase 4) */
  currentDepth?: number;

  /** Optional: maximum depth allowed (Phase 4, default: 3) */
  maxDepth?: number;

  /** Optional: parent subagent ID (for tracking lineage) */
  parentSubagentId?: string;

  /** Optional: parent agent's model (for inheriting credentials) */
  parentModel?: string;
}

/**
 * Maximum subagent depth for inter-agent communication (Phase 4)
 */
const DEFAULT_MAX_DEPTH = 3;

/**
 * Subagent - Wrapper around Agent with specialized configuration
 */
export class Subagent {
  private id: string;
  private type: SubagentType;
  private config: SubagentConfig;
  private agent: Agent;
  private status: 'idle' | 'running' | 'completed' | 'error' = 'idle';
  private result: string | null = null;
  private error: string | null = null;
  private sessionManager: SubagentSessionManager;
  private sessionId: string | null = null;
  private options: SubagentOptions;
  private currentDepth: number;
  private maxDepth: number;

  constructor(options: SubagentOptions) {
    this.id = this.generateId();
    this.type = options.type;
    this.options = options;
    this.sessionManager = new SubagentSessionManager();
    this.currentDepth = options.currentDepth ?? 0;
    this.maxDepth = options.maxDepth ?? DEFAULT_MAX_DEPTH;

    // Merge base config with overrides
    const baseConfig = SUBAGENT_CONFIGS[options.type];
    this.config = { ...baseConfig, ...options.config };

    // Determine the model to use
    // Priority 1: Explicit options.model
    // Priority 2: Parent model (inherit from parent agent)
    // Priority 3: Config default model
    let targetModel = options.model ?? options.parentModel ?? this.config.defaultModel;

    // Determine provider and authMethod
    // If provided explicitly (from parent agent), use them directly
    // Otherwise infer from the model
    let provider: Provider = options.provider ?? inferProvider(targetModel);
    let authMethod: AuthMethod | undefined = options.authMethod ?? inferAuthMethod(targetModel);

    // Verbose debug: Log credential inheritance
    if (isVerboseDebugEnabled('subagents')) {
      logger.debug('Subagent', 'Subagent credentials', {
        type: this.type,
        model: targetModel,
        provider,
        authMethod,
        inheritedFromParent: !!(options.provider || options.parentModel),
        explicitModel: !!options.model,
      });
    }

    // Build Agent configuration
    const agentConfig: AgentConfig = {
      provider,
      authMethod,
      model: targetModel,
      cwd: options.cwd ?? process.cwd(),
      systemPrompt: this.config.systemPrompt,
      maxTurns: this.config.maxTurns,
      tools: this.config.allowedTools,
      // Tool access is restricted via allowedTools filtering
    };

    // Create isolated Agent instance
    this.agent = new Agent(agentConfig);
  }

  /**
   * Execute the subagent task (foreground execution)
   * Returns a summary of findings, not full conversation
   */
  async run(prompt: string, signal?: AbortSignal): Promise<TaskOutput> {
    // Verbose debug: Log subagent execution start
    if (isVerboseDebugEnabled('subagents')) {
      logger.debug('Subagent', `Starting subagent execution`, {
        type: this.type,
        id: this.id,
        model: this.config.defaultModel,
        maxTurns: this.config.maxTurns,
        allowedTools: this.config.allowedTools.join(', '),
        currentDepth: this.currentDepth,
        maxDepth: this.maxDepth,
      });
    }

    // Check depth limit (Phase 4: inter-agent communication)
    if (this.currentDepth >= this.maxDepth) {
      if (isVerboseDebugEnabled('subagents')) {
        logger.debug('Subagent', `Maximum depth exceeded`, {
          currentDepth: this.currentDepth,
          maxDepth: this.maxDepth,
        });
      }
      return {
        success: false,
        agentId: this.id,
        error: `Maximum subagent depth (${this.maxDepth}) exceeded. Cannot spawn more nested subagents.`,
      };
    }

    this.status = 'running';
    const startTime = Date.now();

    try {
      // Create isolated session for this subagent
      // Session name includes type for debugging
      const sessionName = `Subagent-${this.type}-${this.id.split('-').pop()}`;

      // Note: We don't call agent.startSession() explicitly because
      // the agent.run() method auto-creates a session if none exists

      let fullOutput = '';
      let turns = 0;
      let finalTokenUsage: { input: number; output: number } | undefined;

      // Run agent loop and collect events
      for await (const event of this.agent.run(prompt, signal)) {
        switch (event.type) {
          case 'text':
            fullOutput += event.text;
            break;

          case 'done':
            turns++;

            // Verbose debug: Log turn completion
            if (isVerboseDebugEnabled('subagents')) {
              logger.debug('Subagent', `Turn ${turns}/${this.config.maxTurns} completed`, {
                type: this.type,
                id: this.id,
              });
            }

            if (event.usage) {
              finalTokenUsage = {
                input: event.usage.inputTokens,
                output: event.usage.outputTokens,
              };
            }
            break;

          case 'error':
            this.status = 'error';
            this.error = event.error.message;

            if (isVerboseDebugEnabled('subagents')) {
              logger.debug('Subagent', `Execution error`, {
                type: this.type,
                id: this.id,
                error: event.error.message,
              });
            }

            return {
              success: false,
              agentId: this.id,
              error: event.error.message,
            };
        }
      }

      // Generate summary from conversation
      const summary = await this.generateSummary();

      this.status = 'completed';
      this.result = summary;

      // Save session if persistence enabled (Phase 3)
      if (this.options.persistSession) {
        if (!this.sessionId) {
          // Create new session
          const description = this.options.description || `${this.type} task`;
          this.sessionId = await this.sessionManager.createSubagentSession(
            this.type,
            description,
            this.options.parentSessionId
          );
        }

        // Save session state
        await this.sessionManager.saveSubagentSession(this.sessionId, this.agent);
      }

      const resultMetadata: TaskOutput['metadata'] = {
        subagentType: this.type,
        model: this.config.defaultModel,
        turns,
        durationMs: Date.now() - startTime,
        tokenUsage: finalTokenUsage,
      };

      // Add sessionId to metadata if persisted
      if (this.sessionId) {
        (resultMetadata as { sessionId?: string }).sessionId = this.sessionId;
      }

      // Verbose debug: Log subagent execution complete
      if (isVerboseDebugEnabled('subagents')) {
        logger.debug('Subagent', `Subagent execution complete`, {
          type: this.type,
          id: this.id,
          turns,
          durationMs: Date.now() - startTime,
          resultLength: summary.length,
          sessionPersisted: !!this.sessionId,
        });
      }

      return {
        success: true,
        agentId: this.id,
        result: summary,
        metadata: resultMetadata,
      };
    } catch (error) {
      this.status = 'error';
      this.error = error instanceof Error ? error.message : String(error);

      // Verbose debug: Log execution exception
      if (isVerboseDebugEnabled('subagents')) {
        logger.debug('Subagent', `Execution exception`, {
          type: this.type,
          id: this.id,
          error: this.error,
        });
      }

      // Save error state if persistence enabled
      if (this.options.persistSession && this.sessionId) {
        await this.sessionManager.saveSubagentSession(this.sessionId, this.agent);
      }

      return {
        success: false,
        agentId: this.id,
        error: this.error,
      };
    }
  }

  /**
   * Generate concise summary from subagent conversation
   * Only the summary is returned to the main agent (not full history)
   */
  private async generateSummary(): Promise<string> {
    const messages = this.agent.getHistory();

    if (messages.length === 0) {
      return 'No output generated';
    }

    // Get summary configuration with defaults
    const summaryConfig = this.config.summaryConfig ?? this.getDefaultSummaryConfig();
    const maxLength = summaryConfig.maxLength;
    const truncationStrategy = summaryConfig.truncationStrategy ?? 'simple';

    // For Explore and Plan subagents: return last assistant message
    if (this.type === 'Explore' || this.type === 'Plan') {
      const assistantMessages = messages.filter((m) => m.role === 'assistant');
      const lastMsg = assistantMessages[assistantMessages.length - 1];

      if (lastMsg && typeof lastMsg.content === 'string') {
        return this.truncateText(lastMsg.content, maxLength, truncationStrategy);
      }
    }

    // For Bash and general-purpose: concatenate all assistant messages
    const assistantTexts = messages
      .filter((m) => m.role === 'assistant')
      .map((m) => {
        if (typeof m.content === 'string') {
          return m.content;
        }
        // Handle tool use messages
        if (Array.isArray(m.content)) {
          return m.content
            .filter((block) => block.type === 'text')
            .map((block) => (block as { type: 'text'; text: string }).text)
            .join('\n');
        }
        return '';
      })
      .filter((text) => text.length > 0);

    const combined = assistantTexts.join('\n\n');

    return this.truncateText(combined, maxLength, truncationStrategy);
  }

  /**
   * Get default summary configuration based on subagent type
   */
  private getDefaultSummaryConfig(): import('./types.js').SummaryConfig {
    switch (this.type) {
      case 'Explore':
        return { maxLength: 1000, truncationStrategy: 'simple' };
      case 'Plan':
        return { maxLength: 2000, truncationStrategy: 'simple' };
      default:
        return { maxLength: 2000, truncationStrategy: 'simple' };
    }
  }

  /**
   * Truncate text to maximum length using specified strategy
   */
  private truncateText(text: string, maxLength: number, strategy: 'simple' | 'smart'): string {
    if (text.length <= maxLength) {
      return text;
    }

    if (strategy === 'simple') {
      return text.slice(0, maxLength);
    }

    // Smart truncation: try to break at sentence boundary
    const truncated = text.slice(0, maxLength);
    const lastPeriod = truncated.lastIndexOf('.');
    const lastNewline = truncated.lastIndexOf('\n');
    const breakPoint = Math.max(lastPeriod, lastNewline);

    if (breakPoint > maxLength * 0.7) {
      // If we can break at a good point (not too early), use it
      return truncated.slice(0, breakPoint + 1);
    }

    // Otherwise fall back to simple truncation
    return truncated;
  }

  /**
   * Generate unique subagent ID
   */
  private generateId(): string {
    const timestamp = Date.now();
    const random = Math.random().toString(36).slice(2, 8);
    return `subagent-${timestamp}-${random}`;
  }

  /**
   * Check if a provider has available API keys
   */
  private isProviderAvailable(provider: Provider): boolean {
    switch (provider) {
      case 'anthropic':
        return !!(process.env.ANTHROPIC_API_KEY || process.env.ANTHROPIC_VERTEX_PROJECT_ID);
      case 'openai':
        return !!process.env.OPENAI_API_KEY;
      case 'google':
        return !!process.env.GOOGLE_API_KEY;
      default:
        return false;
    }
  }

  /**
   * Get system default model from settings.json or environment variables
   */
  private getSystemDefaultModel(): string | undefined {
    // Try ~/.gen/settings.json first
    const genSettingsPath = path.join(os.homedir(), '.gen', 'settings.json');
    try {
      if (fs.existsSync(genSettingsPath)) {
        const content = fs.readFileSync(genSettingsPath, 'utf-8');
        const settings = JSON.parse(content);
        if (settings.model) {
          return settings.model;
        }
      }
    } catch {
      // Ignore errors, fall through to env vars
    }

    // Fall back to environment variables
    return process.env.GEN_MODEL;
  }

  /**
   * Get current status
   */
  getStatus(): 'idle' | 'running' | 'completed' | 'error' {
    return this.status;
  }

  /**
   * Get result (if completed)
   */
  getResult(): string | null {
    return this.result;
  }

  /**
   * Get agent ID
   */
  getId(): string {
    return this.id;
  }

  /**
   * Get configuration
   */
  getConfig(): SubagentConfig {
    return this.config;
  }

  /**
   * Get underlying Agent instance (for advanced use cases)
   */
  getAgent(): Agent {
    return this.agent;
  }

  /**
   * Get current depth in subagent chain (Phase 4)
   */
  getCurrentDepth(): number {
    return this.currentDepth;
  }

  /**
   * Get maximum allowed depth (Phase 4)
   */
  getMaxDepth(): number {
    return this.maxDepth;
  }

  /**
   * Resume from saved session (Phase 3)
   */
  async resumeSession(sessionId: string, newPrompt: string): Promise<TaskOutput> {
    // Validate session
    const validation = await this.sessionManager.validateSubagentSession(sessionId);
    if (!validation.valid) {
      return {
        success: false,
        agentId: this.id,
        error: `Cannot resume: ${validation.reason}`,
      };
    }

    // Load session
    const session = await this.sessionManager.getSessionManager().load(sessionId);
    if (!session) {
      return {
        success: false,
        agentId: this.id,
        error: `Session not found: ${sessionId}`,
      };
    }

    // Restore agent state
    this.sessionId = sessionId;

    // Resume agent's session
    await this.agent.getSessionManager().save(session);
    await this.agent.resumeSession(sessionId);

    // Increment resume count
    await this.sessionManager.incrementResumeCount(sessionId);

    // Continue execution with new prompt
    return await this.run(newPrompt);
  }
}
