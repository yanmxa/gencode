# Proposal: Plugin System

- **Proposal ID**: 0022
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a plugin system that allows packaging and distributing collections of skills, commands, and subagents. Plugins enable community contributions and organizational customization of mycode capabilities.

## Motivation

Currently, mycode has no mechanism for distributing extensions:

1. **No packaging**: Can't bundle related capabilities
2. **No versioning**: No way to track capability versions
3. **No distribution**: Can't share with others
4. **No discovery**: Hard to find available extensions
5. **Manual installation**: Copy/paste files to add features

A plugin system enables ecosystem growth and easy extensibility.

## Claude Code Reference

Claude Code uses a plugin system with marketplace support:

### Plugin Structure
```
plugin-name/
├── .claude-plugin/
│   └── plugin.json          # Plugin manifest
├── commands/
│   ├── command-1.md
│   └── command-2.md
├── skills/
│   └── skill-name/
│       └── SKILL.md
├── agents/
│   └── subagent-name.md
└── README.md
```

### Plugin Manifest (plugin.json)
```json
{
  "name": "jira-tools",
  "description": "Jira management and automation tools",
  "version": "1.0.0",
  "author": {
    "name": "Team Name"
  },
  "repository": "https://github.com/org/repo"
}
```

### Marketplace Configuration
```json
{
  "cc-plugins": {
    "type": "local",
    "path": "/path/to/plugins"
  },
  "remote-plugins": {
    "type": "github",
    "source": "org/repo",
    "cache_dir": "~/.mycode/plugins/cache/remote"
  }
}
```

### Plugin Activation
```json
{
  "enabledPlugins": {
    "jira-tools@remote-plugins": true,
    "git@cc-plugins": true
  }
}
```

## Detailed Design

### API Design

```typescript
// src/plugins/types.ts
interface PluginManifest {
  name: string;
  description: string;
  version: string;
  author?: {
    name: string;
    email?: string;
    url?: string;
  };
  repository?: string;
  homepage?: string;
  keywords?: string[];
  dependencies?: Record<string, string>;
}

interface Plugin {
  manifest: PluginManifest;
  commands: CommandDefinition[];
  skills: SkillDefinition[];
  agents: AgentDefinition[];
  path: string;
  marketplace: string;
}

interface Marketplace {
  id: string;
  type: 'local' | 'github' | 'npm';
  source: string;           // path for local, "org/repo" for github
  cacheDir?: string;
}

interface PluginRegistry {
  marketplaces: Map<string, Marketplace>;
  plugins: Map<string, Plugin>;      // "name@marketplace" -> Plugin
  enabled: Set<string>;              // Enabled plugin IDs
}
```

### Plugin Manager

