# Proposal: Permission Enhancements

- **Proposal ID**: 0023
- **Author**: mycode team
- **Status**: Implemented
- **Created**: 2025-01-15
- **Updated**: 2026-01-15
- **Implemented**: 2026-01-15

## Summary

Enhance the permission system with pattern-based rules, prompt-based approvals, persistent allowlists, and fine-grained control over tool execution. This provides users with flexible, secure control over agent capabilities.

## Motivation

The current permission system is basic:

1. **Tool-level only**: Can't distinguish safe vs dangerous uses
2. **No persistence**: Approvals lost between sessions
3. **No patterns**: Can't approve "all git commands"
4. **Manual each time**: Repetitive confirmation fatigue
5. **No audit trail**: No record of what was approved

Enhanced permissions balance security with usability.

## Claude Code Reference

Claude Code provides sophisticated permission handling:

### ExitPlanMode Permissions
```typescript
ExitPlanMode({
  allowedPrompts: [
    { tool: "Bash", prompt: "run tests" },
    { tool: "Bash", prompt: "install dependencies" },
    { tool: "Bash", prompt: "build the project" }
  ]
})
```

### Settings-Based Permissions
```json
{
  "permissions": {
    "allow": [
      "Bash(git add:*)",
      "Bash(git commit:*)",
      "Bash(npm install:*)"
    ]
  }
}
```

### Permission Guidelines
- Scope permissions narrowly
- Use semantic descriptions, not literal commands
- Read-only permissions for read-only operations
- Session-scoped by default
- Input-aware caching

## Detailed Design

### API Design

```typescript
// src/permissions/types.ts
type PermissionMode = 'auto' | 'confirm' | 'deny';

interface PermissionRule {
  tool: string | RegExp;
  mode: PermissionMode;
  pattern?: string | RegExp;      // Input pattern matching
  prompt?: string;                // Semantic description
  scope?: 'session' | 'project' | 'global';
  expiresAt?: Date;
}

interface PromptPermission {
  tool: string;
  prompt: string;                 // Semantic description
}

interface PermissionConfig {
  defaultMode: PermissionMode;
  rules: PermissionRule[];
  allowedPrompts: PromptPermission[];
}

interface PermissionContext {
  tool: string;
  input: unknown;
  sessionId: string;
  projectPath: string;
}

interface PermissionDecision {
  allowed: boolean;
  reason: string;
  matchedRule?: PermissionRule;
  requiresConfirmation: boolean;
}

interface PermissionAuditEntry {
  timestamp: Date;
  tool: string;
  input: unknown;
  decision: 'allowed' | 'denied' | 'confirmed';
  rule?: string;
  userId?: string;
}
```

### Enhanced Permission Manager

