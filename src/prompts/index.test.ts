/**
 * Prompt System Tests
 *
 * Tests to ensure prompts are correctly loaded and contain key guidance.
 */

import { describe, it, expect } from '@jest/globals';
import {
  loadPrompt,
  loadSystemPrompt,
  buildSystemPrompt,
  buildSystemPromptWithMemory,
  buildSystemPromptForModel,
  mapProviderToPromptType,
  getPromptTypeForModel,
  formatEnvironmentInfo,
  getEnvironmentInfo,
  type ProviderType,
} from './index.js';

describe('Prompt Loader', () => {
  describe('loadPrompt', () => {
    it('should load base.txt from system category', () => {
      const content = loadPrompt('system', 'base');
      expect(content).toContain('GenCode');
      expect(content.length).toBeGreaterThan(100);
    });

    it('should load provider-specific prompts', () => {
      const providers: ProviderType[] = ['anthropic', 'openai', 'gemini', 'generic'];
      for (const provider of providers) {
        const content = loadPrompt('system', provider);
        expect(content.length).toBeGreaterThan(0);
      }
    });

    it('should load tool descriptions', () => {
      const tools = ['read', 'write', 'edit', 'bash', 'glob', 'grep'];
      for (const tool of tools) {
        const content = loadPrompt('tools', tool);
        expect(content.length).toBeGreaterThan(0);
      }
    });
  });

  describe('loadSystemPrompt', () => {
    it('should concatenate base + provider prompts', () => {
      const prompt = loadSystemPrompt('anthropic');

      // Should contain base content
      expect(prompt).toContain('GenCode');
      expect(prompt).toContain('# Tone and Style');

      // Should contain anthropic-specific content
      expect(prompt).toContain('Claude');
    });

    it('should work for all provider types', () => {
      const providers: ProviderType[] = ['anthropic', 'openai', 'gemini', 'generic'];
      for (const provider of providers) {
        const prompt = loadSystemPrompt(provider);
        expect(prompt).toContain('GenCode');
        expect(prompt.length).toBeGreaterThan(500);
      }
    });
  });

  describe('buildSystemPrompt', () => {
    it('should inject environment info', () => {
      const prompt = buildSystemPrompt('generic', '/tmp/test', true);

      expect(prompt).toContain('<env>');
      expect(prompt).toContain('Working directory: /tmp/test');
      expect(prompt).toContain('Is directory a git repo: Yes');
      expect(prompt).toContain('Platform:');
      expect(prompt).toContain("Today's date:");
      expect(prompt).toContain('</env>');
    });

    it('should replace {{ENVIRONMENT}} placeholder', () => {
      const prompt = buildSystemPrompt('generic', '/test');
      expect(prompt).not.toContain('{{ENVIRONMENT}}');
    });
  });

  describe('buildSystemPromptWithMemory', () => {
    it('should append memory context when provided', () => {
      const memoryContext = '# Project Rules\n- Use TypeScript';
      const prompt = buildSystemPromptWithMemory('generic', '/test', false, memoryContext);

      expect(prompt).toContain('<claudeMd>');
      expect(prompt).toContain('# Project Rules');
      expect(prompt).toContain('Use TypeScript');
      expect(prompt).toContain('</claudeMd>');
    });

    it('should not add memory section when context is empty', () => {
      const prompt = buildSystemPromptWithMemory('generic', '/test', false);
      expect(prompt).not.toContain('<claudeMd>');
    });
  });

  describe('mapProviderToPromptType', () => {
    it('should map known providers correctly', () => {
      expect(mapProviderToPromptType('anthropic')).toBe('anthropic');
      expect(mapProviderToPromptType('openai')).toBe('openai');
      // Google provider uses Gemini models, so it maps to gemini prompt
      expect(mapProviderToPromptType('google')).toBe('gemini');
    });

    it('should return generic for unknown providers', () => {
      expect(mapProviderToPromptType('unknown')).toBe('generic');
      expect(mapProviderToPromptType('ollama')).toBe('generic');
    });
  });

  describe('getPromptTypeForModel', () => {
    it('should use fallback provider when model lookup fails', () => {
      // Unknown model with fallback
      expect(getPromptTypeForModel('unknown-model', 'anthropic')).toBe('anthropic');
      expect(getPromptTypeForModel('unknown-model', 'openai')).toBe('openai');
    });

    it('should return generic when no fallback provided', () => {
      expect(getPromptTypeForModel('unknown-model')).toBe('generic');
    });
  });

  describe('buildSystemPromptForModel', () => {
    it('should build prompt using fallback provider', () => {
      const prompt = buildSystemPromptForModel(
        'unknown-model',
        '/test',
        false,
        undefined,
        'anthropic'
      );

      expect(prompt).toContain('GenCode');
      expect(prompt).toContain('Claude');
    });
  });

  describe('Environment Info', () => {
    it('should generate correct environment info', () => {
      const env = getEnvironmentInfo('/my/project', true);

      expect(env.cwd).toBe('/my/project');
      expect(env.isGitRepo).toBe(true);
      expect(env.platform).toBe(process.platform);
      expect(env.date).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    });

    it('should format environment info correctly', () => {
      const env = getEnvironmentInfo('/test', false);
      const formatted = formatEnvironmentInfo(env);

      expect(formatted).toContain('<env>');
      expect(formatted).toContain('Working directory: /test');
      expect(formatted).toContain('Is directory a git repo: No');
      expect(formatted).toContain('</env>');
    });
  });
});

