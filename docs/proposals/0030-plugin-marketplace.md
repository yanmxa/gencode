# Proposal: Plugin Marketplace

- **Proposal ID**: 0030
- **Author**: mycode team
- **Status**: Draft
- **Created**: 2025-01-15
- **Updated**: 2025-01-15

## Summary

Implement a plugin marketplace system for discovering, installing, and updating plugins from multiple sources. This enables community-driven extension of mycode capabilities.

## Motivation

With the plugin system (0022) in place, users need:

1. **Discovery**: Find available plugins
2. **Easy installation**: One-command install
3. **Updates**: Keep plugins current
4. **Trust**: Verify plugin sources
5. **Community**: Share and contribute

A marketplace enables ecosystem growth.

## Claude Code Reference

Claude Code uses a multi-marketplace architecture:

### Known Marketplaces
```json
{
  "cc-plugins": {
    "type": "local",
    "path": "/path/to/local/plugins"
  },
  "acm-workflows-plugins": {
    "type": "github",
    "source": "stolostron/acm-workflows"
  },
  "claude-plugins-official": {
    "type": "github",
    "source": "anthropics/claude-plugins-official"
  }
}
```

### Plugin Activation
```json
{
  "enabledPlugins": {
    "jira-tools@acm-workflows-plugins": true,
    "git@cc-plugins": true
  }
}
```

## Detailed Design

### API Design

```typescript
// src/marketplace/types.ts
interface Marketplace {
  id: string;
  name: string;
  type: 'local' | 'github' | 'npm' | 'registry';
  source: string;
  cacheDir?: string;
  priority?: number;
  enabled: boolean;
}

interface MarketplacePlugin {
  name: string;
  version: string;
  description: string;
  author: string;
  repository?: string;
  homepage?: string;
  keywords?: string[];
  downloads?: number;
  rating?: number;
  lastUpdated: string;
  marketplace: string;
}

interface PluginSearchResult {
  plugins: MarketplacePlugin[];
  total: number;
  page: number;
  pageSize: number;
}

interface PluginInstallResult {
  success: boolean;
  plugin?: MarketplacePlugin;
  version: string;
  error?: string;
}
```

### Marketplace Manager

```typescript
// src/marketplace/manager.ts
class MarketplaceManager {
  private marketplaces: Map<string, Marketplace> = new Map();
  private cache: PluginCache;

  constructor() {
    this.loadMarketplaces();
    this.cache = new PluginCache();
  }

  async search(
    query: string,
    options?: { marketplace?: string; limit?: number }
  ): Promise<PluginSearchResult> {
    const results: MarketplacePlugin[] = [];

    const markets = options?.marketplace
      ? [this.marketplaces.get(options.marketplace)].filter(Boolean)
      : Array.from(this.marketplaces.values());

    for (const market of markets) {
      if (!market?.enabled) continue;

      const plugins = await this.searchMarketplace(market, query);
      results.push(...plugins);
    }

    // Sort by relevance
    results.sort((a, b) => this.scoreRelevance(b, query) - this.scoreRelevance(a, query));

    const limit = options?.limit || 20;
    return {
      plugins: results.slice(0, limit),
      total: results.length,
      page: 1,
      pageSize: limit
    };
  }

  async install(
    pluginId: string,
    version?: string
  ): Promise<PluginInstallResult> {
    const [name, marketplaceId] = this.parsePluginId(pluginId);
    const marketplace = this.marketplaces.get(marketplaceId);

    if (!marketplace) {
      return { success: false, error: `Marketplace not found: ${marketplaceId}`, version: '' };
    }

    const plugin = await this.fetchPlugin(marketplace, name, version);
    if (!plugin) {
      return { success: false, error: `Plugin not found: ${name}`, version: '' };
    }

    await this.downloadAndCache(marketplace, plugin);

    return {
      success: true,
      plugin,
      version: plugin.version
    };
  }

  async update(pluginId?: string): Promise<UpdateResult[]> {
    const results: UpdateResult[] = [];

    if (pluginId) {
      // Update specific plugin
      const result = await this.updatePlugin(pluginId);
      results.push(result);
    } else {
      // Update all installed plugins
      const installed = await this.getInstalledPlugins();
      for (const plugin of installed) {
        const result = await this.updatePlugin(`${plugin.name}@${plugin.marketplace}`);
        results.push(result);
      }
    }

    return results;
  }

  async addMarketplace(marketplace: Marketplace): Promise<void> {
    this.marketplaces.set(marketplace.id, marketplace);
    await this.saveMarketplaces();
  }

  async removeMarketplace(id: string): Promise<void> {
    this.marketplaces.delete(id);
    await this.saveMarketplaces();
  }

  private async searchMarketplace(
    marketplace: Marketplace,
    query: string
  ): Promise<MarketplacePlugin[]> {
    switch (marketplace.type) {
      case 'local':
        return this.searchLocal(marketplace, query);
      case 'github':
        return this.searchGitHub(marketplace, query);
      case 'npm':
        return this.searchNpm(marketplace, query);
      case 'registry':
        return this.searchRegistry(marketplace, query);
      default:
        return [];
    }
  }

  private async searchGitHub(
    marketplace: Marketplace,
    query: string
  ): Promise<MarketplacePlugin[]> {
    const [owner, repo] = marketplace.source.split('/');

    // Fetch plugin index from repo
    const indexUrl = `https://raw.githubusercontent.com/${owner}/${repo}/main/plugins/index.json`;

    try {
      const response = await fetch(indexUrl);
      const index = await response.json() as { plugins: MarketplacePlugin[] };

      return index.plugins.filter(p =>
        p.name.includes(query) ||
        p.description.toLowerCase().includes(query.toLowerCase()) ||
        p.keywords?.some(k => k.includes(query))
      );
    } catch {
      return [];
    }
  }
}
```

### CLI Commands

```typescript
// src/cli/commands/marketplace.ts
const marketplaceCommands = {
  '/marketplace search <query>': 'Search for plugins',
  '/marketplace install <plugin>': 'Install a plugin',
  '/marketplace update [plugin]': 'Update plugin(s)',
  '/marketplace list': 'List installed plugins',
  '/marketplace info <plugin>': 'Show plugin details',
  '/marketplace add <url>': 'Add marketplace source',
  '/marketplace remove <id>': 'Remove marketplace',
  '/marketplace browse': 'Browse popular plugins'
};
```

### File Changes

| File | Action | Description |
|------|--------|-------------|
| `src/marketplace/types.ts` | Create | Type definitions |
| `src/marketplace/manager.ts` | Create | Marketplace management |
| `src/marketplace/cache.ts` | Create | Plugin caching |
| `src/marketplace/sources/github.ts` | Create | GitHub source |
| `src/marketplace/sources/npm.ts` | Create | npm source |
| `src/marketplace/index.ts` | Create | Module exports |
| `src/cli/commands/marketplace.ts` | Create | CLI commands |

## User Experience

### Search Plugins
```
User: /marketplace search jira

