/**
 * OpenAI Provider Implementation
 * Supports GPT-4, GPT-4o, GPT-3.5-turbo, and other OpenAI models
 */

import OpenAI from 'openai';
import type {
  LLMProvider,
  CompletionOptions,
  CompletionResponse,
  StreamChunk,
  Message,
  MessageContent,
  ToolDefinition,
  StopReason,
  OpenAIConfig,
  ModelInfo,
} from './types.js';

type OpenAIMessage = OpenAI.Chat.Completions.ChatCompletionMessageParam;
type OpenAITool = OpenAI.Chat.Completions.ChatCompletionTool;

export class OpenAIProvider implements LLMProvider {
  readonly name = 'openai';
  private client: OpenAI;

  constructor(config: OpenAIConfig = {}) {
    this.client = new OpenAI({
      apiKey: config.apiKey ?? process.env.OPENAI_API_KEY,
      baseURL: config.baseURL,
      organization: config.organization,
    });
  }

  async complete(options: CompletionOptions): Promise<CompletionResponse> {
    const messages = this.convertMessages(options.messages, options.systemPrompt);
    const tools = options.tools ? this.convertTools(options.tools) : undefined;

    const response = await this.client.chat.completions.create({
      model: options.model,
      messages,
      tools,
      max_tokens: options.maxTokens,
      temperature: options.temperature,
    });

    return this.convertResponse(response);
  }

  async *stream(options: CompletionOptions): AsyncGenerator<StreamChunk, void, unknown> {
    const messages = this.convertMessages(options.messages, options.systemPrompt);
    const tools = options.tools ? this.convertTools(options.tools) : undefined;

    const stream = await this.client.chat.completions.create({
      model: options.model,
      messages,
      tools,
      max_tokens: options.maxTokens,
      temperature: options.temperature,
      stream: true,
    });

    const toolCalls: Map<number, { id: string; name: string; arguments: string }> = new Map();
    let textContent = '';
    let finishReason: string | null = null;

    for await (const chunk of stream) {
      const delta = chunk.choices[0]?.delta;
      finishReason = chunk.choices[0]?.finish_reason ?? finishReason;

      if (delta?.content) {
        textContent += delta.content;
        yield { type: 'text', text: delta.content };
      }

      if (delta?.tool_calls) {
        for (const tc of delta.tool_calls) {
          const existing = toolCalls.get(tc.index);
          if (existing) {
            if (tc.function?.arguments) {
              existing.arguments += tc.function.arguments;
              yield { type: 'tool_input', id: existing.id, input: tc.function.arguments };
            }
          } else if (tc.id && tc.function?.name) {
            toolCalls.set(tc.index, {
              id: tc.id,
              name: tc.function.name,
              arguments: tc.function.arguments ?? '',
            });
            yield { type: 'tool_start', id: tc.id, name: tc.function.name };
            if (tc.function.arguments) {
              yield { type: 'tool_input', id: tc.id, input: tc.function.arguments };
            }
          }
        }
      }
    }

    // Build final response
    const content: MessageContent[] = [];
    if (textContent) {
      content.push({ type: 'text', text: textContent });
    }
    for (const tc of toolCalls.values()) {
      content.push({
        type: 'tool_use',
        id: tc.id,
        name: tc.name,
        input: JSON.parse(tc.arguments || '{}'),
      });
    }

    yield {
      type: 'done',
      response: {
        content,
        stopReason: this.convertStopReason(finishReason),
      },
    };
  }

  private convertMessages(messages: Message[], systemPrompt?: string): OpenAIMessage[] {
    const result: OpenAIMessage[] = [];

    if (systemPrompt) {
      result.push({ role: 'system', content: systemPrompt });
    }

    for (const msg of messages) {
      if (msg.role === 'system') {
        result.push({ role: 'system', content: this.getTextContent(msg.content) });
      } else if (msg.role === 'user') {
        // Check if message contains tool results
        if (Array.isArray(msg.content)) {
          const toolResults = msg.content.filter((c) => c.type === 'tool_result');
          if (toolResults.length > 0) {
            for (const tr of toolResults) {
              if (tr.type === 'tool_result') {
                result.push({
                  role: 'tool',
                  tool_call_id: tr.toolUseId,
                  content: tr.content,
                });
              }
            }
            continue;
          }
        }
        result.push({ role: 'user', content: this.getTextContent(msg.content) });
      } else if (msg.role === 'assistant') {
        const assistantMsg: OpenAI.Chat.Completions.ChatCompletionAssistantMessageParam = {
          role: 'assistant',
        };

        if (Array.isArray(msg.content)) {
          const textParts = msg.content.filter((c) => c.type === 'text');
          const toolUses = msg.content.filter((c) => c.type === 'tool_use');

          if (textParts.length > 0) {
            assistantMsg.content = textParts.map((t) => (t as { text: string }).text).join('');
          }

          if (toolUses.length > 0) {
            assistantMsg.tool_calls = toolUses.map((t) => {
              const tu = t as { id: string; name: string; input: Record<string, unknown> };
              return {
                id: tu.id,
                type: 'function' as const,
                function: {
                  name: tu.name,
                  arguments: JSON.stringify(tu.input),
                },
              };
            });
          }
        } else {
          assistantMsg.content = msg.content;
        }

        result.push(assistantMsg);
      }
    }

    return result;
  }

  private convertTools(tools: ToolDefinition[]): OpenAITool[] {
    return tools.map((tool) => ({
      type: 'function' as const,
      function: {
        name: tool.name,
        description: tool.description,
        parameters: tool.parameters,
      },
    }));
  }

  private convertResponse(response: OpenAI.Chat.Completions.ChatCompletion): CompletionResponse {
    const choice = response.choices[0];
    const content: MessageContent[] = [];

    if (choice.message.content) {
      content.push({ type: 'text', text: choice.message.content });
    }

    if (choice.message.tool_calls) {
      for (const tc of choice.message.tool_calls) {
        if (tc.type === 'function' && tc.function) {
          content.push({
            type: 'tool_use',
            id: tc.id,
            name: tc.function.name,
            input: JSON.parse(tc.function.arguments || '{}'),
          });
        }
      }
    }

    return {
      content,
      stopReason: this.convertStopReason(choice.finish_reason),
      usage: response.usage
        ? {
            inputTokens: response.usage.prompt_tokens,
            outputTokens: response.usage.completion_tokens,
          }
        : undefined,
    };
  }

  private convertStopReason(reason: string | null): StopReason {
    switch (reason) {
      case 'tool_calls':
        return 'tool_use';
      case 'length':
        return 'max_tokens';
      case 'stop':
      default:
        return 'end_turn';
    }
  }

  private getTextContent(content: string | MessageContent[]): string {
    if (typeof content === 'string') {
      return content;
    }
    return content
      .filter((c) => c.type === 'text')
      .map((c) => (c as { text: string }).text)
      .join('');
  }

  async listModels(): Promise<ModelInfo[]> {
    const list = await this.client.models.list();
    const models: ModelInfo[] = [];
    for await (const model of list) {
      models.push({ id: model.id, name: model.id });
    }
    return models.sort((a, b) => a.id.localeCompare(b.id));
  }
}
