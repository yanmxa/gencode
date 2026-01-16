/**
 * Config Merger Tests
 */

import { describe, it, expect } from '@jest/globals';
import {
  deepMerge,
  mergeSettings,
  extractManagedDeny,
  applyManagedRestrictions,
  mergeAllSources,
  mergeWithCliArgs,
  createMergeSummary,
} from './merger.js';
import type { ConfigSource, Settings } from './types.js';

describe('deepMerge', () => {
  it('should merge simple objects', () => {
    const base = { a: 1, b: 2 };
    const override = { b: 3, c: 4 };
    const result = deepMerge(base, override);

    expect(result).toEqual({ a: 1, b: 3, c: 4 });
  });

  it('should deep merge nested objects', () => {
    const base = {
      provider: 'openai',
      permissions: {
        allow: ['Bash(git:*)'],
        deny: ['WebFetch'],
      },
    };
    const override = {
      model: 'claude-sonnet',
      permissions: {
        allow: ['Bash(npm:*)'],
      },
    };
    const result = deepMerge(base, override);

    expect(result.provider).toBe('openai');
    expect(result.model).toBe('claude-sonnet');
    expect(result.permissions.allow).toContain('Bash(git:*)');
    expect(result.permissions.allow).toContain('Bash(npm:*)');
    expect(result.permissions.deny).toContain('WebFetch');
  });

  it('should concatenate and deduplicate arrays', () => {
    const base = { items: ['a', 'b', 'c'] };
    const override = { items: ['b', 'c', 'd'] };
    const result = deepMerge(base, override);

    expect(result.items).toEqual(['a', 'b', 'c', 'd']);
  });

  it('should override scalar values', () => {
    const base = { model: 'gpt-4', theme: 'dark' };
    const override = { model: 'claude-sonnet' };
    const result = deepMerge(base, override);

    expect(result.model).toBe('claude-sonnet');
    expect(result.theme).toBe('dark');
  });

  it('should handle undefined values', () => {
    const base = { a: 1, b: 2 };
    const override = { a: undefined, c: 3 };
    const result = deepMerge(base, override as any);

    expect(result.a).toBe(1); // undefined should not override
    expect(result.c).toBe(3);
  });

  it('should handle empty objects', () => {
    const base = { a: 1 };
    const override = {};
    const result = deepMerge(base, override);

    expect(result).toEqual({ a: 1 });
  });
});

describe('mergeSettings', () => {
  it('should merge multiple sources in order', () => {
    const sources: ConfigSource[] = [
      {
        level: 'user',
        path: '~/.claude/settings.json',
        namespace: 'claude',
        settings: { provider: 'openai', model: 'gpt-4' },
      },
      {
        level: 'user',
        path: '~/.gencode/settings.json',
        namespace: 'gencode',
        settings: { provider: 'anthropic' }, // Override provider
      },
      {
        level: 'project',
        path: '.gencode/settings.json',
        namespace: 'gencode',
        settings: { model: 'claude-sonnet' }, // Override model
      },
    ];

    const result = mergeSettings(sources);

    expect(result.provider).toBe('anthropic'); // From gencode user
    expect(result.model).toBe('claude-sonnet'); // From project
  });

  it('should concatenate permission arrays', () => {
    const sources: ConfigSource[] = [
      {
        level: 'user',
        path: '~/.claude/settings.json',
        namespace: 'claude',
        settings: {
          permissions: { allow: ['Bash(git:*)'] },
        },
      },
      {
        level: 'project',
        path: '.gencode/settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { allow: ['Bash(npm:*)'], deny: ['WebFetch'] },
        },
      },
    ];

    const result = mergeSettings(sources);

    expect(result.permissions?.allow).toContain('Bash(git:*)');
    expect(result.permissions?.allow).toContain('Bash(npm:*)');
    expect(result.permissions?.deny).toContain('WebFetch');
  });

  it('should return empty object for empty sources', () => {
    const result = mergeSettings([]);
    expect(result).toEqual({});
  });
});

describe('extractManagedDeny', () => {
  it('should extract deny rules from managed sources', () => {
    const sources: ConfigSource[] = [
      {
        level: 'user',
        path: '~/.gencode/settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { deny: ['WebFetch'] },
        },
      },
      {
        level: 'managed',
        path: '/etc/gencode/managed-settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { deny: ['Bash(curl:*)'] },
        },
      },
    ];

    const result = extractManagedDeny(sources);

    // Should only include managed deny rules
    expect(result).toContain('Bash(curl:*)');
    expect(result).not.toContain('WebFetch');
  });

  it('should deduplicate managed deny rules', () => {
    const sources: ConfigSource[] = [
      {
        level: 'managed',
        path: '/Library/.../ClaudeCode/managed-settings.json',
        namespace: 'claude',
        settings: {
          permissions: { deny: ['Bash(curl:*)'] },
        },
      },
      {
        level: 'managed',
        path: '/Library/.../GenCode/managed-settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { deny: ['Bash(curl:*)', 'WebFetch'] },
        },
      },
    ];

    const result = extractManagedDeny(sources);

    expect(result).toEqual(['Bash(curl:*)', 'WebFetch']);
  });

  it('should return empty array if no managed sources', () => {
    const sources: ConfigSource[] = [
      {
        level: 'user',
        path: '~/.gencode/settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { deny: ['WebFetch'] },
        },
      },
    ];

    const result = extractManagedDeny(sources);
    expect(result).toEqual([]);
  });
});

