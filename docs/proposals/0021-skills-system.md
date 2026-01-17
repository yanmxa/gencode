# Proposal: Skills System

- **Proposal ID**: 0021
- **Author**: mycode team
- **Status**: Draft
- **Priority**: P1 (Core Feature)
- **Created**: 2025-01-15
- **Updated**: 2025-01-17

## Summary

Implement a skills system that allows defining reusable, model-invoked capabilities with specialized prompts and tool configurations. Skills provide domain expertise and workflow automation that the agent can call upon when appropriate.

## Motivation

Currently, mycode has no mechanism for extending agent capabilities beyond built-in tools:

1. **No domain expertise**: Agent lacks specialized knowledge
2. **Repetitive prompts**: Same workflows require repeated instructions
3. **No customization**: Users can't add domain-specific capabilities
4. **Limited automation**: Complex workflows need manual steps
5. **No reusability**: Good patterns can't be packaged and shared

Skills enable encapsulated, reusable agent capabilities.

## Claude Code Reference

Claude Code's skills system provides model-invoked capabilities:

### Skill Structure
```
~/.claude/skills/skill-name/
├── SKILL.md          # Skill definition with frontmatter
├── scripts/          # Optional helper scripts
└── resources/        # Optional data files
```

### SKILL.md Format
```yaml
---
name: test-plan-generator
description: Generate comprehensive test plans for releases...
allowed-tools: [Bash, AskUserQuestion, TodoWrite, Read, Write]
---

# Test Plan Generator

You are a test planning specialist...

## Core Responsibilities
1. Analyze test requirements
2. Generate structured test plans
3. Create Jira issues for test cases

## Workflow
...
```

### Skill vs Command vs Subagent
| Feature | Skill | Command | Subagent |
|---------|-------|---------|----------|
| Invocation | Model-invoked | User-invoked | Model-invoked |
| Context | Main conversation | Main conversation | Isolated |
| Use Case | Domain expertise | Quick automation | Complex workflows |

### Skill Tool
```typescript
Skill({
  skill: "test-plan-generator",
  args: "--release 2.0.0"
})
```

## Detailed Design

### API Design

```typescript
// src/skills/types.ts
interface SkillDefinition {
  name: string;
  description: string;
  allowedTools?: string[];
  systemPrompt: string;
  metadata?: {
    version?: string;
    author?: string;
    tags?: string[];
  };
}

interface SkillInput {
  skill: string;           // Skill name or full path
  args?: string;           // Arguments to pass
}

interface SkillOutput {
  success: boolean;
  result?: string;
  error?: string;
}

interface SkillRegistry {
  skills: Map<string, SkillDefinition>;
  skillPaths: Map<string, string>;  // name -> filesystem path
}

interface SkillContext {
  args: string;
  cwd: string;
  session: Session;
}
```

### Skill Tool Implementation

```typescript
// src/tools/skill/skill-tool.ts
const skillTool: Tool<SkillInput> = {
  name: 'Skill',
  description: `Execute a skill within the main conversation.

Skills are specialized capabilities that provide domain expertise.
When the task matches a skill's purpose, invoke it to leverage
specialized knowledge and workflows.

Parameters:
- skill: The skill name (e.g., "test-plan-generator")
- args: Optional arguments (e.g., "--release 2.0.0")

Available skills are loaded from:
- ~/.mycode/skills/
- Project .mycode/skills/
- Installed plugins

Use this when the current task aligns with an available skill.
`,
  parameters: z.object({
    skill: z.string(),
    args: z.string().optional()
  }),
  execute: async (input, context) => {
    const skill = await skillRegistry.get(input.skill);
    if (!skill) {
      return {
        success: false,
        error: `Skill not found: ${input.skill}`
      };
    }

    // Execute skill in main context
    return await executeSkill(skill, {
      args: input.args || '',
      cwd: context.cwd,
      session: context.session
    });
  }
};
```

### Skill Registry

```typescript
// src/skills/registry.ts
class SkillRegistry {
  private skills: Map<string, SkillDefinition> = new Map();
  private skillPaths: Map<string, string> = new Map();

  constructor() {
    this.loadSkills();
  }

  private async loadSkills(): Promise<void> {
    // Load from multiple sources
    await this.loadFromDirectory(expandPath('~/.mycode/skills'));
    await this.loadFromDirectory(path.join(process.cwd(), '.mycode/skills'));
    await this.loadFromPlugins();
  }

  private async loadFromDirectory(dir: string): Promise<void> {
    if (!fs.existsSync(dir)) return;

    const entries = await fs.readdir(dir, { withFileTypes: true });
    for (const entry of entries) {
      if (entry.isDirectory()) {
        const skillPath = path.join(dir, entry.name, 'SKILL.md');
        if (fs.existsSync(skillPath)) {
          await this.loadSkill(skillPath);
        }
      }
    }
  }

  private async loadSkill(skillPath: string): Promise<void> {
    const content = await fs.readFile(skillPath, 'utf-8');
    const { frontmatter, body } = parseFrontmatter(content);

    const skill: SkillDefinition = {
      name: frontmatter.name,
      description: frontmatter.description,
      allowedTools: frontmatter['allowed-tools'],
      systemPrompt: body,
      metadata: {
        version: frontmatter.version,
        author: frontmatter.author,
        tags: frontmatter.tags
      }
    };

    this.skills.set(skill.name, skill);
    this.skillPaths.set(skill.name, path.dirname(skillPath));
  }

  get(name: string): SkillDefinition | undefined {
    return this.skills.get(name);
  }

  list(): SkillDefinition[] {
    return Array.from(this.skills.values());
  }

  getPath(name: string): string | undefined {
    return this.skillPaths.get(name);
  }
}

export const skillRegistry = new SkillRegistry();
```