```typescript
// src/plugins/manager.ts
class PluginManager {
  private marketplaces: Map<string, Marketplace> = new Map();
  private plugins: Map<string, Plugin> = new Map();
  private enabled: Set<string> = new Set();
  private cacheDir: string;

  constructor(cacheDir = '~/.mycode/plugins/cache') {
    this.cacheDir = expandPath(cacheDir);
    this.loadMarketplaces();
    this.loadEnabledPlugins();
  }

  private loadMarketplaces(): void {
    const configPath = expandPath('~/.mycode/plugins/marketplaces.json');
    if (fs.existsSync(configPath)) {
      const config = JSON.parse(fs.readFileSync(configPath, 'utf-8'));
      for (const [id, marketplace] of Object.entries(config)) {
        this.marketplaces.set(id, marketplace as Marketplace);
      }
    }

    // Add default local marketplace
    this.marketplaces.set('local', {
      id: 'local',
      type: 'local',
      source: expandPath('~/.mycode/plugins')
    });
  }

  async install(pluginId: string): Promise<Plugin> {
    const [name, marketplaceId] = this.parsePluginId(pluginId);
    const marketplace = this.marketplaces.get(marketplaceId);

    if (!marketplace) {
      throw new Error(`Marketplace not found: ${marketplaceId}`);
    }

    let plugin: Plugin;

    switch (marketplace.type) {
      case 'local':
        plugin = await this.loadLocalPlugin(marketplace, name);
        break;
      case 'github':
        plugin = await this.fetchGitHubPlugin(marketplace, name);
        break;
      case 'npm':
        plugin = await this.fetchNpmPlugin(marketplace, name);
        break;
    }

    this.plugins.set(pluginId, plugin);
    return plugin;
  }

  async enable(pluginId: string): Promise<void> {
    if (!this.plugins.has(pluginId)) {
      await this.install(pluginId);
    }
    this.enabled.add(pluginId);
    this.saveEnabledPlugins();
  }

  async disable(pluginId: string): Promise<void> {
    this.enabled.delete(pluginId);
    this.saveEnabledPlugins();
  }

  private async loadLocalPlugin(marketplace: Marketplace, name: string): Promise<Plugin> {
    const pluginPath = path.join(marketplace.source, name);
    const manifestPath = path.join(pluginPath, '.claude-plugin', 'plugin.json');

    if (!fs.existsSync(manifestPath)) {
      throw new Error(`Plugin manifest not found: ${manifestPath}`);
    }

    const manifest: PluginManifest = JSON.parse(
      fs.readFileSync(manifestPath, 'utf-8')
    );

    return {
      manifest,
      commands: await this.loadCommands(path.join(pluginPath, 'commands')),
      skills: await this.loadSkills(path.join(pluginPath, 'skills')),
      agents: await this.loadAgents(path.join(pluginPath, 'agents')),
      path: pluginPath,
      marketplace: marketplace.id
    };
  }

  private async fetchGitHubPlugin(marketplace: Marketplace, name: string): Promise<Plugin> {
    const cacheDir = path.join(
      this.cacheDir,
      marketplace.id,
      name
    );

    // Check cache
    if (fs.existsSync(cacheDir)) {
      return this.loadLocalPlugin({ ...marketplace, source: cacheDir }, '.');
    }

    // Fetch from GitHub
    const [owner, repo] = marketplace.source.split('/');
    const pluginPath = `plugins/${name}`;

    await this.downloadFromGitHub(owner, repo, pluginPath, cacheDir);

    return this.loadLocalPlugin({ ...marketplace, source: cacheDir }, '.');
  }

  getEnabledPlugins(): Plugin[] {
    return Array.from(this.enabled)
      .map(id => this.plugins.get(id))
      .filter((p): p is Plugin => p !== undefined);
  }

  getAllCommands(): CommandDefinition[] {
    return this.getEnabledPlugins()
      .flatMap(p => p.commands.map(c => ({
        ...c,
        fullName: `${p.manifest.name}:${c.name}`
      })));
  }

  getAllSkills(): SkillDefinition[] {
    return this.getEnabledPlugins()
      .flatMap(p => p.skills.map(s => ({
        ...s,
        fullName: `${p.manifest.name}:${s.name}`
      })));
  }
}

export const pluginManager = new PluginManager();
```

### CLI Commands

```typescript
// src/cli/commands/plugin.ts
const pluginCommands = {
  '/plugin list': 'List all available plugins',
  '/plugin install <name@marketplace>': 'Install a plugin',
  '/plugin enable <name@marketplace>': 'Enable a plugin',
  '/plugin disable <name@marketplace>': 'Disable a plugin',
  '/plugin info <name@marketplace>': 'Show plugin details',
  '/plugin update [name@marketplace]': 'Update plugin(s)',
  '/plugin create <name>': 'Create new plugin template'
};

async function handlePluginCommand(args: string[]): Promise<void> {
  const [subcommand, ...rest] = args;

  switch (subcommand) {
    case 'list':
      const plugins = pluginManager.getAllPlugins();
      printPluginTable(plugins);
      break;

    case 'install':
      const installed = await pluginManager.install(rest[0]);
      console.log(`Installed: ${installed.manifest.name}@${installed.manifest.version}`);
      break;

    case 'enable':
      await pluginManager.enable(rest[0]);
      console.log(`Enabled: ${rest[0]}`);
      break;

    // ... other subcommands
  }
}
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/plugins/types.ts` | Create | Type definitions |
| `src/plugins/manager.ts` | Create | Plugin lifecycle management |
| `src/plugins/loader.ts` | Create | Plugin loading utilities |
| `src/plugins/marketplace.ts` | Create | Marketplace interaction |
| `src/plugins/index.ts` | Create | Module exports |
| `src/cli/commands/plugin.ts` | Create | Plugin CLI commands |
| `src/skills/registry.ts` | Modify | Load skills from plugins |