describe('applyManagedRestrictions', () => {
  it('should add managed deny rules to deny list', () => {
    const settings: Settings = {
      permissions: {
        allow: ['Bash(git:*)'],
        deny: ['WebFetch'],
      },
    };
    const managedDeny = ['Bash(curl:*)'];

    const result = applyManagedRestrictions(settings, managedDeny);

    expect(result.permissions?.deny).toContain('WebFetch');
    expect(result.permissions?.deny).toContain('Bash(curl:*)');
  });

  it('should remove managed deny from allow list', () => {
    const settings: Settings = {
      permissions: {
        allow: ['Bash(git:*)', 'Bash(curl:*)'], // curl should be removed
        deny: [],
      },
    };
    const managedDeny = ['Bash(curl:*)'];

    const result = applyManagedRestrictions(settings, managedDeny);

    expect(result.permissions?.allow).toContain('Bash(git:*)');
    expect(result.permissions?.allow).not.toContain('Bash(curl:*)');
    expect(result.permissions?.deny).toContain('Bash(curl:*)');
  });

  it('should return unchanged settings if no managed deny', () => {
    const settings: Settings = {
      provider: 'anthropic',
      permissions: { allow: ['Bash(git:*)'] },
    };

    const result = applyManagedRestrictions(settings, []);

    expect(result).toEqual(settings);
  });
});

describe('mergeAllSources', () => {
  it('should merge all sources and extract managed deny', () => {
    const sources: ConfigSource[] = [
      {
        level: 'user',
        path: '~/.gencode/settings.json',
        namespace: 'gencode',
        settings: { provider: 'anthropic' },
      },
      {
        level: 'managed',
        path: '/etc/gencode/managed-settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { deny: ['Bash(curl:*)'] },
        },
      },
    ];

    const result = mergeAllSources(sources);

    expect(result.settings.provider).toBe('anthropic');
    expect(result.managedDeny).toContain('Bash(curl:*)');
    expect(result.sources).toHaveLength(2);
  });
});

describe('mergeWithCliArgs', () => {
  it('should merge CLI args with highest priority', () => {
    const sources: ConfigSource[] = [
      {
        level: 'user',
        path: '~/.gencode/settings.json',
        namespace: 'gencode',
        settings: { provider: 'openai', model: 'gpt-4' },
      },
    ];

    const merged = mergeAllSources(sources);
    const result = mergeWithCliArgs(merged, { model: 'claude-sonnet' });

    expect(result.settings.provider).toBe('openai');
    expect(result.settings.model).toBe('claude-sonnet');
    expect(result.sources).toHaveLength(2);
    expect(result.sources[1].level).toBe('cli');
  });

  it('should not allow CLI args to override managed deny', () => {
    const sources: ConfigSource[] = [
      {
        level: 'managed',
        path: '/etc/gencode/managed-settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { deny: ['Bash(curl:*)'] },
        },
      },
    ];

    const merged = mergeAllSources(sources);
    // Try to add curl to allow list via CLI
    const result = mergeWithCliArgs(merged, {
      permissions: { allow: ['Bash(curl:*)'] },
    });

    // curl should still be in deny, not in allow
    expect(result.settings.permissions?.deny).toContain('Bash(curl:*)');
    expect(result.settings.permissions?.allow).not.toContain('Bash(curl:*)');
  });
});

describe('createMergeSummary', () => {
  it('should create readable summary', () => {
    const sources: ConfigSource[] = [
      {
        level: 'user',
        path: '~/.gencode/settings.json',
        namespace: 'gencode',
        settings: { provider: 'anthropic' },
      },
      {
        level: 'managed',
        path: '/etc/gencode/managed-settings.json',
        namespace: 'gencode',
        settings: {
          permissions: { deny: ['Bash(curl:*)'] },
        },
      },
    ];

    const merged = mergeAllSources(sources);
    const summary = createMergeSummary(merged);

    expect(summary).toContain('Configuration Sources');
    expect(summary).toContain('user:gencode');
    expect(summary).toContain('managed:gencode');
    expect(summary).toContain('[enforced]');
    expect(summary).toContain('Managed Deny Rules');
    expect(summary).toContain('Bash(curl:*)');
  });
});
