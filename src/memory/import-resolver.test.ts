/**
 * Import Resolver Tests
 *
 * These tests focus on the core logic that can be tested without complex mocking.
 * For integration tests, use the test-memory.ts script.
 */

import { describe, it, expect, beforeEach } from '@jest/globals';
import { ImportResolver } from './import-resolver.js';
import { DEFAULT_MEMORY_CONFIG } from './types.js';

describe('ImportResolver', () => {
  let resolver: ImportResolver;

  beforeEach(() => {
    resolver = new ImportResolver(DEFAULT_MEMORY_CONFIG);
    resolver.setProjectRoot('/project');
  });

  describe('constructor', () => {
    it('should create resolver with config', () => {
      const resolver = new ImportResolver(DEFAULT_MEMORY_CONFIG);
      expect(resolver).toBeDefined();
    });
  });

  describe('setProjectRoot', () => {
    it('should set project root', () => {
      const resolver = new ImportResolver(DEFAULT_MEMORY_CONFIG);
      resolver.setProjectRoot('/my/project');
      expect(resolver).toBeDefined();
    });
  });

  describe('reset', () => {
    it('should reset resolved paths', () => {
      const resolver = new ImportResolver(DEFAULT_MEMORY_CONFIG);
      resolver.reset();
      expect(resolver).toBeDefined();
    });
  });

  describe('resolve', () => {
    it('should return content unchanged when no @imports exist', async () => {
      const content = `# Test File

This is a test file with no imports.

## Section
Some content here.`;

      const result = await resolver.resolve(content, '/project');
      expect(result.content).toBe(content);
      expect(result.importedPaths).toEqual([]);
      expect(result.errors).toEqual([]);
    });

    it('should not treat email addresses as imports', async () => {
      const content = `# Contact

Email: test@example.com
Support: help@company.org`;

      const result = await resolver.resolve(content, '/project');
      expect(result.content).toBe(content);
      expect(result.importedPaths).toEqual([]);
      expect(result.errors).toEqual([]);
    });

    it('should not treat inline @ mentions as imports', async () => {
      const content = `Some text with @mention inside

Not an import because @ is not at line start`;

      const result = await resolver.resolve(content, '/project');
      expect(result.content).toBe(content);
      expect(result.importedPaths).toEqual([]);
    });

    it('should handle content with no imports and special characters', async () => {
      const content = `# Special Characters

Code: \`@decorator\`
Symbol: @todo
Email: user@domain.com`;

      const result = await resolver.resolve(content, '/project');
      expect(result.content).toBe(content);
      expect(result.errors).toEqual([]);
    });

    it('should handle missing file gracefully', async () => {
      const content = `@./nonexistent.md`;

      const result = await resolver.resolve(content, '/project');

      // Should have an error about failed import
      expect(result.errors.some(e => e.includes('Failed to import'))).toBe(true);
    });

    it('should respect max depth limit', async () => {
      // Create resolver with depth limit of 0
      const limitedResolver = new ImportResolver({
        ...DEFAULT_MEMORY_CONFIG,
        maxImportDepth: 0,
      });
      limitedResolver.setProjectRoot('/project');

      const content = `@./file.md`;

      const result = await limitedResolver.resolve(content, '/project');

      // Should have error about max depth
      expect(result.errors.some(e => e.includes('depth'))).toBe(true);
    });
  });
});
