/**
 * Google Vertex AI Provider Implementation
 * Supports Claude models deployed on Google Cloud Vertex AI
 *
 * Authentication uses Google Cloud's default credential chain:
 * 1. GOOGLE_APPLICATION_CREDENTIALS (service account JSON)
 * 2. gcloud auth application-default login (ADC)
 * 3. GCE/GKE metadata service (when running on GCP)
 */

import { GoogleAuth } from 'google-auth-library';
import type {
  LLMProvider,
  CompletionOptions,
  CompletionResponse,
  StreamChunk,
  Message,
  MessageContent,
  ToolDefinition,
  StopReason,
  VertexAIConfig,
  ModelInfo,
} from './types.js';

// Vertex AI API types (compatible with Anthropic format)
interface VertexAIMessage {
  role: 'user' | 'assistant';
  content: string | VertexAIContent[];
}

interface VertexAIContent {
  type: 'text' | 'tool_use' | 'tool_result';
  text?: string;
  id?: string;
  name?: string;
  input?: Record<string, unknown>;
  tool_use_id?: string;
  content?: string;
  is_error?: boolean;
}

interface VertexAITool {
  name: string;
  description: string;
  input_schema: {
    type: string;
    properties?: Record<string, unknown>;
    required?: string[];
  };
}

interface VertexAIRequest {
  anthropic_version: string;
  max_tokens: number;
  messages: VertexAIMessage[];
  system?: string;
  tools?: VertexAITool[];
  temperature?: number;
  stream?: boolean;
}

interface VertexAIResponse {
  id: string;
  type: string;
  role: string;
  content: Array<{
    type: 'text' | 'tool_use';
    text?: string;
    id?: string;
    name?: string;
    input?: Record<string, unknown>;
  }>;
  stop_reason: 'end_turn' | 'tool_use' | 'max_tokens' | 'stop_sequence';
  usage: {
    input_tokens: number;
    output_tokens: number;
  };
}

// SSE stream event types
interface StreamEventContentBlockStart {
  type: 'content_block_start';
  index: number;
  content_block: {
    type: 'text' | 'tool_use';
    text?: string;
    id?: string;
    name?: string;
  };
}

interface StreamEventContentBlockDelta {
  type: 'content_block_delta';
  index: number;
  delta: {
    type: 'text_delta' | 'input_json_delta';
    text?: string;
    partial_json?: string;
  };
}

interface StreamEventMessageDelta {
  type: 'message_delta';
  delta: {
    stop_reason: string;
  };
  usage: {
    output_tokens: number;
  };
}

interface StreamEventMessageStart {
  type: 'message_start';
  message: {
    id: string;
    usage: {
      input_tokens: number;
    };
  };
}

type StreamEvent =
  | StreamEventContentBlockStart
  | StreamEventContentBlockDelta
  | StreamEventMessageDelta
  | StreamEventMessageStart
  | { type: 'content_block_stop' }
  | { type: 'message_stop' }
  | { type: 'ping' };

export class VertexAIProvider implements LLMProvider {
  readonly name = 'vertex-ai';
  private projectId: string;
  private region: string;
  private auth: GoogleAuth;
  private accessToken?: string;

  constructor(config: VertexAIConfig = {}) {
    this.projectId =
      config.projectId ??
      process.env.ANTHROPIC_VERTEX_PROJECT_ID ??
      process.env.GOOGLE_CLOUD_PROJECT ??
      '';

    this.region =
      config.region ??
      process.env.ANTHROPIC_VERTEX_REGION ??
      process.env.CLOUD_ML_REGION ??
      'us-east5';

    this.accessToken = config.accessToken;

    if (!this.projectId) {
      throw new Error(
        'Vertex AI requires a project ID. Set ANTHROPIC_VERTEX_PROJECT_ID environment variable.'
      );
    }

    this.auth = new GoogleAuth({
      scopes: ['https://www.googleapis.com/auth/cloud-platform'],
    });
  }

  private async getAccessToken(): Promise<string> {
    if (this.accessToken) {
      return this.accessToken;
    }

    const client = await this.auth.getClient();
    const tokenResponse = await client.getAccessToken();
    if (!tokenResponse.token) {
      throw new Error('Failed to get access token from Google Cloud');
    }
    return tokenResponse.token;
  }

  private getEndpoint(model: string, stream: boolean = false): string {
    const method = stream ? 'streamRawPredict' : 'rawPredict';
    return `https://${this.region}-aiplatform.googleapis.com/v1/projects/${this.projectId}/locations/${this.region}/publishers/anthropic/models/${model}:${method}`;
  }

