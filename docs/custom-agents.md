# Custom Agents

GenCode supports custom agent definitions, allowing you to create specialized subagents tailored to your workflow. Custom agents can be defined in JSON or Markdown format and are automatically loaded at runtime.

## Agent Merge Mechanism

GenCode supports loading custom agents from two locations with a merge strategy:

1. **Claude Code agents**: `~/.claude/agents/` (lower priority)
2. **GenCode agents**: `~/.gen/agents/` (higher priority)

**Priority Rules:**
- If an agent with the same name exists in both locations, GenCode's version takes precedence
- This allows you to override Claude Code agents with your own customizations
- Deleting a GenCode agent will automatically fall back to the Claude Code version (if it exists)

## Creating Custom Agents

### JSON Format (Recommended for GenCode)

Create a file in `~/.gen/agents/<agent-name>.json`:

```json
{
  "name": "code-reviewer",
  "type": "custom",
  "description": "Expert code review specialist",
  "allowedTools": ["Read", "Grep", "Glob", "WebFetch"],
  "defaultModel": "claude-sonnet-4",
  "maxTurns": 15,
  "permissionMode": "permissive",
  "systemPrompt": "You are a senior code reviewer with expertise in:\n- Security vulnerabilities\n- Performance optimization\n- Best practices\n- Code maintainability\n\nWhen reviewing code:\n1. Look for security issues (injection, XSS, etc.)\n2. Check for performance bottlenecks\n3. Verify adherence to best practices\n4. Suggest improvements for maintainability\n\nProvide constructive, actionable feedback."
}
```

### Markdown Format (Compatible with Claude Code)

Create a file in `~/.claude/agents/<agent-name>.md`:

```markdown
---
name: test-architect
description: Test architecture and coverage specialist
allowedTools: ["Read", "Grep", "Glob", "WebFetch"]
defaultModel: claude-sonnet-4
maxTurns: 12
permissionMode: permissive
---

You are a test architecture specialist focused on:
- Test coverage analysis
- Testing strategy design
- Test framework recommendations
- Integration and E2E test patterns

When analyzing test requirements:
1. Assess current test coverage
2. Identify gaps in testing
3. Recommend appropriate test types
4. Suggest test framework improvements

Provide comprehensive test strategy recommendations.
```

## Configuration Fields

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | Yes | string | Unique agent identifier |
| `type` | Yes | string | Must be "custom" |
| `description` | Yes | string | Agent description |
| `allowedTools` | Yes | string[] | Tools the agent can use |
| `defaultModel` | Yes | string | Default LLM model |
| `maxTurns` | Yes | number | Maximum conversation turns |
| `permissionMode` | No | string | "inherit", "isolated", or "permissive" (default: "permissive") |
| `systemPrompt` | Yes | string | System prompt for the agent |

## Available Tools

Tools you can include in `allowedTools`:

- **Read-only**: `Read`, `Glob`, `Grep`, `WebFetch`, `WebSearch`
- **Write**: `Write`, `Edit`
- **Execution**: `Bash`
- **Interactive**: `TodoWrite`, `AskUserQuestion`
- **Meta**: `Task` (for spawning nested subagents)
- **Wildcard**: `*` (all tools)

## Using Custom Agents

Once defined, custom agents can be used just like built-in agents:

```typescript
Task({
  description: "Review authentication",
  prompt: "Review the authentication implementation for security issues",
  subagent_type: "code-reviewer"  // Your custom agent
})
```

## Managing Custom Agents

### List All Agents with Sources

```typescript
import { getLoader } from './subagents/configs.js';

const loader = getLoader();
const agentsWithSources = await loader.listAgentsWithSources();

for (const [name, source] of agentsWithSources) {
  console.log(`${name}: ${source}`);
}
```

### Get Agent Information

```typescript
const info = await loader.getAgentInfo('code-reviewer');
if (info) {
  console.log(`Agent: code-reviewer`);
  console.log(`Source: ${info.source}`);  // 'gencode' or 'claude'
  console.log(`Tools: ${info.config.allowedTools.join(', ')}`);
}
```

### Override Claude Code Agent

If you have a Claude Code agent at `~/.claude/agents/my-agent.md`, you can override it by creating:

`~/.gen/agents/my-agent.json`:

```json
{
  "name": "my-agent",
  "type": "custom",
  "description": "Customized version with different tools",
  "allowedTools": ["Read", "Write", "Edit", "Bash"],
  "defaultModel": "claude-sonnet-4",
  "maxTurns": 20,
  "systemPrompt": "My customized system prompt..."
}
```

Now when you use `my-agent`, GenCode's version will be used.

### Delete Override (Fall Back to Claude Code)

```typescript
const loader = getLoader();
await loader.deleteAgentConfig('my-agent');
// Now Claude Code's version will be used (if it exists)
```

## Best Practices

1. **Naming**: Use descriptive, kebab-case names (e.g., `code-reviewer`, `test-architect`)
2. **Tool Selection**: Only include tools the agent actually needs
3. **Model Choice**:
   - Use `claude-haiku-4` for simple, fast tasks
   - Use `claude-sonnet-4` for complex reasoning
   - Use `claude-opus-4-5` for most sophisticated tasks
4. **System Prompts**: Be specific about the agent's expertise and workflow
5. **Max Turns**: Set appropriately based on expected task complexity
6. **Testing**: Test custom agents with real tasks before relying on them

## Examples

### Documentation Writer

```json
{
  "name": "doc-writer",
  "type": "custom",
  "description": "Technical documentation specialist",
  "allowedTools": ["Read", "Write", "Glob", "Grep", "WebFetch"],
  "defaultModel": "claude-sonnet-4",
  "maxTurns": 15,
  "systemPrompt": "You are a technical documentation specialist. Create clear, comprehensive documentation with examples, API references, and user guides."
}
```

### Performance Analyzer

```json
{
  "name": "perf-analyzer",
  "type": "custom",
  "description": "Performance analysis and optimization expert",
  "allowedTools": ["Read", "Grep", "Glob", "Bash"],
  "defaultModel": "claude-sonnet-4",
  "maxTurns": 12,
  "systemPrompt": "You are a performance optimization expert. Analyze code for bottlenecks, suggest optimizations, and benchmark improvements."
}
```

### Security Auditor

```json
{
  "name": "security-auditor",
  "type": "custom",
  "description": "Security vulnerability assessment specialist",
  "allowedTools": ["Read", "Grep", "Glob", "WebFetch"],
  "defaultModel": "claude-opus-4-5",
  "maxTurns": 20,
  "systemPrompt": "You are a security auditor specializing in web application vulnerabilities. Scan for OWASP Top 10 issues, insecure dependencies, and potential attack vectors."
}
```

## Troubleshooting

### Agent Not Loading

Check the console for error messages:
```
Failed to load custom agent from my-agent.json (gencode): Invalid config
```

Common issues:
- Invalid JSON syntax
- Missing required fields
- Invalid tool names
- Invalid model names

### Agent Using Wrong Version

Check which version is active:
```typescript
const source = await loader.getAgentSource('my-agent');
console.log(`Active version: ${source}`);  // 'gencode' or 'claude'
```

If you want to force GenCode version, ensure the file exists in `~/.gen/agents/`.

### Reload After Changes

The loader caches configs. To reload:
```typescript
await loader.reload();
```

Or restart GenCode to pick up changes automatically.