## User Experience

### List Plugins
```
User: /plugin list

Installed Plugins:
┌────────────────────────────────────────────────────────────────┐
│ Plugin           Version  Marketplace  Status    Components   │
├────────────────────────────────────────────────────────────────┤
│ jira-tools       1.2.0    acm-plugins  enabled   3 cmd, 2 skl │
│ git              0.5.0    local        enabled   5 cmd, 1 skl │
│ test-helpers     1.0.0    npm          disabled  2 cmd        │
└────────────────────────────────────────────────────────────────┘

Use /plugin info <name> for details
```

### Install Plugin
```
User: /plugin install code-quality@npm

Fetching code-quality from npm registry...
Installing version 2.1.0...

✓ Installed: code-quality@2.1.0
  Components:
  - Commands: lint, format, analyze
  - Skills: code-reviewer
  - Agents: (none)

Enable with: /plugin enable code-quality@npm
```

### Plugin Details
```
User: /plugin info jira-tools@acm-plugins

┌─ jira-tools ──────────────────────────────────────┐
│ Version: 1.2.0                                    │
│ Author: ACM QE Team                               │
│ Repository: github.com/org/acm-workflows          │
│                                                   │
│ Description:                                      │
│ Comprehensive Jira management and automation      │
│ tools for Red Hat ACM workflows.                  │
│                                                   │
│ Commands:                                         │
│ • /jira:my-issues - List assigned issues         │
│ • /jira:sprint-issues - Sprint overview          │
│ • /jira:add-labels - Add labels to issues        │
│                                                   │
│ Skills:                                           │
│ • jira-analyzer - Complexity assessment          │
│ • test-plan-generator - Generate test plans      │
│                                                   │
│ Status: Enabled                                   │
└───────────────────────────────────────────────────┘
```

### Create Plugin Template
```
User: /plugin create my-tools

Created plugin template at:
  ~/.mycode/plugins/my-tools/

Structure:
  .claude-plugin/plugin.json
  commands/
  skills/
  agents/
  README.md

Edit plugin.json to configure, then:
  /plugin enable my-tools@local
```

## Alternatives Considered

### Alternative 1: NPM-only Distribution
Use npm for all plugins.

**Pros**: Established ecosystem
**Cons**: Requires npm account, heavier
**Decision**: Support NPM as one option

### Alternative 2: Git Submodules
Use git submodules for plugins.

**Pros**: Version control integrated
**Cons**: Complex for users
**Decision**: Rejected - Too complex

### Alternative 3: Single File Plugins
Package plugins as single files.

**Pros**: Simpler distribution
**Cons**: Limited capabilities
**Decision**: Rejected - Need multi-file support

## Security Considerations

1. **Source Verification**: Verify plugin sources
2. **Sandboxing**: Limit plugin capabilities
3. **Permission Review**: Show required permissions on install
4. **Version Pinning**: Support exact version requirements
5. **Signature Verification**: Optional plugin signing

```typescript
interface SecurityPolicy {
  requireSignature: boolean;
  allowedMarketplaces: string[];
  blockedPlugins: string[];
  maxPlugins: number;
}
```

## Testing Strategy

1. **Unit Tests**:
   - Manifest parsing
   - Plugin loading
   - Marketplace fetching
   - Version resolution

2. **Integration Tests**:
   - Install/enable/disable flow
   - Multi-marketplace scenarios
   - Plugin conflicts

3. **Manual Testing**:
   - Various plugin types
   - Error scenarios
   - Update workflow

## Migration Path

1. **Phase 1**: Local plugin support
2. **Phase 2**: GitHub marketplace
3. **Phase 3**: NPM marketplace
4. **Phase 4**: Plugin creation tools
5. **Phase 5**: Marketplace browser

## References

- [Claude Code Plugin System](https://code.claude.com/docs/en/plugins)
- [VSCode Extension API](https://code.visualstudio.com/api)
- [npm Package Specification](https://docs.npmjs.com/about-packages-and-modules)
- [Skills System Proposal (0021)](./0021-skills-system.md)
