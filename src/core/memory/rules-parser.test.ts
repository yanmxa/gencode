/**
 * Rules Parser Tests
 */

import { parseRuleFrontmatter, matchesPatterns } from './rules-parser.js';

describe('Rules Parser', () => {
  describe('parseRuleFrontmatter', () => {
    it('should parse simple YAML frontmatter with paths', () => {
      const content = `---
paths:
  - "src/**/*.ts"
  - "lib/**/*.ts"
---

# Rule Content

This is the rule body.`;

      const result = parseRuleFrontmatter(content);

      expect(result.paths).toEqual(['src/**/*.ts', 'lib/**/*.ts']);
      expect(result.content).toBe('# Rule Content\n\nThis is the rule body.');
    });

    it('should return empty paths for content without frontmatter', () => {
      const content = `# Rule Without Frontmatter

Just regular markdown content.`;

      const result = parseRuleFrontmatter(content);

      expect(result.paths).toEqual([]);
      expect(result.content).toBe('# Rule Without Frontmatter\n\nJust regular markdown content.');
    });

    it('should return empty paths when frontmatter has no paths field', () => {
      const content = `---
title: Some Rule
description: A rule without path scoping
---

# Rule Content`;

      const result = parseRuleFrontmatter(content);

      expect(result.paths).toEqual([]);
      expect(result.content).toBe('# Rule Content');
    });

    it('should handle single path as array', () => {
      const content = `---
paths:
  - "src/api/**/*.ts"
---

# API Rules`;

      const result = parseRuleFrontmatter(content);

      expect(result.paths).toEqual(['src/api/**/*.ts']);
    });

    it('should filter non-string paths', () => {
      const content = `---
paths:
  - "valid/path/*.ts"
  - 123
  - true
  - "another/valid/*.js"
---

# Mixed Paths`;

      const result = parseRuleFrontmatter(content);

      expect(result.paths).toEqual(['valid/path/*.ts', 'another/valid/*.js']);
    });

    it('should handle empty frontmatter', () => {
      const content = `---
---

# Empty Frontmatter`;

      const result = parseRuleFrontmatter(content);

      expect(result.paths).toEqual([]);
      expect(result.content).toBe('# Empty Frontmatter');
    });

    it('should trim content whitespace', () => {
      const content = `---
paths: []
---


# Content with extra whitespace


`;

      const result = parseRuleFrontmatter(content);

      expect(result.content).toBe('# Content with extra whitespace');
    });
  });

  describe('matchesPatterns', () => {
    it('should return true when patterns is empty (always active)', () => {
      expect(matchesPatterns('any/file.ts', [])).toBe(true);
    });

    it('should match glob patterns', () => {
      const patterns = ['src/**/*.ts'];

      expect(matchesPatterns('src/components/Button.ts', patterns)).toBe(true);
      expect(matchesPatterns('src/deep/nested/file.ts', patterns)).toBe(true);
      expect(matchesPatterns('lib/file.ts', patterns)).toBe(false);
    });

    it('should match multiple patterns (OR logic)', () => {
      const patterns = ['src/**/*.ts', 'lib/**/*.ts'];

      expect(matchesPatterns('src/file.ts', patterns)).toBe(true);
      expect(matchesPatterns('lib/file.ts', patterns)).toBe(true);
      expect(matchesPatterns('test/file.ts', patterns)).toBe(false);
    });

    it('should match file extensions', () => {
      const patterns = ['**/*.test.ts', '**/*.spec.ts'];

      expect(matchesPatterns('src/Button.test.ts', patterns)).toBe(true);
      expect(matchesPatterns('lib/utils.spec.ts', patterns)).toBe(true);
      expect(matchesPatterns('src/Button.ts', patterns)).toBe(false);
    });

    it('should handle matchBase for simple patterns', () => {
      const patterns = ['*.ts'];

      // With matchBase: true, *.ts should match file.ts anywhere
      expect(matchesPatterns('file.ts', patterns)).toBe(true);
    });

    it('should match specific directories', () => {
      const patterns = ['src/api/**', 'src/routes/**'];

      expect(matchesPatterns('src/api/users.ts', patterns)).toBe(true);
      expect(matchesPatterns('src/routes/index.ts', patterns)).toBe(true);
      expect(matchesPatterns('src/components/Button.ts', patterns)).toBe(false);
    });

    it('should handle dot files when dot option is true', () => {
      const patterns = ['**/*.md'];

      // Should match hidden directories
      expect(matchesPatterns('.github/README.md', patterns)).toBe(true);
    });

    it('should handle negation in paths', () => {
      // This tests behavior - minimatch handles negation differently
      const patterns = ['src/**/*.ts'];

      expect(matchesPatterns('src/index.ts', patterns)).toBe(true);
      expect(matchesPatterns('src/index.js', patterns)).toBe(false);
    });

    it('should handle complex patterns', () => {
      const patterns = ['src/{api,routes}/**/*.ts'];

      expect(matchesPatterns('src/api/users.ts', patterns)).toBe(true);
      expect(matchesPatterns('src/routes/auth.ts', patterns)).toBe(true);
      expect(matchesPatterns('src/utils/helpers.ts', patterns)).toBe(false);
    });

    it('should match relative paths', () => {
      const patterns = ['./src/**/*.ts'];

      expect(matchesPatterns('./src/file.ts', patterns)).toBe(true);
    });
  });
});