  async complete(options: CompletionOptions): Promise<CompletionResponse> {
    const messages = this.convertMessages(options.messages);
    const tools = options.tools ? this.convertTools(options.tools) : undefined;

    const requestBody: VertexAIRequest = {
      anthropic_version: 'vertex-2023-10-16',
      max_tokens: options.maxTokens ?? 4096,
      messages,
      system: options.systemPrompt,
      tools,
      temperature: options.temperature,
    };

    const accessToken = await this.getAccessToken();
    const endpoint = this.getEndpoint(options.model, false);

    const response = await fetch(endpoint, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${accessToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(requestBody),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Vertex AI API error: ${response.status} ${errorText}`);
    }

    const data = (await response.json()) as VertexAIResponse;
    return this.convertResponse(data);
  }

  async *stream(options: CompletionOptions): AsyncGenerator<StreamChunk, void, unknown> {
    const messages = this.convertMessages(options.messages);
    const tools = options.tools ? this.convertTools(options.tools) : undefined;

    const requestBody: VertexAIRequest = {
      anthropic_version: 'vertex-2023-10-16',
      max_tokens: options.maxTokens ?? 4096,
      messages,
      system: options.systemPrompt,
      tools,
      temperature: options.temperature,
      stream: true,
    };

    const accessToken = await this.getAccessToken();
    const endpoint = this.getEndpoint(options.model, true);

    const response = await fetch(endpoint, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${accessToken}`,
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(requestBody),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Vertex AI API error: ${response.status} ${errorText}`);
    }

    if (!response.body) {
      throw new Error('No response body from Vertex AI');
    }

    const toolInputBuffers: Map<number, { id: string; name: string; input: string }> = new Map();
    const contentBlocks: Map<
      number,
      { type: 'text' | 'tool_use'; text?: string; id?: string; name?: string; input?: string }
    > = new Map();
    let inputTokens = 0;
    let outputTokens = 0;
    let stopReason: StopReason = 'end_turn';

    // Parse SSE stream
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        // Process complete SSE events
        const lines = buffer.split('\n');
        buffer = lines.pop() ?? '';

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const jsonStr = line.slice(6).trim();
            if (!jsonStr || jsonStr === '[DONE]') continue;

            try {
              const event = JSON.parse(jsonStr) as StreamEvent;

              if (event.type === 'message_start') {
                inputTokens = event.message.usage.input_tokens;
              } else if (event.type === 'content_block_start') {
                const block = event.content_block;
                if (block.type === 'tool_use' && block.id && block.name) {
                  toolInputBuffers.set(event.index, {
                    id: block.id,
                    name: block.name,
                    input: '',
                  });
                  contentBlocks.set(event.index, {
                    type: 'tool_use',
                    id: block.id,
                    name: block.name,
                    input: '',
                  });
                  yield { type: 'tool_start', id: block.id, name: block.name };
                } else if (block.type === 'text') {
                  contentBlocks.set(event.index, { type: 'text', text: '' });
                }
              } else if (event.type === 'content_block_delta') {
                const delta = event.delta;
                if (delta.type === 'text_delta' && delta.text) {
                  const block = contentBlocks.get(event.index);
                  if (block && block.type === 'text') {
                    block.text = (block.text ?? '') + delta.text;
                  }
                  yield { type: 'text', text: delta.text };
                } else if (delta.type === 'input_json_delta' && delta.partial_json) {
                  const toolBuffer = toolInputBuffers.get(event.index);
                  if (toolBuffer) {
                    toolBuffer.input += delta.partial_json;
                    const block = contentBlocks.get(event.index);
                    if (block && block.type === 'tool_use') {
                      block.input = toolBuffer.input;
                    }
                    yield { type: 'tool_input', id: toolBuffer.id, input: delta.partial_json };
                  }
                }
              } else if (event.type === 'message_delta') {
                stopReason = this.convertStopReason(event.delta.stop_reason);
                outputTokens = event.usage.output_tokens;
              }
            } catch {
              // Skip malformed JSON
            }
          }
        }
      }
    } finally {
      reader.releaseLock();
    }

    // Build final response
    const content = this.buildFinalContent(contentBlocks, toolInputBuffers);

    yield {
      type: 'done',
      response: {
        content,
        stopReason,
        usage: {
          inputTokens,
          outputTokens,
        },
      },
    };
  }

  private buildFinalContent(
    contentBlocks: Map<
      number,
      { type: 'text' | 'tool_use'; text?: string; id?: string; name?: string; input?: string }
    >,
    toolInputBuffers: Map<number, { id: string; name: string; input: string }>
  ): MessageContent[] {
    const result: MessageContent[] = [];

    const sortedIndices = Array.from(contentBlocks.keys()).sort((a, b) => a - b);

    for (const index of sortedIndices) {
      const block = contentBlocks.get(index);
      if (!block) continue;

      if (block.type === 'text' && block.text) {
        result.push({ type: 'text', text: block.text });
      } else if (block.type === 'tool_use' && block.id && block.name) {
        const toolBuffer = toolInputBuffers.get(index);
        let input: Record<string, unknown> = {};
        try {
          input = JSON.parse(toolBuffer?.input ?? '{}');
        } catch {
          // Use empty object if parsing fails
        }
        result.push({
          type: 'tool_use',
          id: block.id,
          name: block.name,
          input,
        });
      }
    }

    return result;
  }

  private convertMessages(messages: Message[]): VertexAIMessage[] {
    const result: VertexAIMessage[] = [];

    for (const msg of messages) {
      // Skip system messages - handled separately
      if (msg.role === 'system') {
        continue;
      }

      if (msg.role === 'user') {
        const content = this.convertToVertexContent(msg.content, 'user');
        result.push({ role: 'user', content });
      } else if (msg.role === 'assistant') {
        const content = this.convertToVertexContent(msg.content, 'assistant');
        result.push({ role: 'assistant', content });
      }
    }

    return result;
  }

  private convertToVertexContent(
    content: string | MessageContent[],
    role: 'user' | 'assistant'
  ): VertexAIContent[] | string {
    if (typeof content === 'string') {
      return content;
    }

    const result: VertexAIContent[] = [];

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

  private convertTools(tools: ToolDefinition[]): VertexAITool[] {
    return tools.map((tool) => ({
      name: tool.name,
      description: tool.description,
      input_schema: tool.parameters as VertexAITool['input_schema'],
    }));
  }

  private convertResponse(response: VertexAIResponse): CompletionResponse {
    return {
      content: this.convertContent(response.content),
      stopReason: this.convertStopReason(response.stop_reason),
      usage: {
        inputTokens: response.usage.input_tokens,
        outputTokens: response.usage.output_tokens,
      },
    };
  }

  private convertContent(
    content: Array<{
      type: 'text' | 'tool_use';
      text?: string;
      id?: string;
      name?: string;
      input?: Record<string, unknown>;
    }>
  ): MessageContent[] {
    return content.map((block) => {
      if (block.type === 'text') {
        return { type: 'text' as const, text: block.text ?? '' };
      } else if (block.type === 'tool_use') {
        return {
          type: 'tool_use' as const,
          id: block.id ?? '',
          name: block.name ?? '',
          input: block.input ?? {},
        };
      }
      // Fallback for unknown types
      return { type: 'text' as const, text: '' };
    });
  }

  private convertStopReason(reason: string): StopReason {
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

  async listModels(): Promise<ModelInfo[]> {
    // Use Vertex AI Model Garden API to list Anthropic publisher models
    // API: GET https://{region}-aiplatform.googleapis.com/v1beta1/publishers/anthropic/models
    const accessToken = await this.getAccessToken();
    const endpoint = `https://${this.region}-aiplatform.googleapis.com/v1beta1/publishers/anthropic/models`;

    try {
      const response = await fetch(endpoint, {
        method: 'GET',
        headers: {
          Authorization: `Bearer ${accessToken}`,
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        // Fall back to known models if API fails
        return this.getKnownModels();
      }

      const data = (await response.json()) as {
        publisherModels?: Array<{
          name?: string;
          versionId?: string;
          openSourceCategory?: string;
          supportedActions?: Record<string, unknown>;
          publisherModelTemplate?: string;
        }>;
      };

      if (!data.publisherModels || data.publisherModels.length === 0) {
        return this.getKnownModels();
      }

      const models: ModelInfo[] = [];
      for (const model of data.publisherModels) {
        // Extract model ID from name (e.g., "publishers/anthropic/models/claude-3-5-sonnet")
        const modelName = model.name ?? '';
        const modelId = modelName.split('/').pop() ?? '';

        if (modelId && modelId.includes('claude')) {
          // Add version suffix if available
          const versionedId = model.versionId ? `${modelId}@${model.versionId}` : modelId;
          models.push({
            id: versionedId,
            name: this.formatModelName(modelId),
            description: `Claude model on Vertex AI`,
          });
        }
      }

      return models.length > 0 ? models : this.getKnownModels();
    } catch {
      // Fall back to known models on error
      return this.getKnownModels();
    }
  }

  private formatModelName(modelId: string): string {
    // Convert model ID to human-readable name
    // e.g., "claude-3-5-sonnet" -> "Claude 3.5 Sonnet"
    return modelId
      .split('-')
      .map((part) => {
        if (part === 'claude') return 'Claude';
        if (/^\d+$/.test(part)) return part;
        return part.charAt(0).toUpperCase() + part.slice(1);
      })
      .join(' ')
      .replace(/(\d) (\d)/g, '$1.$2'); // "3 5" -> "3.5"
  }

  private getKnownModels(): ModelInfo[] {
    // Fallback known models when API is unavailable
    return [
      {
        id: 'claude-sonnet-4-5@20250929',
        name: 'Claude Sonnet 4.5',
        description: 'Latest Claude Sonnet model on Vertex AI',
      },
      {
        id: 'claude-haiku-4-5@20251001',
        name: 'Claude Haiku 4.5',
        description: 'Fast and efficient Claude model',
      },
      {
        id: 'claude-opus-4-1@20250805',
        name: 'Claude Opus 4.1',
        description: 'Most capable Claude model',
      },
    ];
  }
}
