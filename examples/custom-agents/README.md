# Custom Agent Examples

This directory contains example custom agent configurations for GenCode.

## Available Examples

### code-reviewer.json
Expert code review specialist focusing on:
- Security vulnerabilities
- Performance optimization
- Code quality and maintainability
- Best practices

**Format**: JSON (GenCode native)
**Usage**: Copy to `~/.gen/agents/code-reviewer.json`

### test-architect.md
Test architecture and coverage specialist focusing on:
- Test coverage analysis
- Testing strategy design
- Test framework recommendations
- Test organization

**Format**: Markdown with frontmatter (Claude Code compatible)
**Usage**: Copy to `~/.claude/agents/test-architect.md` or `~/.gen/agents/test-architect.md`

## Installation

### Option 1: GenCode Directory (Recommended)

```bash
# Copy to GenCode agents directory
mkdir -p ~/.gen/agents
cp code-reviewer.json ~/.gen/agents/
cp test-architect.md ~/.gen/agents/
```

### Option 2: Claude Code Directory (Compatible)

```bash
# Copy to Claude Code agents directory
mkdir -p ~/.claude/agents
cp test-architect.md ~/.claude/agents/
```

### Option 3: Both (with Override)

```bash
# Install in both directories
mkdir -p ~/.gen/agents ~/.claude/agents

# Claude Code version (lower priority)
cp test-architect.md ~/.claude/agents/

# GenCode version (higher priority - will override)
cp code-reviewer.json ~/.gen/agents/
```

## Using Custom Agents

Once installed, use them in Task calls:

```typescript
// Code review
Task({
  description: "Review auth code",
  prompt: "Review the authentication implementation in src/auth/ for security issues",
  subagent_type: "code-reviewer"
})

// Test analysis
Task({
  description: "Analyze test coverage",
  prompt: "Analyze the test coverage for the user service and recommend improvements",
  subagent_type: "test-architect"
})
```

## Creating Your Own

1. **Choose a format**:
   - JSON for GenCode-native agents (simpler, more structured)
   - Markdown for Claude Code compatibility (richer documentation)

2. **Define configuration**:
   ```json
   {
     "name": "my-agent",
     "type": "custom",
     "description": "Brief description",
     "allowedTools": ["Read", "Write", ...],
     "defaultModel": "claude-sonnet-4",
     "maxTurns": 15,
     "systemPrompt": "Detailed instructions..."
   }
   ```

3. **Save to agents directory**:
   - `~/.gen/agents/my-agent.json` (GenCode)
   - `~/.claude/agents/my-agent.md` (Claude Code)

4. **Test your agent**:
   ```typescript
   Task({
     description: "Test custom agent",
     prompt: "Test task for my custom agent",
     subagent_type: "my-agent"
   })
   ```

## Best Practices

1. **Naming**: Use descriptive, kebab-case names
2. **Tools**: Only include tools the agent needs
3. **Model Selection**:
   - `claude-haiku-4`: Fast, simple tasks
   - `claude-sonnet-4`: Complex reasoning (recommended)
   - `claude-opus-4-5`: Most sophisticated tasks
4. **System Prompt**: Be specific about expertise and workflow
5. **Max Turns**: Match expected task complexity

## More Examples

For more custom agent examples and detailed documentation, see:
- [Custom Agents Documentation](../../docs/custom-agents.md)
- [Subagent System Proposal](../../docs/proposals/0003-task-subagents.md)
