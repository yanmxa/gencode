# Proposal: Configuration System

- **Proposal ID**: 0041
- **Author**: gencode team
- **Status**: Implemented
- **Created**: 2026-01-15
- **Updated**: 2026-01-16
- **Implemented**: 2026-01-16

## Summary

Implement a comprehensive configuration system for gencode, supporting multi-level configuration loading (user and project level), environment variable handling, permission management, and configuration merging. This design draws from both Claude Code's `~/.claude/` directory structure and OpenCode's configuration patterns.

## Motivation

A robust configuration system is essential for:

1. **Personalization**: Users need to customize behavior, model selection, and permissions
2. **Project-specific settings**: Different projects may require different configurations
3. **Team collaboration**: Shared project settings can be version controlled
4. **Security**: Sensitive local settings should be gitignored while shared settings remain portable
5. **Provider flexibility**: gencode's multi-provider architecture needs clean configuration for API keys and provider selection

## Claude Code Reference

### Configuration File Hierarchy

Claude Code uses a layered configuration system with the following priority (high to low):

| Level | Location | Purpose | Git Tracked |
|-------|----------|---------|-------------|
| 1 | `~/.claude.json` | Legacy main config | No |
| 2 | `~/.claude/settings.json` | User global settings | No |
| 3 | `~/.claude/settings.local.json` | User local settings | No |
| 4 | `.claude/settings.json` | Project shared settings | Yes |
| 5 | `.claude/settings.local.json` | Project personal settings | No (gitignored) |
| 6 | `.mcp.json` | Project MCP servers | Yes |

### Environment Variables

Claude Code supports extensive environment variables:

**Authentication & API:**
- `ANTHROPIC_API_KEY` - Primary API key
- `ANTHROPIC_AUTH_TOKEN` - Alternative token
- `ANTHROPIC_BASE_URL` - Custom API endpoint
- `ANTHROPIC_MODEL` - Default model selection
- `ANTHROPIC_SMALL_FAST_MODEL` - Fast model for quick operations

**Cloud Providers:**
- `CLAUDE_CODE_USE_BEDROCK` - Enable AWS Bedrock
- `CLAUDE_CODE_SKIP_BEDROCK_AUTH` - Bypass AWS auth

**Operational:**
- `CLAUDE_CODE_MAX_OUTPUT_TOKENS` - Token limit
- `CLAUDE_CODE_ACTION` - Permission mode (acceptEdits, plan, bypassPermissions)
- `FORCE_CODE_TERMINAL` - Force CLI mode

**Debugging:**
- `DEBUG` - Verbose logging
- `DISABLE_ERROR_REPORTING` - No error submission
- `DISABLE_TELEMETRY` - No usage tracking

### settings.json Structure

```json
{
  "projects": {
    "/path/to/project": {
      "mcpServers": { },
      "allowedTools": [ ]
    }
  },
  "permissions": {
    "allow": ["Bash(npm:*)", "Read(/path/**)"],
    "deny": ["Read(./.env)"]
  },
  "model": "claude-opus-4-5-20251101",
  "spinnerTipsEnabled": false,
  "attribution": {
    "commits": true,
    "pullRequests": true
  },
  "mcpServers": { },
  "enableAllProjectMcpServers": true
}
```

### Tool Permission Patterns

- `Bash(git log:*)` - Git commands
- `Bash(npm run:*)` - NPM scripts
- `Read(/path/**)` - File patterns with globs
- `WebFetch(domain:github.com)` - Domain restrictions
- Tool names: `Task`, `Glob`, `Grep`, `LS`, `Edit`, `MultiEdit`, `Write`, `WebSearch`

## OpenCode Reference

OpenCode provides additional patterns worth adopting:

### Configuration Loading Order (Low → High Priority)

1. Remote config (`.well-known/opencode`)
2. Global config (`~/.config/opencode/opencode.json`)
3. `OPENCODE_CONFIG` env var file
4. Project config (`opencode.json`)
5. `OPENCODE_CONFIG_CONTENT` inline JSON

### Key Features

- **JSON/JSONC support**: Comments and trailing commas allowed
- **Environment variable substitution**: `{env:VARIABLE_NAME}` syntax
- **File content inclusion**: `{file:path/to/file}` syntax
- **Deep merge**: Arrays concatenate rather than replace
- **Zod schema validation**: Runtime type checking

## Detailed Design

### Directory Structure