```typescript
// src/permissions/manager.ts
class PermissionManager {
  private config: PermissionConfig;
  private sessionApprovals: Map<string, Set<string>> = new Map();
  private persistentRules: PermissionRule[] = [];
  private auditLog: PermissionAuditEntry[] = [];
  private promptMatcher: PromptMatcher;

  constructor(config?: Partial<PermissionConfig>) {
    this.config = {
      defaultMode: 'confirm',
      rules: DEFAULT_RULES,
      allowedPrompts: [],
      ...config
    };
    this.loadPersistentRules();
    this.promptMatcher = new PromptMatcher();
  }

  async checkPermission(
    context: PermissionContext
  ): Promise<PermissionDecision> {
    const { tool, input, sessionId } = context;

    // Check explicit deny rules first
    const denyRule = this.findMatchingRule(context, 'deny');
    if (denyRule) {
      this.logAudit(context, 'denied', denyRule);
      return {
        allowed: false,
        reason: `Denied by rule: ${denyRule.prompt || denyRule.pattern}`,
        matchedRule: denyRule,
        requiresConfirmation: false
      };
    }

    // Check auto-allow rules
    const autoRule = this.findMatchingRule(context, 'auto');
    if (autoRule) {
      this.logAudit(context, 'allowed', autoRule);
      return {
        allowed: true,
        reason: 'Auto-approved by rule',
        matchedRule: autoRule,
        requiresConfirmation: false
      };
    }

    // Check prompt-based permissions
    const promptMatch = await this.matchPrompt(tool, input);
    if (promptMatch) {
      this.logAudit(context, 'allowed', { prompt: promptMatch });
      return {
        allowed: true,
        reason: `Matches approved prompt: ${promptMatch}`,
        requiresConfirmation: false
      };
    }

    // Check session approvals cache
    const cacheKey = this.getCacheKey(tool, input);
    const sessionCache = this.sessionApprovals.get(sessionId);
    if (sessionCache?.has(cacheKey)) {
      return {
        allowed: true,
        reason: 'Previously approved in session',
        requiresConfirmation: false
      };
    }

    // Default: requires confirmation
    return {
      allowed: false,
      reason: 'Requires user confirmation',
      requiresConfirmation: true
    };
  }

  async requestPermission(
    context: PermissionContext,
    confirmCallback: ConfirmCallback
  ): Promise<boolean> {
    const decision = await this.checkPermission(context);

    if (decision.allowed) return true;
    if (!decision.requiresConfirmation) return false;

    // Request user confirmation
    const confirmed = await confirmCallback(
      context.tool,
      context.input,
      this.getSuggestions(context)
    );

    if (confirmed) {
      this.cacheApproval(context);
      this.logAudit(context, 'confirmed');
    } else {
      this.logAudit(context, 'denied');
    }

    return confirmed;
  }

  addAllowedPrompts(prompts: PromptPermission[]): void {
    this.config.allowedPrompts.push(...prompts);
  }

  clearSessionApprovals(sessionId: string): void {
    this.sessionApprovals.delete(sessionId);
  }

  private async matchPrompt(tool: string, input: unknown): Promise<string | null> {
    for (const permission of this.config.allowedPrompts) {
      if (permission.tool !== tool) continue;

      const matches = await this.promptMatcher.matches(
        permission.prompt,
        input
      );
      if (matches) return permission.prompt;
    }
    return null;
  }

  private findMatchingRule(
    context: PermissionContext,
    mode: PermissionMode
  ): PermissionRule | undefined {
    const allRules = [...this.config.rules, ...this.persistentRules];

    return allRules.find(rule => {
      if (rule.mode !== mode) return false;
      if (!this.matchesTool(rule.tool, context.tool)) return false;
      if (rule.pattern && !this.matchesPattern(rule.pattern, context.input)) {
        return false;
      }
      return true;
    });
  }

  private matchesTool(pattern: string | RegExp, tool: string): boolean {
    if (typeof pattern === 'string') {
      return pattern === tool || pattern === '*';
    }
    return pattern.test(tool);
  }

  private matchesPattern(pattern: string | RegExp, input: unknown): boolean {
    const inputStr = JSON.stringify(input);
    if (typeof pattern === 'string') {
      // Support glob-like patterns
      const regex = new RegExp(pattern.replace(/\*/g, '.*'));
      return regex.test(inputStr);
    }
    return pattern.test(inputStr);
  }

  // Persistence
  async saveRule(rule: PermissionRule): Promise<void> {
    this.persistentRules.push(rule);
    await this.savePersistentRules();
  }

  private getSuggestions(context: PermissionContext): PermissionSuggestion[] {
    return [
      { action: 'allow_once', label: 'Allow this time' },
      { action: 'allow_session', label: 'Allow for this session' },
      { action: 'allow_always', label: 'Always allow this' },
      { action: 'deny', label: 'Deny' }
    ];
  }
}
```

### Prompt Matcher

```typescript
// src/permissions/prompt-matcher.ts
class PromptMatcher {
  private patterns: Map<string, (input: unknown) => boolean> = new Map();

  constructor() {
    this.registerBuiltInPatterns();
  }

  private registerBuiltInPatterns(): void {
    // Git operations
    this.patterns.set('run tests', input =>
      this.matchesCommand(input, ['npm test', 'pytest', 'go test', 'jest'])
    );

    this.patterns.set('install dependencies', input =>
      this.matchesCommand(input, ['npm install', 'pip install', 'cargo build'])
    );

    this.patterns.set('build the project', input =>
      this.matchesCommand(input, ['npm run build', 'make', 'cargo build'])
    );

    this.patterns.set('git operations', input =>
      this.matchesCommand(input, ['git '])
    );
  }

  async matches(prompt: string, input: unknown): Promise<boolean> {
    // Check exact pattern match
    const pattern = this.patterns.get(prompt.toLowerCase());
    if (pattern) return pattern(input);

    // Fuzzy semantic matching (for complex prompts)
    return this.semanticMatch(prompt, input);
  }

  private matchesCommand(input: unknown, prefixes: string[]): boolean {
    const command = (input as { command?: string })?.command || '';
    return prefixes.some(prefix => command.startsWith(prefix));
  }

  private semanticMatch(prompt: string, input: unknown): boolean {
    // Simple keyword matching for now
    const inputStr = JSON.stringify(input).toLowerCase();
    const keywords = prompt.toLowerCase().split(/\s+/);
    return keywords.some(kw => inputStr.includes(kw));
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/permissions/types.ts` | Modify | Enhanced types |
| `src/permissions/manager.ts` | Modify | Enhanced manager |
| `src/permissions/prompt-matcher.ts` | Create | Semantic matching |
| `src/permissions/persistence.ts` | Create | Rule persistence |
| `src/permissions/audit.ts` | Create | Audit logging |
| `src/cli/commands/permissions.ts` | Create | Permission CLI |

## User Experience

### Permission Prompt with Options
```
┌─ Permission Request ──────────────────────────────┐
│ Tool: Bash                                        │
│ Command: npm install lodash                       │
│                                                   │
│ This operation requires your approval.            │
│                                                   │
│ [1] Allow once                                    │
│ [2] Allow for this session                        │
│ [3] Always allow "npm install" commands           │
│ [4] Deny                                          │
└───────────────────────────────────────────────────┘
```

