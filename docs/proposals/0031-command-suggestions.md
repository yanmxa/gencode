# Proposal: Command Suggestions

- **Proposal ID**: 0031
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement intelligent command suggestions and autocomplete for the CLI, providing contextual recommendations as users type. This improves discoverability and reduces typing effort.

## Motivation

Currently, users must know exact command names:

1. **No discovery**: Must read docs to find commands
2. **No autocomplete**: Type full command names
3. **No context**: Suggestions not context-aware
4. **Typo friction**: No fuzzy matching
5. **Slow onboarding**: New users struggle

Command suggestions improve productivity and discoverability.

## Claude Code Reference

Claude Code provides Tab completion and suggestions:

### Observed Behavior
- Tab completes commands
- Shows available options
- Fuzzy matching for typos

## Detailed Design

### API Design

```typescript
// src/cli/suggestions/types.ts
interface Suggestion {
  text: string;
  displayText?: string;
  description?: string;
  type: 'command' | 'option' | 'path' | 'history' | 'skill';
  score: number;
  source: string;
}

interface SuggestionContext {
  input: string;
  cursorPosition: number;
  cwd: string;
  history: string[];
  session?: Session;
}

interface SuggestionConfig {
  maxSuggestions: number;
  fuzzyMatch: boolean;
  includeHistory: boolean;
  includePaths: boolean;
  includeSkills: boolean;
}
```

### Suggestion Engine

```typescript
// src/cli/suggestions/engine.ts
class SuggestionEngine {
  private config: SuggestionConfig;
  private providers: SuggestionProvider[] = [];

  constructor(config?: Partial<SuggestionConfig>) {
    this.config = {
      maxSuggestions: 10,
      fuzzyMatch: true,
      includeHistory: true,
      includePaths: true,
      includeSkills: true,
      ...config
    };

    this.registerProviders();
  }

  private registerProviders(): void {
    this.providers.push(new CommandSuggestionProvider());
    this.providers.push(new HistorySuggestionProvider());
    this.providers.push(new PathSuggestionProvider());
    this.providers.push(new SkillSuggestionProvider());
  }

  async suggest(context: SuggestionContext): Promise<Suggestion[]> {
    const { input } = context;

    // Gather suggestions from all providers
    const allSuggestions: Suggestion[] = [];

    for (const provider of this.providers) {
      if (!this.isProviderEnabled(provider)) continue;

      const suggestions = await provider.suggest(context);
      allSuggestions.push(...suggestions);
    }

    // Score and sort
    const scored = allSuggestions.map(s => ({
      ...s,
      score: this.calculateScore(s, input)
    }));

    scored.sort((a, b) => b.score - a.score);

    return scored.slice(0, this.config.maxSuggestions);
  }

  private calculateScore(suggestion: Suggestion, input: string): number {
    let score = suggestion.score;

    // Exact prefix match
    if (suggestion.text.startsWith(input)) {
      score += 100;
    }

    // Fuzzy match
    if (this.config.fuzzyMatch && this.fuzzyMatch(suggestion.text, input)) {
      score += 50;
    }

    // Recency for history
    if (suggestion.type === 'history') {
      score += 20;
    }

    // Command priority
    if (suggestion.type === 'command') {
      score += 30;
    }

    return score;
  }

  private fuzzyMatch(text: string, pattern: string): boolean {
    let patternIdx = 0;
    for (const char of text.toLowerCase()) {
      if (char === pattern[patternIdx]?.toLowerCase()) {
        patternIdx++;
      }
    }
    return patternIdx === pattern.length;
  }
}

// Providers
class CommandSuggestionProvider implements SuggestionProvider {
  async suggest(context: SuggestionContext): Promise<Suggestion[]> {
    const commands = [
      { name: '/help', description: 'Show help' },
      { name: '/sessions', description: 'List sessions' },
      { name: '/resume', description: 'Resume session' },
      { name: '/new', description: 'New session' },
      { name: '/clear', description: 'Clear screen' },
      { name: '/tasks', description: 'Show background tasks' },
      { name: '/costs', description: 'Show cost report' },
      { name: '/plugin', description: 'Manage plugins' },
      // ... more commands
    ];

    return commands
      .filter(c => c.name.startsWith(context.input) || this.fuzzyMatch(c.name, context.input))
      .map(c => ({
        text: c.name,
        description: c.description,
        type: 'command' as const,
        score: 50,
        source: 'commands'
      }));
  }
}

class PathSuggestionProvider implements SuggestionProvider {
  async suggest(context: SuggestionContext): Promise<Suggestion[]> {
    // Only suggest paths if input looks like a path
    if (!context.input.includes('/') && !context.input.startsWith('.')) {
      return [];
    }

    const dir = path.dirname(context.input) || '.';
    const prefix = path.basename(context.input);

    try {
      const entries = await fs.readdir(path.resolve(context.cwd, dir));
      return entries
        .filter(e => e.startsWith(prefix))
        .slice(0, 10)
        .map(e => ({
          text: path.join(dir, e),
          type: 'path' as const,
          score: 30,
          source: 'filesystem'
        }));
    } catch {
      return [];
    }
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/cli/suggestions/types.ts` | Create | Type definitions |
| `src/cli/suggestions/engine.ts` | Create | Core engine |
| `src/cli/suggestions/providers/*.ts` | Create | Suggestion providers |
| `src/cli/input.ts` | Modify | Integrate suggestions |
| `src/cli/ui.ts` | Modify | Display suggestions |

## User Experience

### Tab Completion
```
> /ses<Tab>

  /sessions     List sessions
  /session-info Show session details
```

### Fuzzy Matching
```
> /rsm<Tab>

  /resume       Resume a session (fuzzy match: rsm → resume)
```

### Path Completion
```
> Read src/comp<Tab>

  src/components/
  src/compiler/
```

### Inline Suggestions
```
> /plug
       ↓
  ┌─────────────────────────────────┐
  │ /plugin list    List plugins    │
  │ /plugin install Install plugin  │
  │ /plugin enable  Enable plugin   │
  └─────────────────────────────────┘
```

## Alternatives Considered

### Alternative 1: No Autocomplete
Rely on documentation.

**Pros**: Simpler
**Cons**: Poor UX
**Decision**: Rejected

### Alternative 2: Full TUI
Rich terminal UI with menus.

**Pros**: More discoverable
**Cons**: Complex, heavy
**Decision**: Deferred

## Security Considerations

1. Path suggestions limited to cwd
2. No secret exposure in suggestions
3. History filtering for sensitive commands

## Testing Strategy

1. Unit tests for matching logic
2. Integration tests for providers
3. Manual testing for UX

## Migration Path

1. **Phase 1**: Command completion
2. **Phase 2**: Path completion
3. **Phase 3**: History suggestions
4. **Phase 4**: Fuzzy matching
5. **Phase 5**: Inline display

## References

- [readline - Node.js](https://nodejs.org/api/readline.html)
- [Fuse.js - Fuzzy Search](https://fusejs.io/)
