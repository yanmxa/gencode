/**
 * Skills Discovery Tests
 *
 * Updated to use public API (discover) instead of private methods.
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import * as os from 'os';
import { SkillDiscovery } from './discovery.js';

describe('SkillDiscovery', () => {
  let tempDir: string;
  let discovery: SkillDiscovery;

  beforeEach(async () => {
    tempDir = await fs.mkdtemp(path.join(os.tmpdir(), 'skills-discovery-test-'));
    // Use projectOnly mode to avoid loading user's real skills in tests
    discovery = new SkillDiscovery({ projectOnly: true });
  });

  afterEach(async () => {
    await fs.rm(tempDir, { recursive: true, force: true });
  });

  /**
   * Helper to create skill file in the proper directory structure
   * @param level - 'user' or 'project'
   * @param namespace - 'gen' or 'claude'
   * @param name - Skill name (directory name)
   * @param description - Skill description
   */
  async function createSkill(
    level: 'user' | 'project',
    namespace: 'gen' | 'claude',
    name: string,
    description: string
  ) {
    const baseDir = level === 'user' ? os.homedir() : tempDir;
    const namespaceDir = namespace === 'gen' ? '.gen' : '.claude';
    const skillsDir = path.join(baseDir, namespaceDir, 'skills');
    const skillDir = path.join(skillsDir, name);

    await fs.mkdir(skillDir, { recursive: true });
    await fs.writeFile(
      path.join(skillDir, 'SKILL.md'),
      `---
name: ${name}
description: ${description}
---

# ${name} Content
`
    );
  }

  /**
   * Helper to create skill in project directory structure
   * Used for most tests to avoid modifying user's home directory
   */
  async function createProjectSkill(
    namespace: 'gen' | 'claude',
    name: string,
    description: string
  ) {
    return createSkill('project', namespace, name, description);
  }

  it('should discover skills from project .gen directory', async () => {
    await createProjectSkill('gen', 'skill1', 'First skill');
    await createProjectSkill('gen', 'skill2', 'Second skill');

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(2);
    expect(discovery.has('skill1')).toBe(true);
    expect(discovery.has('skill2')).toBe(true);
  });

  it('should handle merge priority correctly (gen > claude)', async () => {
    // Create same-name skill in both namespaces
    await createProjectSkill('claude', 'test-skill', 'Claude version');
    await createProjectSkill('gen', 'test-skill', 'Gen version (should win)');

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(1);
    const skill = discovery.get('test-skill');
    expect(skill).toBeDefined();
    expect(skill?.description).toBe('Gen version (should win)');
    expect(skill?.source.namespace).toBe('gen');
  });

  it('should keep different-name skills from all directories', async () => {
    await createProjectSkill('claude', 'skill-a', 'Skill A from claude');
    await createProjectSkill('gen', 'skill-b', 'Skill B from gen');

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(2);
    expect(discovery.has('skill-a')).toBe(true);
    expect(discovery.has('skill-b')).toBe(true);
  });

  it('should handle empty directories gracefully', async () => {
    // Create empty .gen/skills directory
    const emptyDir = path.join(tempDir, '.gen', 'skills');
    await fs.mkdir(emptyDir, { recursive: true });

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(0);
  });

  it('should handle non-existent directories gracefully', async () => {
    // Don't create any directories at all
    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(0);
  });

  it('should skip directories without SKILL.md', async () => {
    const skillsDir = path.join(tempDir, '.gen', 'skills');
    await fs.mkdir(path.join(skillsDir, 'not-a-skill'), { recursive: true });
    await fs.writeFile(path.join(skillsDir, 'not-a-skill', 'README.md'), 'Not a skill');

    await createProjectSkill('gen', 'real-skill', 'Real skill');

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(1);
    expect(discovery.has('real-skill')).toBe(true);
    expect(discovery.has('not-a-skill')).toBe(false);
  });

  it('should skip files (not directories) in skills directory', async () => {
    const skillsDir = path.join(tempDir, '.gen', 'skills');
    await fs.mkdir(skillsDir, { recursive: true });
    await fs.writeFile(path.join(skillsDir, 'README.md'), 'Readme file');

    await createProjectSkill('gen', 'valid-skill', 'Valid skill');

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(1);
    expect(discovery.has('valid-skill')).toBe(true);
  });

  it('should getAll() return all skills', async () => {
    await createProjectSkill('gen', 'skill1', 'First');
    await createProjectSkill('gen', 'skill2', 'Second');
    await createProjectSkill('gen', 'skill3', 'Third');

    await discovery.discover(tempDir);

    const allSkills = discovery.getAll();
    expect(allSkills).toHaveLength(3);
    expect(allSkills.map((s) => s.name)).toContain('skill1');
    expect(allSkills.map((s) => s.name)).toContain('skill2');
    expect(allSkills.map((s) => s.name)).toContain('skill3');
  });

  it('should get() return undefined for non-existent skill', async () => {
    await discovery.discover(tempDir);

    const skill = discovery.get('nonexistent');
    expect(skill).toBeUndefined();
  });

  it('should handle invalid SKILL.md files gracefully', async () => {
    const skillsDir = path.join(tempDir, '.gen', 'skills');
    const invalidSkillDir = path.join(skillsDir, 'invalid-skill');
    await fs.mkdir(invalidSkillDir, { recursive: true });

    // Create SKILL.md without required fields
    await fs.writeFile(
      path.join(invalidSkillDir, 'SKILL.md'),
      `---
invalid: true
---
`
    );

    // Create valid skill
    await createProjectSkill('gen', 'valid-skill', 'Valid skill');

    // Should skip invalid and load valid
    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(1);
    expect(discovery.has('valid-skill')).toBe(true);
    expect(discovery.has('invalid-skill')).toBe(false);
  });

  it('should track level and namespace correctly (project/claude)', async () => {
    await createProjectSkill('claude', 'test-skill', 'Test');

    await discovery.discover(tempDir);

    const skill = discovery.get('test-skill');
    expect(skill?.source.level).toBe('project');
    expect(skill?.source.namespace).toBe('claude');
  });

  it('should track level and namespace correctly (project/gen)', async () => {
    await createProjectSkill('gen', 'test-skill', 'Test');

    await discovery.discover(tempDir);

    const skill = discovery.get('test-skill');
    expect(skill?.source.level).toBe('project');
    expect(skill?.source.namespace).toBe('gen');
  });

  it('should store full path information', async () => {
    await createProjectSkill('gen', 'test-skill', 'Test');

    await discovery.discover(tempDir);

    const skill = discovery.get('test-skill');
    expect(skill?.source.path).toContain('SKILL.md');
    expect(skill?.directory).toBe(path.join(tempDir, '.gen', 'skills', 'test-skill'));
  });

  it('should support both .gen and .claude namespaces', async () => {
    await createProjectSkill('claude', 'claude-skill', 'From Claude');
    await createProjectSkill('gen', 'gen-skill', 'From Gen');

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(2);

    const claudeSkill = discovery.get('claude-skill');
    expect(claudeSkill?.source.namespace).toBe('claude');

    const genSkill = discovery.get('gen-skill');
    expect(genSkill?.source.namespace).toBe('gen');
  });

  it('should merge skills with correct priority (project gen > project claude)', async () => {
    await createProjectSkill('claude', 'shared-skill', 'Claude project version');
    await createProjectSkill('gen', 'shared-skill', 'Gen project version (wins)');

    await discovery.discover(tempDir);

    expect(discovery.count()).toBe(1);
    const skill = discovery.get('shared-skill');
    expect(skill?.description).toBe('Gen project version (wins)');
    expect(skill?.source.namespace).toBe('gen');
    expect(skill?.source.level).toBe('project');
  });

  it('should has() return correct boolean values', async () => {
    await createProjectSkill('gen', 'existing-skill', 'Exists');

    await discovery.discover(tempDir);

    expect(discovery.has('existing-skill')).toBe(true);
    expect(discovery.has('non-existent-skill')).toBe(false);
  });

  it('should count() return correct number of skills', async () => {
    await discovery.discover(tempDir);
    expect(discovery.count()).toBe(0);

    await createProjectSkill('gen', 'skill1', 'First');
    await discovery.reload(tempDir); // Use reload to refresh after adding skills
    expect(discovery.count()).toBe(1);

    await createProjectSkill('gen', 'skill2', 'Second');
    await discovery.reload(tempDir); // Use reload again
    expect(discovery.count()).toBe(2);
  });

  it('should names() return all skill names', async () => {
    await createProjectSkill('gen', 'skill-alpha', 'Alpha');
    await createProjectSkill('gen', 'skill-beta', 'Beta');
    await createProjectSkill('claude', 'skill-gamma', 'Gamma');

    await discovery.discover(tempDir);

    const names = discovery.names();
    expect(names).toHaveLength(3);
    expect(names).toContain('skill-alpha');
    expect(names).toContain('skill-beta');
    expect(names).toContain('skill-gamma');
  });
});
