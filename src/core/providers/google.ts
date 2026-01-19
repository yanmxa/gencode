/**
 * Google Provider Implementation
 * Supports Gemini models: Gemini 1.5 Pro, Gemini 1.5 Flash, Gemini 2.0, etc.
 */

import { GoogleGenerativeAI, SchemaType } from '@google/generative-ai';
import type { Content, Part, Tool, GenerateContentResult } from '@google/generative-ai';
import { calculateCost } from '../pricing/calculator.js';
import type {
  LLMProvider,
  ProviderClassMeta,
  CompletionOptions,
  CompletionResponse,
  StreamChunk,
  Message,
  MessageContent,
  ToolDefinition,
  StopReason,
  GoogleConfig,
  JSONSchema,
  ModelInfo,
} from './types.js';

export class GoogleProvider implements LLMProvider {
  static readonly meta: ProviderClassMeta = {
    provider: 'google',
    authMethod: 'api_key',
    envVars: ['GOOGLE_API_KEY', 'GEMINI_API_KEY'],
    displayName: 'Google',
    description: 'Google Generative AI (Gemini models)',
  };

  readonly name = 'google';
  private client: GoogleGenerativeAI;
  private apiKey: string;

  constructor(config: GoogleConfig = {}) {
    const apiKey = config.apiKey ?? process.env.GOOGLE_API_KEY ?? process.env.GEMINI_API_KEY;
    if (!apiKey) {
      throw new Error('Google API key is required. Set GOOGLE_API_KEY or GEMINI_API_KEY.');
    }
    this.apiKey = apiKey;
    this.client = new GoogleGenerativeAI(apiKey);
  }

  async complete(options: CompletionOptions): Promise<CompletionResponse> {
    const model = this.client.getGenerativeModel({
      model: options.model,
      systemInstruction: options.systemPrompt,
      generationConfig: {
        maxOutputTokens: options.maxTokens,
        temperature: options.temperature,
      },
      tools: options.tools ? this.convertTools(options.tools) : undefined,
    });

    const contents = this.convertMessages(options.messages);
    const result = await model.generateContent({ contents });

    return this.convertResponse(result, options.model);
  }

  async *stream(options: CompletionOptions): AsyncGenerator<StreamChunk, void, unknown> {
    const model = this.client.getGenerativeModel({
      model: options.model,
      systemInstruction: options.systemPrompt,
      generationConfig: {
        maxOutputTokens: options.maxTokens,
        temperature: options.temperature,
      },
      tools: options.tools ? this.convertTools(options.tools) : undefined,
    });

    const contents = this.convertMessages(options.messages);
    const result = await model.generateContentStream({ contents });

    let textContent = '';
    const functionCalls: Array<{
      id: string;
      name: string;
      args: Record<string, unknown>;
      thoughtSignature?: string;
    }> = [];
    let callIndex = 0;

    for await (const chunk of result.stream) {
      const parts = chunk.candidates?.[0]?.content?.parts ?? [];

      for (const part of parts) {
        if ('text' in part && part.text) {
          textContent += part.text;
          yield { type: 'text', text: part.text };
        } else if ('functionCall' in part && part.functionCall) {
          const fc = part.functionCall;
          const id = `call_${callIndex++}`;
          // Capture thoughtSignature for Gemini 3+ models
          const partAny = part as { thoughtSignature?: string };

          // Emit reasoning content if available (Gemini 3+ thinking)
          if (partAny.thoughtSignature) {
            yield { type: 'reasoning', text: partAny.thoughtSignature };
          }

          functionCalls.push({
            id,
            name: fc.name,
            args: (fc.args as Record<string, unknown>) ?? {},
            thoughtSignature: partAny.thoughtSignature,
          });
          yield { type: 'tool_start', id, name: fc.name };
          yield { type: 'tool_input', id, input: JSON.stringify(fc.args) };
        }
      }
    }

    // Build final response
    const content: MessageContent[] = [];
    if (textContent) {
      content.push({ type: 'text', text: textContent });
    }
    for (const fc of functionCalls) {
      content.push({
        type: 'tool_use',
        id: fc.id,
        name: fc.name,
        input: fc.args,
        thoughtSignature: fc.thoughtSignature,
      });
    }

    const finalResponse = await result.response;
    const stopReason = this.getStopReason(finalResponse, functionCalls.length > 0);

    // Debug: Log usage metadata
    if (process.env.DEBUG_TOKENS) {
      console.error('[Google] usageMetadata:', JSON.stringify(finalResponse.usageMetadata, null, 2));
    }

    const usage = finalResponse.usageMetadata
      ? {
          inputTokens: finalResponse.usageMetadata.promptTokenCount ?? 0,
          // Fix: candidatesTokenCount is unreliable, calculate from total - prompt
          // Ensure outputTokens is never negative
          outputTokens: Math.max(
            0,
            (finalResponse.usageMetadata.totalTokenCount ?? 0) - (finalResponse.usageMetadata.promptTokenCount ?? 0)
          ),
        }
      : undefined;

    // Warn if suspicious token count
    if (usage && usage.outputTokens === 0 && content.length > 0) {
      console.warn(
        '[Google] Warning: usageMetadata shows 0 output tokens but content was returned. ' +
        'This may indicate a Gemini API issue.'
      );
    }

    const cost = usage ? calculateCost(this.name, options.model, usage) : undefined;

    yield {
      type: 'done',
      response: {
        content,
        stopReason,
        usage,
        cost,
      },
    };
  }