describe('Prompt Content Validation', () => {
  describe('base.txt - Claude Code key guidance', () => {
    let basePrompt: string;

    beforeAll(() => {
      basePrompt = loadPrompt('system', 'base');
    });

    it('should contain token minimization guidance', () => {
      expect(basePrompt).toMatch(/minimize output tokens/i);
    });

    it('should contain CommonMark rendering info', () => {
      expect(basePrompt).toMatch(/CommonMark/i);
    });

    it('should contain forbidden preamble/postamble phrases', () => {
      expect(basePrompt).toContain('The answer is');
      expect(basePrompt).toContain('Here is the content');
    });

    it('should contain conciseness guidance', () => {
      expect(basePrompt).toMatch(/concise|fewer than 4 lines/i);
    });

    it('should contain non-preachy refusal guidance', () => {
      expect(basePrompt).toMatch(/preachy|annoying/i);
    });

    it('should contain examples', () => {
      expect(basePrompt).toContain('<example>');
      expect(basePrompt).toContain('</example>');
      // Verify at least one simple example
      expect(basePrompt).toMatch(/2 \+ 2[\s\S]*?4/);
    });

    it('should contain task management section', () => {
      expect(basePrompt).toContain('TodoWrite');
    });

    it('should contain tool usage policy', () => {
      expect(basePrompt).toContain('Tool Usage');
    });

    it('should contain environment placeholder', () => {
      expect(basePrompt).toContain('{{ENVIRONMENT}}');
    });
  });

  describe('generic.txt - standalone comprehensive prompt', () => {
    let genericPrompt: string;

    beforeAll(() => {
      genericPrompt = loadPrompt('system', 'generic');
    });

    it('should contain token minimization guidance', () => {
      expect(genericPrompt).toMatch(/minimize output tokens/i);
    });

    it('should contain tool selection guidelines table', () => {
      expect(genericPrompt).toContain('| Task | Tool |');
    });

    it('should contain software engineering workflow', () => {
      expect(genericPrompt).toContain('Software Engineering Workflow');
    });

    it('should contain security guidance', () => {
      expect(genericPrompt).toMatch(/security|secrets/i);
    });

    it('should contain examples', () => {
      expect(genericPrompt).toContain('<example>');
      const exampleCount = (genericPrompt.match(/<example>/g) || []).length;
      expect(exampleCount).toBeGreaterThanOrEqual(3);
    });
  });

  describe('Provider-specific prompts', () => {
    it('anthropic.txt should reference Claude capabilities', () => {
      const prompt = loadPrompt('system', 'anthropic');
      expect(prompt).toMatch(/claude|thinking|anthropic/i);
    });

    it('openai.txt should reference GPT capabilities', () => {
      const prompt = loadPrompt('system', 'openai');
      expect(prompt).toMatch(/gpt|openai|structured/i);
    });

    it('gemini.txt should reference Gemini capabilities', () => {
      const prompt = loadPrompt('system', 'gemini');
      expect(prompt).toMatch(/gemini|google|multimodal/i);
    });
  });
});

describe('Tool Description Validation', () => {
  const requiredTools = ['read', 'write', 'edit', 'bash', 'glob', 'grep', 'todowrite'];

  it.each(requiredTools)('should have description for %s tool', (tool) => {
    const description = loadPrompt('tools', tool);
    expect(description.length).toBeGreaterThan(50);
  });

  it('bash.txt should contain git safety guidance', () => {
    const bash = loadPrompt('tools', 'bash');
    expect(bash).toMatch(/git|commit/i);
  });

  it('todowrite.txt should contain task state guidance', () => {
    const todo = loadPrompt('tools', 'todowrite');
    expect(todo).toMatch(/pending|in_progress|completed/i);
  });
});
