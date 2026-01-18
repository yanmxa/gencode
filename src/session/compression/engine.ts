/**
 * Three-layer compression engine (inspired by OpenCode)
 * Layer 1: Tool output pruning (fast, no cost)
 * Layer 2: Compaction summarization (LLM-based, medium cost)
 * Layer 3: Message filtering (recovery optimization)
 */

import type { Message, MessageContent } from '../../providers/types.js';
import type {
  CompressionConfig,
  ConversationSummary,
  ToolUsageSummary,
  TokenUsage,
  ModelInfo,
} from './types.js';
import { DEFAULT_COMPRESSION_CONFIG } from './types.js';

// Provider interface (minimal subset needed for compression)
interface LLMProvider {
  complete(options: {
    model: string;
    messages: Message[];
    maxTokens?: number;
  }): Promise<{ content: string | MessageContent[] }>;
  getModel?(): string;
}

/**
 * Compression engine implementing OpenCode's three-layer strategy
 */
export class CompressionEngine {
  private provider: LLMProvider;
  private config: CompressionConfig;

  // OpenCode constants
  private readonly CHARS_PER_TOKEN = 4;

  constructor(provider: LLMProvider, config?: Partial<CompressionConfig>) {
    this.provider = provider;
    this.config = { ...DEFAULT_COMPRESSION_CONFIG, ...config };
  }

  /**
   * Estimate tokens using 4:1 character-to-token ratio (OpenCode approach)
   */
  estimateTokens(text: string): number {
    return Math.max(0, Math.round((text || '').length / this.CHARS_PER_TOKEN));
  }

  /**
   * Calculate total tokens from messages
   */
  calculateTotalTokens(messages: Message[], tokenUsage?: TokenUsage): number {
    // If we have actual token usage data, use it
    if (tokenUsage) {
      return (
        tokenUsage.input +
        (tokenUsage.cache?.read || 0) +
        tokenUsage.output +
        (tokenUsage.reasoning || 0)
      );
    }

    // Otherwise estimate
    let total = 0;
    for (const msg of messages) {
      const content =
        typeof msg.content === 'string' ? msg.content : JSON.stringify(msg.content);
      total += this.estimateTokens(content);
    }
    return total;
  }

  /**
   * Calculate usable context space (OpenCode logic)
   */
  getUsableContext(model: ModelInfo): number {
    const maxOutput = Math.min(
      model.outputLimit || 4096,
      this.config.reservedOutputTokens
    );
    return model.contextWindow - maxOutput;
  }

  /**
   * Check if compression is needed (OpenCode isOverflow logic)
   */
  needsCompression(
    messages: Message[],
    model: ModelInfo,
    tokenUsage?: TokenUsage
  ): {
    needed: boolean;
    strategy: 'prune' | 'compact' | 'none';
    usagePercent?: number;
    shouldWarn?: boolean;
  } {
    if (!this.config.enabled) {
      return { needed: false, strategy: 'none' };
    }

    const totalTokens = this.calculateTotalTokens(messages, tokenUsage);
    const usable = this.getUsableContext(model);
    const usagePercent = (totalTokens / usable) * 100;

    // Warn at 80% usage
    const shouldWarn = usagePercent >= 80;

    // Auto-compress at 90% usage or if exceeding usable space
    const needed = totalTokens > usable || usagePercent >= 90;

    if (!needed) {
      return { needed: false, strategy: 'none', usagePercent, shouldWarn };
    }

    // Determine compression strategy
    let strategy: 'prune' | 'compact' = 'compact';
    if (totalTokens > this.config.pruneMinimum && this.config.enablePruning) {
      strategy = 'prune';
    }

    return { needed: true, strategy, usagePercent, shouldWarn };
  }

  /**
   * Layer 1: Tool output pruning (OpenCode pruning logic)
   * Fast and cost-free - removes old tool results
   */
  async pruneToolOutputs(messages: Message[]): Promise<{
    pruned: boolean;
    prunedCount: number;
    savedTokens: number;
  }> {
    const totalTokens = this.calculateTotalTokens(messages);
    if (totalTokens < this.config.pruneMinimum) {
      return { pruned: false, prunedCount: 0, savedTokens: 0 };
    }

    // Collect recent tool outputs (protect last 40k tokens)
    let protectedTokens = 0;
    const protectedIndices = new Set<number>();

    for (
      let i = messages.length - 1;
      i >= 0 && protectedTokens < this.config.pruneProtect;
      i--
    ) {
      const msg = messages[i];
      if (this.hasToolResults(msg)) {
        const msgTokens = this.calculateTotalTokens([msg]);
        protectedTokens += msgTokens;
        protectedIndices.add(i);
      }
    }

    // Prune older tool outputs
    let prunedCount = 0;
    let savedTokens = 0;

    for (let i = 0; i < messages.length; i++) {
      if (!protectedIndices.has(i) && this.hasToolResults(messages[i])) {
        const before = this.calculateTotalTokens([messages[i]]);
        this.clearToolResults(messages[i]);
        const after = this.calculateTotalTokens([messages[i]]);
        savedTokens += before - after;
        prunedCount++;
      }
    }

    return { pruned: prunedCount > 0, prunedCount, savedTokens };
  }

  /**
   * Layer 2: Compaction summarization (OpenCode compaction logic)
   * Generates continuation prompt focusing on future context needs
   */
  async compact(
    messages: Message[],
    range: [number, number]
  ): Promise<ConversationSummary> {
    const toSummarize = messages.slice(range[0], range[1] + 1);

    // Extract structured information
    const filesModified = this.extractFilesModified(toSummarize);
    const toolsUsed = this.extractToolUsage(toSummarize);
    const keyDecisions = await this.extractKeyDecisions(toSummarize);

    // Generate continuation prompt (OpenCode style)
    const continuationPrompt = await this.generateContinuationPrompt(toSummarize);

    return {
      id: this.generateId(),
      type: 'compaction',
      coveringMessages: range,
      content: continuationPrompt,
      keyDecisions,
      filesModified,
      toolsUsed,
      generatedAt: new Date().toISOString(),
      estimatedTokens: this.estimateTokens(continuationPrompt),
    };
  }