  private convertMessages(messages: Message[]): Content[] {
    const contents: Content[] = [];

    for (const msg of messages) {
      // Skip system messages - handled via systemInstruction
      if (msg.role === 'system') {
        continue;
      }

      const role = msg.role === 'assistant' ? 'model' : 'user';
      const parts = this.convertToParts(msg.content, role);

      if (parts.length > 0) {
        contents.push({ role, parts });
      }
    }

    return contents;
  }

  private convertToParts(content: string | MessageContent[], role: string): Part[] {
    if (typeof content === 'string') {
      return [{ text: content }];
    }

    const parts: Part[] = [];

    for (const item of content) {
      if (item.type === 'text') {
        parts.push({ text: item.text });
      } else if (item.type === 'tool_use' && role === 'model') {
        // Function call from model - include thoughtSignature for Gemini 3+
        const fcPart: Part & { thoughtSignature?: string } = {
          functionCall: {
            name: item.name,
            args: item.input,
          },
        };
        if (item.thoughtSignature) {
          fcPart.thoughtSignature = item.thoughtSignature;
        }
        parts.push(fcPart as Part);
      } else if (item.type === 'tool_result' && role === 'user') {
        // Function response
        parts.push({
          functionResponse: {
            name: item.toolUseId, // Use toolUseId as name reference
            response: { result: item.content },
          },
        });
      }
    }

    return parts;
  }

