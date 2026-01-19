/**
 * Skills Parser Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { parseSkillFile } from './parser.js';

describe('parseSkillFile', () => {
  let tempDir: string;

  beforeEach(async () => {
    tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'skills-test-'));
  });

  afterEach(async () => {
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  it('should parse valid SKILL.md with all fields', async () => {
    const skillPath = path.join(tempDir, 'SKILL.md');
    await fs.writeFile(
      skillPath,
      `---
name: test-skill
description: A test skill
allowed-tools: [Read, Write, Bash]
version: 1.0.0
author: Test Author
tags: [test, demo]
---

# Test Skill Content

This is the skill body content.
`
    );

    const skill = await parseSkillFile(skillPath, 'user', 'gen');

    expect(skill.name).toBe('test-skill');
    expect(skill.description).toBe('A test skill');
    expect(skill.allowedTools).toEqual(['Read', 'Write', 'Bash']);
    expect(skill.version).toBe('1.0.0');
    expect(skill.author).toBe('Test Author');
    expect(skill.tags).toEqual(['test', 'demo']);
    expect(skill.content).toBe('# Test Skill Content\n\nThis is the skill body content.');
    expect(skill.source.path).toBe(skillPath);
    expect(skill.directory).toBe(tempDir);
    expect(skill.source.level).toBe('user');
    expect(skill.source.namespace).toBe('gen');
  });

  it('should parse minimal SKILL.md with only required fields', async () => {
    const skillPath = path.join(tempDir, 'SKILL.md');
    await fs.writeFile(
      skillPath,
      `---
name: minimal-skill
description: Minimal test skill
---

Skill content here.
`
    );

    const skill = await parseSkillFile(skillPath, 'project', 'claude');

    expect(skill.name).toBe('minimal-skill');
    expect(skill.description).toBe('Minimal test skill');
    expect(skill.allowedTools).toBeUndefined();
    expect(skill.version).toBeUndefined();
    expect(skill.author).toBeUndefined();
    expect(skill.tags).toBeUndefined();
    expect(skill.content).toBe('Skill content here.');
    expect(skill.source.level).toBe('project');
    expect(skill.source.namespace).toBe('claude');
  });

  it('should throw error when name is missing', async () => {
    const skillPath = path.join(tempDir, 'SKILL.md');
    await fs.writeFile(
      skillPath,
      `---
description: Missing name
---

Content
`
    );

    await expect(parseSkillFile(skillPath, 'user', 'gen')).rejects.toThrow(
      /Invalid SKILL.md frontmatter: name: Invalid input: expected string, received undefined/
    );
  });

  it('should throw error when description is missing', async () => {
    const skillPath = path.join(tempDir, 'SKILL.md');
    await fs.writeFile(
      skillPath,
      `---
name: test-skill
---

Content
`
    );

    await expect(parseSkillFile(skillPath, 'user', 'gen')).rejects.toThrow(
      /Invalid SKILL.md frontmatter: description: Invalid input: expected string, received undefined/
    );
  });

  it('should throw error when file does not exist', async () => {
    const skillPath = path.join(tempDir, 'nonexistent.md');

    await expect(parseSkillFile(skillPath, 'user', 'gen')).rejects.toThrow();
  });

  it('should handle empty content body', async () => {
    const skillPath = path.join(tempDir, 'SKILL.md');
    await fs.writeFile(
      skillPath,
      `---
name: empty-skill
description: Skill with no body
---
`
    );

    const skill = await parseSkillFile(skillPath, 'user', 'gen');

    expect(skill.name).toBe('empty-skill');
    expect(skill.description).toBe('Skill with no body');
    expect(skill.content).toBe('');
  });

  it('should trim whitespace from content', async () => {
    const skillPath = path.join(tempDir, 'SKILL.md');
    await fs.writeFile(
      skillPath,
      `---
name: whitespace-skill
description: Test whitespace trimming
---

  Content with whitespace

`
    );

    const skill = await parseSkillFile(skillPath, 'user', 'gen');

    expect(skill.content).toBe('Content with whitespace');
  });

  it('should handle malformed YAML gracefully', async () => {
    const skillPath = path.join(tempDir, 'SKILL.md');
    await fs.writeFile(
      skillPath,
      `---
name: bad-yaml
description: { invalid yaml here
---

Content
`
    );

    await expect(parseSkillFile(skillPath, 'user', 'gen')).rejects.toThrow();
  });
});
