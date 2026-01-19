/**
 * Skill Tool Tests
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { createSkillTool } from '../../core/tools/factories/skill-tool-factory.js';
import type { ToolContext } from '../../core/tools/types.js';

describe('createSkillTool', () => {
  let tempDir: string;
  let mockContext: ToolContext;

  beforeEach(async () => {
    tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'skill-tool-test-'));

    mockContext = {
      cwd: tempDir,
      abortSignal: new AbortController().signal,
    };
  });

  afterEach(async () => {
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  // Helper to create skill
  async function createSkill(namespace: string, name: string, description: string) {
    const skillsDir = path.join(tempDir, `.${namespace}`, 'skills', name);
    await fs.mkdir(skillsDir, { recursive: true });
    await fs.writeFile(
      path.join(skillsDir, 'SKILL.md'),
      `---
name: ${name}
description: ${description}
---

# ${name} Skill

This is the ${name} skill content.
`
    );
  }

  it('should create tool with valid structure', async () => {
    const tool = await createSkillTool(tempDir, { projectOnly: true });

    expect(tool.name).toBe('Skill');
    expect(tool.description).toContain('Execute a skill');
    expect(tool.description).toContain('Available Skills');
    expect(tool.parameters).toBeDefined();
  });

  it('should create tool listing available skills', async () => {
    await createSkill('gen', 'skill1', 'First skill');
    await createSkill('gen', 'skill2', 'Second skill');

    const tool = await createSkillTool(tempDir, { projectOnly: true });

    expect(tool.description).toContain('skill1');
    expect(tool.description).toContain('First skill');
    expect(tool.description).toContain('skill2');
    expect(tool.description).toContain('Second skill');
  });

  it('should execute skill and return formatted content', async () => {
    await createSkill('gen', 'test-skill', 'Test skill description');

    const tool = await createSkillTool(tempDir, { projectOnly: true });
    const result = await tool.execute({ skill: 'test-skill' }, mockContext);

    expect(result.success).toBe(true);
    expect(result.output).toContain('Skill Activated');
    expect(result.output).toContain('test-skill');
    expect(result.output).toContain('Test skill description');
    expect(result.output).toContain('This is the test-skill skill content');
    expect(result.metadata?.title).toBe('Skill: test-skill');
    expect(result.metadata?.subtitle).toBe('Test skill description');
  });

  it('should execute skill with arguments', async () => {
    await createSkill('gen', 'test-skill', 'Test skill');

    const tool = await createSkillTool(tempDir, { projectOnly: true });
    const result = await tool.execute(
      { skill: 'test-skill', args: '--flag value' },
      mockContext
    );

    expect(result.success).toBe(true);
    expect(result.output).toContain('Arguments: --flag value');
  });

  it('should return error for non-existent skill', async () => {
    const tool = await createSkillTool(tempDir, { projectOnly: true });
    const result = await tool.execute({ skill: 'nonexistent' }, mockContext);

    expect(result.success).toBe(false);
    expect(result.error).toContain('Skill not found: nonexistent');
    expect(result.error).toContain('Available skills:');
  });

  it('should respect merge priority (gen over claude)', async () => {
    await createSkill('claude', 'test-skill', 'Claude version');
    await createSkill('gen', 'test-skill', 'Gen version (should win)');

    const tool = await createSkillTool(tempDir, { projectOnly: true });
    const result = await tool.execute({ skill: 'test-skill' }, mockContext);

    expect(result.success).toBe(true);
    expect(result.output).toContain('Gen version (should win)');
    expect(result.output).toContain('Source: project/gen');
  });

  it('should include source information in output', async () => {
    await createSkill('claude', 'test-skill', 'Test skill');

    const tool = await createSkillTool(tempDir, { projectOnly: true });
    const result = await tool.execute({ skill: 'test-skill' }, mockContext);

    expect(result.success).toBe(true);
    expect(result.output).toContain('Source: project/claude');
  });

  it('should handle skills with special characters in content', async () => {
    const skillsDir = path.join(tempDir, '.gen', 'skills', 'special-skill');
    await fs.mkdir(skillsDir, { recursive: true });
    await fs.writeFile(
      path.join(skillsDir, 'SKILL.md'),
      `---
name: special-skill
description: Skill with special chars
---

Content with **markdown**, \`code\`, and [links](http://example.com).
Special chars: & < > " ' / \\
`
    );

    const tool = await createSkillTool(tempDir, { projectOnly: true });
    const result = await tool.execute({ skill: 'special-skill' }, mockContext);

    expect(result.success).toBe(true);
    expect(result.output).toContain('**markdown**');
    expect(result.output).toContain('`code`');
    expect(result.output).toContain('[links](http://example.com)');
  });

  it('should validate parameters with Zod', async () => {
    const tool = await createSkillTool(tempDir, { projectOnly: true });

    // Valid parameters
    const validResult = tool.parameters.safeParse({ skill: 'test-skill' });
    expect(validResult.success).toBe(true);

    // Valid with args
    const validWithArgsResult = tool.parameters.safeParse({
      skill: 'test-skill',
      args: 'optional',
    });
    expect(validWithArgsResult.success).toBe(true);

    // Invalid - missing skill
    const invalidResult = tool.parameters.safeParse({});
    expect(invalidResult.success).toBe(false);

    // Invalid - wrong type
    const invalidTypeResult = tool.parameters.safeParse({ skill: 123 });
    expect(invalidTypeResult.success).toBe(false);
  });
});