```
~/.gen/                          # User-level configuration
├── settings.json                    # Main user config
├── settings.local.json              # User local overrides (gitignored pattern)
├── GENCODE.md                       # User context (like CLAUDE.md)
├── commands/                        # Custom slash commands
├── skills/                          # Custom skills
├── agents/                          # Custom subagents
├── plugins/                         # Plugin management
│   ├── marketplaces.json
│   └── installed.json
├── hooks/                           # Event hooks
└── sessions/                        # Session data

./gencode.json                       # Project config (like opencode.json)
./.gen/                          # Project directory
├── settings.local.json              # Project local overrides (gitignored)
├── GENCODE.md                       # Project context
├── rules/                           # Path-scoped rules
└── skills/                          # Project-specific skills
```

### Configuration Priority (High → Low)

1. **Environment variables** (`GENCODE_*`, provider API keys)
2. **CLI arguments** (`--model`, `--provider`)
3. **Project local** (`./.gen/settings.local.json`)
4. **Project shared** (`./gencode.json`)
5. **User local** (`~/.gen/settings.local.json`)
6. **User global** (`~/.gen/settings.json`)
7. **Defaults**

### API Design

```typescript
// src/config/types.ts
interface GencodeConfig {
  // Provider configuration
  provider?: 'anthropic' | 'openai' | 'gemini' | 'bedrock' | 'vertex';
  model?: string;

  // Permission system
  permissions?: {
    allow?: string[];  // e.g., ["Bash(npm:*)", "Read(**/src/**)"]
    deny?: string[];   // e.g., ["Read(.env)", "Bash(rm -rf:*)"]
  };

  // Environment variables to inject
  env?: Record<string, string>;

  // Hook definitions
  hooks?: {
    preToolUse?: HookConfig[];
    postToolUse?: HookConfig[];
    notification?: HookConfig[];
  };

  // MCP server configuration
  mcpServers?: Record<string, McpServerConfig>;

  // Plugin enablement
  enabledPlugins?: string[];

  // UI preferences
  theme?: 'dark' | 'light' | 'auto';
  spinnerTipsEnabled?: boolean;

  // Attribution settings
  attribution?: {
    commits?: boolean;
    pullRequests?: boolean;
  };
}

interface HookConfig {
  tool?: string;
  command: string;
  timeout?: number;
}

interface McpServerConfig {
  command: string;
  args?: string[];
  env?: Record<string, string>;
}

// src/config/loader.ts
interface ConfigLoader {
  load(): Promise<GencodeConfig>;
  getUserConfig(): Promise<GencodeConfig>;
  getProjectConfig(): Promise<GencodeConfig>;
  getEffectiveConfig(): Promise<GencodeConfig>;
  watch(callback: (config: GencodeConfig) => void): void;
}

// src/config/env.ts
interface EnvHandler {
  getProviderFromEnv(): string | undefined;
  getModelFromEnv(): string | undefined;
  getApiKey(provider: string): string | undefined;
  substituteEnvVars(value: string): string;
}
```

### Environment Variables

**Provider Selection:**
- `GEN_PROVIDER` - Provider name (anthropic, openai, gemini, bedrock, vertex)
- `GEN_MODEL` - Model ID
- `GENCODE_CONFIG` - Custom config file path

**Provider API Keys (Auto-detect):**
- `ANTHROPIC_API_KEY`
- `OPENAI_API_KEY`
- `GOOGLE_API_KEY`
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (for Bedrock)

**Operational:**
- `GENCODE_MAX_OUTPUT_TOKENS` - Token limit
- `GENCODE_DISABLE_TELEMETRY` - Disable tracking
- `GENCODE_DEBUG` - Debug mode
- `HTTP_PROXY` / `HTTPS_PROXY` - Network proxy

### settings.json Schema

```json
{
  "$schema": "https://gencode.dev/settings.schema.json",
  "provider": "anthropic",
  "model": "claude-sonnet-4",
  "permissions": {
    "allow": ["Bash(npm:*)", "Read(**/src/**)"],
    "deny": ["Read(.env)", "Bash(rm -rf:*)"]
  },
  "env": {
    "CUSTOM_VAR": "value",
    "API_URL": "{env:BASE_API_URL}/v1"
  },
  "hooks": {
    "postToolUse": [
      { "tool": "Write", "command": "make fmt" }
    ]
  },
  "mcpServers": {
    "memory": {
      "command": "npx",
      "args": ["@modelcontextprotocol/server-memory"]
    }
  },
  "enabledPlugins": ["git@official", "jira@community"],
  "theme": "dark",
  "spinnerTipsEnabled": true,
  "attribution": {
    "commits": true,
    "pullRequests": true
  }
}
```

### Implementation Approach

#### Phase 1: Core Config Loading