  private convertTools(tools: ToolDefinition[]): Tool[] {
    const result = [
      {
        functionDeclarations: tools.map((tool) => {
          const convertedSchema = this.convertSchema(tool.parameters);

          // Debug: Log Task tool schema
          if (tool.name === 'Task' && process.env.DEBUG_SCHEMA) {
            console.error('[Google] Task tool input schema:');
            console.error(JSON.stringify(tool.parameters, null, 2));
            console.error('[Google] Task tool converted schema:');
            console.error(JSON.stringify(convertedSchema, null, 2));
          }

          return {
            name: tool.name,
            description: tool.description,
            parameters: convertedSchema,
          };
        }),
      },
    ];

    return result;
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private convertSchema(schema: JSONSchema): any {
    const convertType = (type: string): SchemaType => {
      switch (type) {
        case 'string':
          return SchemaType.STRING;
        case 'number':
        case 'integer':
          return SchemaType.NUMBER;
        case 'boolean':
          return SchemaType.BOOLEAN;
        case 'array':
          return SchemaType.ARRAY;
        case 'object':
        default:
          return SchemaType.OBJECT;
      }
    };

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result: any = {
      type: convertType(schema.type),
      description: schema.description,
    };

    if (schema.properties) {
      result.properties = {};
      for (const [key, value] of Object.entries(schema.properties)) {
        result.properties[key] = this.convertSchema(value);
      }
    }

    if (schema.required) {
      result.required = schema.required;
    }

    if (schema.items) {
      result.items = this.convertSchema(schema.items);
    }

    if (schema.enum) {
      result.enum = schema.enum;
    }

    return result;
  }

  private convertResponse(result: GenerateContentResult, model: string): CompletionResponse {
    const response = result.response;
    const parts = response.candidates?.[0]?.content?.parts ?? [];
    const content: MessageContent[] = [];
    let callIndex = 0;

    for (const part of parts) {
      if ('text' in part && part.text) {
        content.push({ type: 'text', text: part.text });
      } else if ('functionCall' in part && part.functionCall) {
        // Capture thoughtSignature for Gemini 3+ models
        const partAny = part as { thoughtSignature?: string };
        content.push({
          type: 'tool_use',
          id: `call_${callIndex++}`,
          name: part.functionCall.name,
          input: (part.functionCall.args as Record<string, unknown>) ?? {},
          thoughtSignature: partAny.thoughtSignature,
        });
      }
    }

    const hasFunctionCalls = parts.some((p) => 'functionCall' in p);

    // Debug: Log usage metadata
    if (process.env.DEBUG_TOKENS) {
      console.error('[Google complete] usageMetadata:', JSON.stringify(response.usageMetadata, null, 2));
    }

    const usage = response.usageMetadata
      ? {
          inputTokens: response.usageMetadata.promptTokenCount ?? 0,
          // Fix: candidatesTokenCount is unreliable, calculate from total - prompt
          // Ensure outputTokens is never negative
          outputTokens: Math.max(
            0,
            (response.usageMetadata.totalTokenCount ?? 0) - (response.usageMetadata.promptTokenCount ?? 0)
          ),
        }
      : undefined;

    // Warn if suspicious token count
    if (usage && usage.outputTokens === 0 && content.length > 0) {
      console.warn(
        '[Google] Warning: usageMetadata shows 0 output tokens but content was returned. ' +
        'This may indicate a Gemini API issue.'
      );
    }

    const cost = usage ? calculateCost(this.name, model, usage) : undefined;

    return {
      content,
      stopReason: this.getStopReason(response, hasFunctionCalls),
      usage,
      cost,
    };
  }

  private getStopReason(response: GenerateContentResult['response'], hasFunctionCalls: boolean): StopReason {
    if (hasFunctionCalls) {
      return 'tool_use';
    }

    const finishReason = response.candidates?.[0]?.finishReason;

    if (finishReason === 'MAX_TOKENS') {
      return 'max_tokens';
    }

    if (finishReason === 'STOP') {
      return 'end_turn';
    }

    if (finishReason === 'SAFETY' || finishReason === 'RECITATION') {
      console.warn(`[Google] Content blocked by ${finishReason} filter`);
      return 'end_turn';
    }

    console.warn(`[Google] Unknown finishReason: ${finishReason}`);
    return 'end_turn';
  }

  async listModels(): Promise<ModelInfo[]> {
    const response = await fetch(
      `https://generativelanguage.googleapis.com/v1beta/models?key=${this.apiKey}`
    );
    const data = (await response.json()) as {
      models?: Array<{
        name: string;
        displayName: string;
        description?: string;
        supportedGenerationMethods?: string[];
      }>;
    };
    return (data.models || [])
      .filter((m) => m.supportedGenerationMethods?.includes('generateContent'))
      .map((m) => ({
        id: m.name.replace('models/', ''),
        name: m.displayName,
        description: m.description,
      }));
  }
}
