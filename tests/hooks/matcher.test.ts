/**
 * Tests for hook matcher functionality
 */

import { describe, it, expect } from '@jest/globals';
import { matchesTool, isValidRegex, testMatch } from '../../src/hooks/matcher.js';

describe('Matcher', () => {
  describe('matchesTool', () => {
    it('should match wildcard patterns', () => {
      expect(matchesTool(undefined, 'Write')).toBe(true);
      expect(matchesTool('', 'Write')).toBe(true);
      expect(matchesTool('*', 'Write')).toBe(true);
      expect(matchesTool(undefined, undefined)).toBe(true);
    });

    it('should match exact tool names', () => {
      expect(matchesTool('Write', 'Write')).toBe(true);
      expect(matchesTool('Read', 'Read')).toBe(true);
      expect(matchesTool('Write', 'Read')).toBe(false);
    });

    it('should match regex patterns', () => {
      expect(matchesTool('Write|Edit', 'Write')).toBe(true);
      expect(matchesTool('Write|Edit', 'Edit')).toBe(true);
      expect(matchesTool('Write|Edit', 'Read')).toBe(false);
      expect(matchesTool('Bash.*', 'Bash')).toBe(true);
      expect(matchesTool('Bash.*', 'BashTest')).toBe(true);
    });

    it('should handle invalid regex as exact match', () => {
      expect(matchesTool('[invalid', '[invalid')).toBe(true);
      expect(matchesTool('[invalid', 'other')).toBe(false);
    });

    it('should return false when toolName is undefined and pattern is specific', () => {
      expect(matchesTool('Write', undefined)).toBe(false);
      expect(matchesTool('Write|Edit', undefined)).toBe(false);
    });
  });

  describe('isValidRegex', () => {
    it('should validate correct regex patterns', () => {
      expect(isValidRegex('Write|Edit')).toBe(true);
      expect(isValidRegex('.*')).toBe(true);
      expect(isValidRegex('Bash(npm:*)')).toBe(true);
      expect(isValidRegex('^Write$')).toBe(true);
    });

    it('should reject invalid regex patterns', () => {
      expect(isValidRegex('[invalid')).toBe(false);
      expect(isValidRegex('(')).toBe(false);
      expect(isValidRegex('*')).toBe(false); // * alone is invalid regex
    });
  });

  describe('testMatch', () => {
    it('should test pattern matches', () => {
      expect(testMatch('Write|Edit', 'Write')).toBe(true);
      expect(testMatch('Write|Edit', 'Edit')).toBe(true);
      expect(testMatch('Write|Edit', 'Read')).toBe(false);
      expect(testMatch('*', 'Anything')).toBe(true);
    });
  });
});