### Skill Execution

```typescript
// src/skills/executor.ts
async function executeSkill(
  skill: SkillDefinition,
  context: SkillContext
): Promise<SkillOutput> {
  // Build skill context message
  const skillContext = `
## Skill Activated: ${skill.name}

${skill.systemPrompt}

## Current Context
- Working Directory: ${context.cwd}
- Arguments: ${context.args || '(none)'}

Please proceed with the skill's responsibilities.
`;

  // Inject skill context into current conversation
  context.session.addMessage({
    role: 'system',
    content: skillContext
  });

  return {
    success: true,
    result: `Skill '${skill.name}' activated. Following skill instructions.`
  };
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/skills/types.ts` | Create | Type definitions |
| `src/skills/registry.ts` | Create | Skill loading and registry |
| `src/skills/executor.ts` | Create | Skill execution logic |
| `src/skills/parser.ts` | Create | SKILL.md parsing |
| `src/skills/index.ts` | Create | Module exports |
| `src/tools/skill/skill-tool.ts` | Create | Skill tool |
| `src/tools/index.ts` | Modify | Register Skill tool |

## User Experience

### Listing Available Skills
```
User: /skills

Available Skills:
┌────────────────────────────────────────────────────────────┐
│ Name                    Description                        │
├────────────────────────────────────────────────────────────┤
│ test-plan-generator     Generate test plans for releases  │
│ jira-analyzer           Analyze Jira issues complexity    │
│ pr-reviewer             Review pull requests              │
│ db-migration            Database migration specialist     │
└────────────────────────────────────────────────────────────┘

Skills are invoked automatically when relevant, or use:
  Skill tool with skill name
```

### Automatic Skill Invocation
```
User: I need to create a test plan for release 2.5

Agent: This task aligns with the test-plan-generator skill.
Let me invoke it to provide structured test planning.

[Skill: skill="test-plan-generator", args="--release 2.5"]

[Skill activated - following test plan generation workflow]

Based on the test-plan-generator methodology:

1. First, let me analyze the release scope...
```

### Skill Execution with Context
```
Agent: [Skill: skill="jira-analyzer"]

┌─ Skill: jira-analyzer ────────────────────────────┐
│ Analyzing Jira issues for complexity assessment  │
│ Using: Bash, Read, TodoWrite                     │
└───────────────────────────────────────────────────┘

Following the jira-analyzer workflow:
1. Fetching issues from the current sprint...
```

### Creating a New Skill
```
User: /skill create code-reviewer

Created skill template at:
  ~/.mycode/skills/code-reviewer/SKILL.md

Edit the SKILL.md file to define your skill:
  - name: Skill identifier
  - description: When to use this skill
  - allowed-tools: Tools the skill can use
  - Body: System prompt with instructions
```

## Alternatives Considered

### Alternative 1: Inline Prompts Only
Keep using system prompts without skills.

**Pros**: Simpler, no new concepts
**Cons**: No reusability, no packaging
**Decision**: Rejected - Skills enable composition

### Alternative 2: Function-based Skills
Skills as JavaScript functions.

**Pros**: More powerful, type-safe
**Cons**: Harder to create, security concerns
**Decision**: Rejected - Markdown is more accessible

### Alternative 3: Skills as Subagents
Always run skills in isolated context.

**Pros**: Clean isolation
**Cons**: Loses main context, slower
**Decision**: Rejected - Main context is valuable

## Security Considerations

1. **Tool Restrictions**: Honor allowed-tools list
2. **Path Validation**: Validate skill paths
3. **Script Execution**: Sandbox skill scripts
4. **Injection Prevention**: Sanitize skill content
5. **Resource Limits**: Limit skill execution time

```typescript
const SKILL_LIMITS = {
  maxPromptLength: 50000,
  maxScriptRuntime: 60000,
  maxScriptsPerSkill: 10
};
```

## Testing Strategy

1. **Unit Tests**:
   - SKILL.md parsing
   - Registry loading
   - Tool filtering
   - Context injection

2. **Integration Tests**:
   - Full skill execution
   - Multiple skill sources
   - Plugin skill loading

3. **Manual Testing**:
   - Various skill types
   - Skill creation workflow
   - Error handling

## Migration Path

1. **Phase 1**: Core registry and loader
2. **Phase 2**: Skill tool implementation
3. **Phase 3**: /skills CLI command
4. **Phase 4**: Skill creation helpers
5. **Phase 5**: Plugin skill integration

No breaking changes to existing functionality.

## References

- [Claude Code Skills Documentation](https://code.claude.com/docs/en/skills)
- [YAML Frontmatter Specification](https://jekyllrb.com/docs/front-matter/)
- [Plugin System Proposal (0022)](./0022-plugin-system.md)