  /**
   * Generate continuation prompt (OpenCode style)
   * Focus on what's needed to continue, not just what was done
   */
  private async generateContinuationPrompt(messages: Message[]): Promise<string> {
    const prompt = `Provide a detailed prompt for continuing our conversation above.

Focus on information that would be helpful for continuing the conversation:
1. What we accomplished so far
2. What we're currently working on
3. Which files we modified and key changes made
4. What we plan to do next
5. Any important context or decisions that would be needed

Remember: The new session will NOT have access to our full conversation history,
so include all essential context needed to continue working effectively.

Be technical and specific. Use structured bullet points.

Conversation:
${this.formatMessagesForSummary(messages)}

Continuation Prompt:`;

    const response = await this.provider.complete({
      model: this.config.model ?? (this.provider.getModel?.() || 'unknown'),
      messages: [{ role: 'user', content: prompt }],
      maxTokens: 1500, // Larger for continuation prompt
    });

    return this.extractTextContent(response.content);
  }

  /**
   * Extract files modified from tool uses
   */
  private extractFilesModified(messages: Message[]): string[] {
    const files = new Set<string>();

    for (const msg of messages) {
      if (typeof msg.content !== 'string') {
        for (const block of msg.content) {
          if (block.type === 'tool_use') {
            if (['Write', 'Edit'].includes(block.name)) {
              const filePath = (block.input as any).file_path;
              if (filePath) files.add(filePath);
            }
          }
        }
      }
    }

    return Array.from(files);
  }

  /**
   * Extract tool usage statistics
   */
  private extractToolUsage(messages: Message[]): ToolUsageSummary[] {
    const toolStats = new Map<string, { count: number; uses: string[] }>();

    for (const msg of messages) {
      if (typeof msg.content !== 'string') {
        for (const block of msg.content) {
          if (block.type === 'tool_use') {
            const stats = toolStats.get(block.name) || { count: 0, uses: [] };
            stats.count++;

            if (stats.uses.length < 3) {
              stats.uses.push(this.summarizeToolUse(block));
            }

            toolStats.set(block.name, stats);
          }
        }
      }
    }

    return Array.from(toolStats.entries()).map(([tool, stats]) => ({
      tool,
      count: stats.count,
      notableUses: stats.uses,
    }));
  }

  /**
   * Extract key decisions from conversation
   */
  private async extractKeyDecisions(messages: Message[]): Promise<string[]> {
    const decisions: string[] = [];
    const decisionIndicators = ['decided to', 'chose to', 'will use', 'going with'];

    for (const msg of messages) {
      const content = typeof msg.content === 'string' ? msg.content : '';

      // Look for decision indicators
      const hasDecision = decisionIndicators.some((indicator) =>
        content.includes(indicator)
      );

      if (hasDecision) {
        const decision = this.extractDecisionContext(content);
        if (decision) decisions.push(decision);
      }
    }

    return decisions.slice(0, 5); // Keep top 5
  }

  // ===== Helper Methods =====

  /**
   * Check if message contains tool results
   */
  private hasToolResults(message: Message): boolean {
    if (typeof message.content === 'string') return false;
    return message.content.some((block) => block.type === 'tool_result');
  }

  /**
   * Clear tool result content (mark as pruned)
   */
  private clearToolResults(message: Message): void {
    if (typeof message.content !== 'string') {
      for (const block of message.content) {
        if (block.type === 'tool_result') {
          // Mark as pruned (OpenCode style)
          (block as any).content = '[Old tool result content cleared]';
          (block as any).pruned = true;
          (block as any).prunedAt = new Date().toISOString();
        }
      }
    }
  }

  /**
   * Format messages for summary prompt
   */
  private formatMessagesForSummary(messages: Message[]): string {
    return messages
      .map((msg, idx) => {
        const role = msg.role.toUpperCase();
        const content =
          typeof msg.content === 'string'
            ? msg.content
            : this.extractTextContent(msg.content);
        return `[${idx + 1}] ${role}: ${content.slice(0, 500)}`;
      })
      .join('\n\n');
  }

  /**
   * Extract text content from message content
   */
  private extractTextContent(content: string | MessageContent[]): string {
    if (typeof content === 'string') return content;
    if (Array.isArray(content)) {
      return content
        .filter((c) => c.type === 'text')
        .map((c) => (c as any).text)
        .join(' ');
    }
    return '';
  }

  /**
   * Summarize a tool use
   */
  private summarizeToolUse(block: any): string {
    switch (block.name) {
      case 'Write':
      case 'Edit':
        return `Modified ${block.input.file_path}`;
      case 'Bash':
        return `Ran: ${block.input.command?.slice(0, 50)}`;
      case 'Read':
        return `Read ${block.input.file_path}`;
      default:
        return `Used ${block.name}`;
    }
  }

  /**
   * Extract decision context from content
   */
  private extractDecisionContext(content: string): string | null {
    const sentences = content.split(/[.!?]/);
    const decisionKeywords = ['decided', 'chose', 'will use', 'going with'];

    const decisionSentence = sentences.find((s) =>
      decisionKeywords.some((keyword) => s.includes(keyword))
    );

    return decisionSentence?.trim() || null;
  }

  /**
   * Generate unique summary ID
   */
  private generateId(): string {
    return `sum-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
  }
}
