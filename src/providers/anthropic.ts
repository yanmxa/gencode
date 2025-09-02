/**
 * Anthropic Provider Implementation
 * Supports Claude 3.5 Sonnet, Claude 3 Opus, Claude 3 Haiku, etc.
 */

import Anthropic from '@anthropic-ai/sdk';
import type {
  LLMProvider,
  CompletionOptions,
  CompletionResponse,
  StreamChunk,
  Message,
  MessageContent,
  ToolDefinition,
  StopReason,
  AnthropicConfig,
} from './types.js';

type AnthropicMessage = Anthropic.MessageParam;
type AnthropicTool = Anthropic.Tool;
type AnthropicContent = Anthropic.ContentBlockParam;

export class AnthropicProvider implements LLMProvider {
  readonly name = 'anthropic';
  private client: Anthropic;

  constructor(config: AnthropicConfig = {}) {
    this.client = new Anthropic({
      apiKey: config.apiKey ?? process.env.ANTHROPIC_API_KEY,
      baseURL: config.baseURL,
    });
  }

  async complete(options: CompletionOptions): Promise<CompletionResponse> {
    const messages = this.convertMessages(options.messages);
    const tools = options.tools ? this.convertTools(options.tools) : undefined;

    const response = await this.client.messages.create({
      model: options.model,
      messages,
      tools,
      system: options.systemPrompt,
      max_tokens: options.maxTokens ?? 4096,
      temperature: options.temperature,
    });

    return this.convertResponse(response);
  }

  async *stream(options: CompletionOptions): AsyncGenerator<StreamChunk, void, unknown> {
    const messages = this.convertMessages(options.messages);
    const tools = options.tools ? this.convertTools(options.tools) : undefined;

    const stream = this.client.messages.stream({
      model: options.model,
      messages,
      tools,
      system: options.systemPrompt,
      max_tokens: options.maxTokens ?? 4096,
      temperature: options.temperature,
    });

    const toolInputBuffers: Map<number, { id: string; name: string; input: string }> = new Map();

    for await (const event of stream) {
      if (event.type === 'content_block_start') {
        const block = event.content_block;

        if (block.type === 'tool_use') {
          toolInputBuffers.set(event.index, {
            id: block.id,
            name: block.name,
            input: '',
          });
          yield { type: 'tool_start', id: block.id, name: block.name };
        }
      } else if (event.type === 'content_block_delta') {
        const delta = event.delta;

        if (delta.type === 'text_delta') {
          yield { type: 'text', text: delta.text };
        } else if (delta.type === 'input_json_delta') {
          const buffer = toolInputBuffers.get(event.index);
          if (buffer) {
            buffer.input += delta.partial_json;
            yield { type: 'tool_input', id: buffer.id, input: delta.partial_json };
          }
        }
      }
    }

    // Get final message
    const finalMessage = await stream.finalMessage();
    const content = this.convertContent(finalMessage.content);

    yield {
      type: 'done',
      response: {
        content,
        stopReason: this.convertStopReason(finalMessage.stop_reason),
        usage: {
          inputTokens: finalMessage.usage.input_tokens,
          outputTokens: finalMessage.usage.output_tokens,
        },
      },
    };
  }

  private convertMessages(messages: Message[]): AnthropicMessage[] {
    const result: AnthropicMessage[] = [];

    for (const msg of messages) {
      // Skip system messages - handled separately
      if (msg.role === 'system') {
        continue;
      }

      if (msg.role === 'user') {
        const content = this.convertToAnthropicContent(msg.content, 'user');
        result.push({ role: 'user', content });
      } else if (msg.role === 'assistant') {
        const content = this.convertToAnthropicContent(msg.content, 'assistant');
        result.push({ role: 'assistant', content });
      }
    }

    return result;
  }

  private convertToAnthropicContent(
    content: string | MessageContent[],
    role: 'user' | 'assistant'
  ): AnthropicContent[] | string {
    if (typeof content === 'string') {
      return content;
    }

    const result: AnthropicContent[] = [];

    for (const item of content) {
      if (item.type === 'text') {
        result.push({ type: 'text', text: item.text });
      } else if (item.type === 'tool_use' && role === 'assistant') {
        result.push({
          type: 'tool_use',
          id: item.id,
          name: item.name,
          input: item.input,
        });
      } else if (item.type === 'tool_result' && role === 'user') {
        result.push({
          type: 'tool_result',
          tool_use_id: item.toolUseId,
          content: item.content,
          is_error: item.isError,
        });
      }
    }

    return result.length > 0 ? result : '';
  }

  private convertTools(tools: ToolDefinition[]): AnthropicTool[] {
    return tools.map((tool) => ({
      name: tool.name,
      description: tool.description,
      input_schema: tool.parameters as Anthropic.Tool.InputSchema,
    }));
  }

  private convertResponse(response: Anthropic.Message): CompletionResponse {
    return {
      content: this.convertContent(response.content),
      stopReason: this.convertStopReason(response.stop_reason),
      usage: {
        inputTokens: response.usage.input_tokens,
        outputTokens: response.usage.output_tokens,
      },
    };
  }

  private convertContent(content: Anthropic.ContentBlock[]): MessageContent[] {
    return content.map((block) => {
      if (block.type === 'text') {
        return { type: 'text' as const, text: block.text };
      } else if (block.type === 'tool_use') {
        return {
          type: 'tool_use' as const,
          id: block.id,
          name: block.name,
          input: block.input as Record<string, unknown>,
        };
      }
      // Fallback for unknown types
      return { type: 'text' as const, text: '' };
    });
  }

  private convertStopReason(reason: Anthropic.Message['stop_reason']): StopReason {
    switch (reason) {
      case 'tool_use':
        return 'tool_use';
      case 'max_tokens':
        return 'max_tokens';
      case 'stop_sequence':
        return 'stop_sequence';
      case 'end_turn':
      default:
        return 'end_turn';
    }
  }
}