Searching marketplaces...

┌─ Plugin Search Results ───────────────────────────────────┐
│                                                          │
│ jira-tools@acm-workflows                    ★ 4.8  ↓ 1.2K│
│ Comprehensive Jira management for ACM workflows          │
│ v1.2.0 • Updated 2 days ago                              │
│                                                          │
│ jira-sync@community                         ★ 4.2  ↓ 856 │
│ Sync Jira issues with local markdown files               │
│ v0.9.0 • Updated 1 week ago                              │
│                                                          │
│ jira-reports@official                       ★ 4.5  ↓ 2.1K│
│ Generate beautiful Jira reports                          │
│ v2.1.0 • Updated 3 days ago                              │
│                                                          │
└───────────────────────────────────────────────────────────┘

Install with: /marketplace install jira-tools@acm-workflows
```

### Install Plugin
```
User: /marketplace install jira-tools@acm-workflows

Installing jira-tools from acm-workflows...

Fetching plugin metadata...
Downloading v1.2.0...
Extracting to ~/.mycode/plugins/cache/...

✓ Installed: jira-tools@1.2.0

Components:
  Commands: 3 (/jira:my-issues, /jira:sprint-issues, /jira:add-labels)
  Skills: 2 (jira-analyzer, test-plan-generator)
  Agents: 1 (jira-administrator)

Enable with: /plugin enable jira-tools@acm-workflows
```

### Browse Popular
```
User: /marketplace browse

Popular Plugins:
┌────────────────────────────────────────────────────────────┐
│ 🔥 Trending This Week                                     │
├────────────────────────────────────────────────────────────┤
│ 1. code-reviewer@official       ★ 4.9  ↓ 5.2K             │
│ 2. git-workflows@community      ★ 4.7  ↓ 3.8K             │
│ 3. test-generator@official      ★ 4.6  ↓ 2.9K             │
│ 4. jira-tools@acm-workflows     ★ 4.8  ↓ 1.2K             │
│ 5. docker-helper@community      ★ 4.5  ↓ 1.1K             │
└────────────────────────────────────────────────────────────┘

Categories: [dev-tools] [testing] [productivity] [devops]
```

## Security Considerations

1. **Source Verification**: Verify marketplace sources
2. **Checksum Validation**: Verify downloaded content
3. **Permission Review**: Show required permissions
4. **Sandboxing**: Option to sandbox plugin execution
5. **Reporting**: Allow reporting malicious plugins

## Testing Strategy

1. **Unit Tests**: Search, install, update logic
2. **Integration Tests**: Full marketplace workflows
3. **E2E Tests**: Real marketplace interaction

## Migration Path

1. **Phase 1**: Basic search and install
2. **Phase 2**: Multiple marketplace support
3. **Phase 3**: Update mechanism
4. **Phase 4**: Ratings and reviews
5. **Phase 5**: Plugin publishing

## References

- [npm Registry](https://www.npmjs.com/)
- [VS Code Marketplace](https://marketplace.visualstudio.com/)
- [Plugin System Proposal (0022)](./0022-plugin-system.md)