```typescript
// src/config/loader.ts
import { parse as parseJsonc } from 'jsonc-parser';
import { findUp } from 'find-up';
import { z } from 'zod';

const ConfigSchema = z.object({
  provider: z.enum(['anthropic', 'openai', 'gemini', 'bedrock', 'vertex']).optional(),
  model: z.string().optional(),
  permissions: z.object({
    allow: z.array(z.string()).optional(),
    deny: z.array(z.string()).optional(),
  }).optional(),
  // ... rest of schema
});

export class ConfigLoader {
  private userConfigDir = path.join(os.homedir(), '.gen');

  async load(): Promise<GencodeConfig> {
    const configs = await Promise.all([
      this.loadDefaults(),
      this.loadUserGlobal(),
      this.loadUserLocal(),
      this.loadProjectShared(),
      this.loadProjectLocal(),
    ]);

    return this.deepMerge(...configs);
  }

  private async loadFile(filePath: string): Promise<Partial<GencodeConfig>> {
    try {
      const content = await fs.readFile(filePath, 'utf-8');
      const parsed = parseJsonc(content); // Supports comments
      return ConfigSchema.partial().parse(parsed);
    } catch {
      return {};
    }
  }

  private deepMerge(...configs: Partial<GencodeConfig>[]): GencodeConfig {
    // Arrays concatenate, objects merge recursively
    return configs.reduce((acc, config) => {
      return mergeWith(acc, config, (objValue, srcValue) => {
        if (Array.isArray(objValue) && Array.isArray(srcValue)) {
          return [...objValue, ...srcValue]; // Concatenate arrays
        }
      });
    }, {} as GencodeConfig);
  }
}
```

#### Phase 2: Environment Variable Handling

```typescript
// src/config/env.ts
export class EnvHandler {
  private readonly providerKeyMap: Record<string, string> = {
    anthropic: 'ANTHROPIC_API_KEY',
    openai: 'OPENAI_API_KEY',
    gemini: 'GOOGLE_API_KEY',
  };

  getProviderFromEnv(): string | undefined {
    return process.env.GEN_PROVIDER;
  }

  getApiKey(provider: string): string | undefined {
    const envKey = this.providerKeyMap[provider];
    return envKey ? process.env[envKey] : undefined;
  }

  // Substitute {env:VAR} patterns
  substituteEnvVars(value: string): string {
    return value.replace(/\{env:(\w+)\}/g, (_, varName) => {
      return process.env[varName] || '';
    });
  }

  autoDetectProvider(): string | undefined {
    for (const [provider, key] of Object.entries(this.providerKeyMap)) {
      if (process.env[key]) {
        return provider;
      }
    }
    return undefined;
  }
}
```

#### Phase 3: Permission Matching

```typescript
// src/config/permissions.ts
import { minimatch } from 'minimatch';

export class PermissionMatcher {
  constructor(private config: GencodeConfig) {}

  isAllowed(tool: string, args: string): boolean {
    const permission = `${tool}(${args})`;

    // Check deny list first
    for (const pattern of this.config.permissions?.deny || []) {
      if (this.matchPermission(permission, pattern)) {
        return false;
      }
    }

    // Check allow list
    for (const pattern of this.config.permissions?.allow || []) {
      if (this.matchPermission(permission, pattern)) {
        return true;
      }
    }

    return false; // Default deny
  }

  private matchPermission(permission: string, pattern: string): boolean {
    // Parse pattern: Tool(arg:pattern)
    const match = pattern.match(/^(\w+)\((.+)\)$/);
    if (!match) return false;

    const [, toolPattern, argPattern] = match;
    const [tool, arg] = permission.match(/^(\w+)\((.+)\)$/)?.slice(1) || [];

    if (toolPattern !== tool && toolPattern !== '*') return false;
    return minimatch(arg, argPattern);
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/config/types.ts` | Create | Configuration type definitions |
| `src/config/loader.ts` | Create | Multi-level config loading |
| `src/config/env.ts` | Create | Environment variable handling |
| `src/config/permissions.ts` | Create | Permission pattern matching |
| `src/config/index.ts` | Create | Public API exports |
| `src/agent/agent.ts` | Modify | Integrate config system |
| `src/cli/index.ts` | Modify | Add --config CLI flag |
| `schemas/settings.schema.json` | Create | JSON Schema for validation |

## User Experience

### First-time Setup

```bash
# gencode auto-detects provider from API keys
$ export OPENAI_API_KEY=sk-...
$ gencode
# Uses OpenAI automatically

# Or specify explicitly
$ export GEN_PROVIDER=anthropic
$ export ANTHROPIC_API_KEY=sk-ant-...
$ gencode
```

### Project Configuration

```bash
# Initialize project config
$ gencode init
# Creates ./gencode.json with sensible defaults

# View effective configuration
$ gencode config
# Shows merged configuration from all sources

# Set project-specific model
$ gencode config set model claude-sonnet-4
# Updates ./gencode.json
```