### View Permissions
```
User: /permissions

Permission Rules:
┌────────────────────────────────────────────────────────────┐
│ Type    Tool   Pattern              Scope      Mode       │
├────────────────────────────────────────────────────────────┤
│ Built-in Read   *                   session    auto       │
│ Built-in Glob   *                   session    auto       │
│ Built-in Grep   *                   session    auto       │
│ Custom   Bash   npm install:*       project    auto       │
│ Custom   Bash   git add:*           global     auto       │
│ Session  Bash   pytest              session    auto       │
└────────────────────────────────────────────────────────────┘

Pending Prompts (from plan approval):
• Bash: run tests
• Bash: build the project
```

### Permission Audit
```
User: /permissions audit

Recent Permission Decisions:
┌────────────────────────────────────────────────────────────────┐
│ Time     Tool   Input               Decision   Rule           │
├────────────────────────────────────────────────────────────────┤
│ 10:42    Bash   npm test            allowed    prompt:tests   │
│ 10:41    Read   src/index.ts        allowed    built-in       │
│ 10:40    Write  src/new.ts          confirmed  user-approval  │
│ 10:38    Bash   rm -rf /tmp/test    denied     blocked-cmd    │
└────────────────────────────────────────────────────────────────┘
```

## Alternatives Considered

### Alternative 1: Capability-Based Security
Use capability tokens instead of rules.

**Pros**: More formal model
**Cons**: Complex for users
**Decision**: Deferred - Consider for enterprise

### Alternative 2: AI-Based Permission Decisions
Let AI assess risk of operations.

**Pros**: Intelligent decisions
**Cons**: Unpredictable, trust issues
**Decision**: Rejected - Users need control

### Alternative 3: Whitelist-Only Mode
Only allow explicitly approved operations.

**Pros**: Maximum security
**Cons**: Too restrictive for usability
**Decision**: Available as strict mode

## Security Considerations

1. **Rule Validation**: Validate rule patterns
2. **Audit Trail**: Immutable audit log
3. **Escalation Prevention**: Can't grant more permissions than held
4. **Expiration**: Time-limited approvals
5. **Scope Limits**: Limit global rules

## Testing Strategy

1. **Unit Tests**:
   - Rule matching logic
   - Pattern matching
   - Prompt matching
   - Cache behavior

2. **Integration Tests**:
   - Full permission flow
   - Persistence
   - Audit logging

3. **Security Tests**:
   - Bypass attempts
   - Injection attacks
   - Escalation scenarios

## Migration Path

1. **Phase 1**: Enhanced rule matching
2. **Phase 2**: Prompt-based permissions
3. **Phase 3**: Persistence layer
4. **Phase 4**: Audit logging
5. **Phase 5**: CLI management

Backward compatible with existing rules.

## Implementation Notes

### Files Created/Modified

| File | Action | Description |
|------|--------|-------------|
| `src/permissions/types.ts` | Modified | Enhanced types with ApprovalAction, PromptPermission, audit types, persistence types |
| `src/permissions/manager.ts` | Modified | Enhanced PermissionManager with pattern matching, prompt matching, persistence, audit |
| `src/permissions/prompt-matcher.ts` | Created | Semantic prompt matching for Claude Code style permissions |
| `src/permissions/persistence.ts` | Created | Persistence layer for storing rules to disk |
| `src/permissions/audit.ts` | Created | Audit logging for permission decisions |
| `src/permissions/index.ts` | Modified | Export all new modules |
| `src/cli/components/PermissionPrompt.tsx` | Created | Enhanced permission prompt UI with approval options |
| `src/cli/components/CommandSuggestions.tsx` | Modified | Added /permissions command |
| `src/cli/components/App.tsx` | Modified | Integrated permission system, added /permissions command |
| `src/cli/components/index.ts` | Modified | Export new components |
| `src/cli/index.tsx` | Modified | Pass permission settings to App |
| `src/agent/agent.ts` | Modified | Enhanced permission integration with new API |

### Key Implementation Details

1. **Pattern-Based Rules**: Supports Claude Code format like `Bash(git add:*)` with glob-style wildcards
2. **Prompt-Based Permissions**: ExitPlanMode style with semantic matching for common operations (run tests, install dependencies, etc.)
3. **Multi-Scope Permissions**: Session (in-memory), Project (.gen/permissions.json), Global (~/.gen/permissions.json)
4. **Approval Options**: Allow once, Allow for session, Always allow (persistent), Deny
5. **Audit Logging**: In-memory audit trail with optional file persistence
6. **CLI Commands**: `/permissions` shows rules, `/permissions audit` shows decision history, `/permissions stats` shows statistics

### Usage Examples

```bash
# View current permission rules
/permissions

# View audit log
/permissions audit

# View statistics
/permissions stats
```

### Settings Configuration

Add to `~/.gen/settings.json`:
```json
{
  "permissions": {
    "allow": [
      "Bash(git add:*)",
      "Bash(npm install:*)"
    ],
    "deny": [
      "Bash(rm -rf:*)"
    ]
  }
}
```

## References

- [Claude Code Permission System](https://code.claude.com/docs/en/permissions)
- [Capability-Based Security](https://en.wikipedia.org/wiki/Capability-based_security)
- [Principle of Least Privilege](https://en.wikipedia.org/wiki/Principle_of_least_privilege)