### Permission Management

```bash
# View current permissions
$ gencode /permissions

# Allow npm commands for this project
$ gencode config allow "Bash(npm:*)"

# Deny reading .env files
$ gencode config deny "Read(.env)"
```

## Alternatives Considered

### Single Config File

A single `~/.genrc` file would be simpler but lacks:
- Project-specific overrides
- Team-shareable settings
- Local-only sensitive settings

### YAML Configuration

YAML is more readable but:
- JSON has better tooling support
- JSONC provides comments without complexity
- Matches Claude Code and OpenCode patterns

### No Environment Variable Substitution

Simpler implementation but:
- Forces duplication of values
- Makes CI/CD integration harder
- Loses flexibility for sensitive values

## Security Considerations

1. **API Key Handling**: Never log or expose API keys; load from environment only
2. **Local Settings**: `settings.local.json` files should be gitignored
3. **Permission Defaults**: Default to deny; require explicit allow
4. **File Permissions**: Config files should be user-readable only (0600)
5. **Command Injection**: Validate hook commands; sanitize environment variable substitution

## Testing Strategy

1. **Unit Tests**:
   - Config loading from multiple sources
   - Deep merge behavior (especially arrays)
   - Environment variable substitution
   - Permission pattern matching

2. **Integration Tests**:
   - Full config resolution with mock file system
   - CLI flag override behavior
   - Provider auto-detection

3. **E2E Tests**:
   - `gencode config` commands
   - Permission enforcement during tool execution

## Migration Path

1. **From mycode**: Migrate `~/.mycode/` to `~/.gen/`
2. **Version Detection**: Check for legacy config locations and prompt migration
3. **Backward Compatibility**: Support reading old config format for one major version

## Dependencies

- [find-up](https://www.npmjs.com/package/find-up) - Directory traversal
- [jsonc-parser](https://www.npmjs.com/package/jsonc-parser) - JSON with comments
- [minimatch](https://www.npmjs.com/package/minimatch) - Glob pattern matching
- [zod](https://www.npmjs.com/package/zod) - Schema validation (already used)
- [lodash.mergewith](https://www.npmjs.com/package/lodash.mergewith) - Deep merge

## Related Proposals

| Proposal | Relationship |
|----------|--------------|
| [0006 Memory System](./0006-memory-system.md) | GENCODE.md is stored in config directories |
| [0009 Hooks System](./0009-hooks-system.md) | Hook definitions stored in settings.json |
| [0022 Plugin System](./0022-plugin-system.md) | Plugin enablement configured in settings |
| [0023 Permission Enhancements](./0023-permission-enhancements.md) | Permission patterns defined in settings |

## References

- [Claude Code Settings - Official Docs](https://code.claude.com/docs/en/settings)
- [Claude Code Configuration Guide | ClaudeLog](https://claudelog.com/configuration/)
- [Claude Code CLI Environment Variables](https://gist.github.com/unkn0wncode/f87295d055dd0f0e8082358a0b5cc467)
- [settings.json in Claude Code Guide](https://www.eesel.ai/blog/settings-json-claude-code)
- [OpenCode Config Docs](https://opencode.ai/docs/config/)
- [OpenCode Configuration System | DeepWiki](https://deepwiki.com/sst/opencode/3-configuration-system)

## Implementation Notes

### Files Created/Modified

| File | Action | Description |
|------|--------|-------------|
| `src/config/types.ts` | Created | Configuration type definitions with Zod schemas |
| `src/config/loader.ts` | Created | Multi-level config loading with deep merge |
| `src/config/env.ts` | Created | Environment variable handling and provider auto-detection |
| `src/config/index.ts` | Created | Public API exports |
| `src/agent/agent.ts` | Modified | Integrated config system |

### Key Implementation Details

1. **Multi-level Loading**: User (`~/.gen/`) → Project (`./.gen/`) with proper merge
2. **Claude Code Compatibility**: Supports both `GENCODE.md` and `CLAUDE.md` for memory files
3. **Environment Variables**: `GEN_PROVIDER`, `GEN_MODEL`, and provider API key auto-detection
4. **Deep Merge**: Arrays concatenate, objects merge recursively
5. **JSONC Support**: Configuration files support comments and trailing commas

### Configuration Priority (High → Low)

1. Environment variables (`GENCODE_*`)
2. CLI arguments
3. Project local (`./.gen/settings.local.json`)
4. Project shared (`./gencode.json`)
5. User local (`~/.gen/settings.local.json`)
6. User global (`~/.gen/settings.json`)
7. Defaults
