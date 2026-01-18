# GenCode Multi-Level Configuration System

## Overview

GenCode implements a multi-level configuration system compatible with Claude Code, while also supporting OpenCode-style flexibility through the `GEN_CONFIG` environment variable.

## Table of Contents

1. [Configuration Hierarchy](#configuration-hierarchy)
2. [Detailed Flow Diagrams](#detailed-flow-diagrams)
3. [Merge Strategy](#merge-strategy)
4. [Resource Discovery System](#resource-discovery-system)
5. [GenCode vs OpenCode vs Claude Code](#gencode-vs-opencode-vs-claude-code-comparison)
6. [Usage Examples](#usage-examples)
7. [API Reference](#api-reference)

---

## Configuration Hierarchy

```
Priority (High → Low):
┌─────────────────────────────────────────────────────────────────────┐
│  Level 6: Managed (System) - HIGHEST PRIORITY                      │
│  Location: /Library/Application Support/GenCode/managed-settings.json
│  Scope: All users on machine (deployed by IT)                      │
│  Cannot be overridden                                               │
├─────────────────────────────────────────────────────────────────────┤
│  Level 5: CLI Arguments                                             │
│  Scope: Current session only                                        │
├─────────────────────────────────────────────────────────────────────┤
│  Level 4: Local (Personal)                                          │
│  Location: .gen/*.local.* + .claude/*.local.*                   │
│  Scope: Current user, current project only (gitignored)             │
├─────────────────────────────────────────────────────────────────────┤
│  Level 3: Project (Shared)                                          │
│  Location: .gen/ + .claude/ (MERGED)                            │
│  Scope: All collaborators (committed to git)                        │
├─────────────────────────────────────────────────────────────────────┤
│  Level 2: Extra Dirs (GEN_CONFIG)                          │
│  Location: Colon-separated paths from environment variable          │
│  Scope: Team/organization shared configs                            │
├─────────────────────────────────────────────────────────────────────┤
│  Level 1: User (Global) - LOWEST PRIORITY                           │
│  Location: ~/.gen/ + ~/.claude/ (MERGED)                        │
│  Scope: Current user, all projects                                  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Detailed Flow Diagrams

### Settings Loading Pipeline

```
                              ┌─────────────────┐
                              │     START       │
                              └────────┬────────┘
                                       │
                    ┌──────────────────▼──────────────────┐
                    │   Initialize Empty Settings         │
                    │   settings = {}                     │
                    └──────────────────┬──────────────────┘
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 1: Load USER Settings (最低优先级，作为基础)                            ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   同层级内合并 (claude 先加载，gencode 后加载覆盖):                            ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  claude  = load("~/.claude/settings.json")    // 低优先级           │   ║
    ║   │  gencode = load("~/.gen/settings.json")   // 高优先级           │   ║
    ║   │  user    = deepMerge(claude, gencode)         // gencode 覆盖       │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
                    ┌──────────────────▼──────────────────┐
                    │  settings = deepMerge({}, user)     │
                    └──────────────────┬──────────────────┘
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 2: Load EXTRA Config Dirs (可选，GEN_CONFIG)                 ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   环境变量: GEN_CONFIG="/team/config:~/shared-rules"               ║
    ║                                                                             ║
    ║   For each dir in GEN_CONFIG.split(':'):                          ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  extra = load("{dir}/settings.json")                                │   ║
    ║   │  settings = deepMerge(settings, extra)                              │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 3: Load PROJECT Settings (团队共享)                                   ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   同层级内合并:                                                              ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  claude  = load(".claude/settings.json")      // 低优先级           │   ║
    ║   │  gencode = load(".gen/settings.json")     // 高优先级           │   ║
    ║   │  project = deepMerge(claude, gencode)         // gencode 覆盖       │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
                    ┌──────────────────▼──────────────────┐
                    │  settings = deepMerge(settings,     │
                    │                       project)      │
                    └──────────────────┬──────────────────┘
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 4: Load LOCAL Settings (个人项目级，gitignored)                        ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   同层级内合并:                                                              ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  claude  = load(".claude/settings.local.json")  // 低优先级         │   ║
    ║   │  gencode = load(".gen/settings.local.json") // 高优先级         │   ║
    ║   │  local   = deepMerge(claude, gencode)           // gencode 覆盖     │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 5: Apply CLI Arguments (会话级临时覆盖)                                ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   命令行参数覆盖文件配置:                                                      ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  --provider anthropic  →  settings.provider = "anthropic"           │   ║
    ║   │  --model claude-sonnet →  settings.model = "claude-sonnet"          │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 6: Apply MANAGED Settings (最高优先级，强制执行)                        ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   系统级位置（按平台）:                                                        ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  macOS:   /Library/Application Support/GenCode/managed-settings.json│   ║
    ║   │           + /Library/Application Support/ClaudeCode/...             │   ║
    ║   │  Linux:   /etc/gencode/managed-settings.json                        │   ║
    ║   │           + /etc/claude-code/managed-settings.json                  │   ║
    ║   │  Windows: C:\Program Files\GenCode\managed-settings.json            │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ║   特殊处理:                                                                   ║
    ║   • managed.permissions.deny → 添加到 deny 列表且不可移除                     ║
    ║   • managed.strictKnownMarketplaces → 强制插件白名单                         ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
                              ┌────────▼────────┐
                              │ Return Final    │
                              │ Settings        │
                              └─────────────────┘
```

### Memory Loading Pipeline

```
                              ┌─────────────────┐
                              │     START       │
                              │  memories = []  │
                              └────────┬────────┘
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 1: Load ENTERPRISE Memory (组织级策略，enforced)                       ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   系统级位置 (都加载，先 claude 后 gencode):                                   ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  macOS:                                                             │   ║
    ║   │    if exists: memories.push({                                       │   ║
    ║   │      content: load("/Library/.../ClaudeCode/CLAUDE.md"),            │   ║
    ║   │      namespace: "claude", enforced: true                            │   ║
    ║   │    })                                                               │   ║
    ║   │    if exists: memories.push({                                       │   ║
    ║   │      content: load("/Library/.../GenCode/GEN.md"),                │   ║
    ║   │      namespace: "gencode", enforced: true                           │   ║
    ║   │    })                                                               │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 2: Load USER Memory (全局个人)                                         ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   都加载 (先 claude 后 gencode):                                              ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  if exists: memories.push(load("~/.claude/CLAUDE.md"))              │   ║
    ║   │  if exists: memories.push(load("~/.gen/GEN.md"))              │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ║   同样加载 rules:                                                            ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  ~/.claude/rules/*.md  → memories.push(each)                        │   ║
    ║   │  ~/.gen/rules/*.md → memories.push(each)                        │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 3: Load EXTRA Config Dirs Memory (可选)                               ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   For each dir in GEN_CONFIG:                                      ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  if exists: memories.push(load("{dir}/CLAUDE.md"))                  │   ║
    ║   │  if exists: memories.push(load("{dir}/GEN.md"))                   │   ║
    ║   │  for each: memories.push(load("{dir}/rules/*.md"))                  │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 4: Load PROJECT Memory (都加载)                                        ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   从 project root 加载 (先 claude 后 gencode):                               ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  # Claude files (pick first found)                                  │   ║
    ║   │  if exists: memories.push(load("./CLAUDE.md"))                      │   ║
    ║   │  else if exists: memories.push(load("./.claude/CLAUDE.md"))         │   ║
    ║   │                                                                     │   ║
    ║   │  # GenCode files (pick first found)                                 │   ║
    ║   │  if exists: memories.push(load("./GEN.md"))                       │   ║
    ║   │  else if exists: memories.push(load("./.gen/GEN.md"))         │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 5: Load PROJECT Rules (模块化规则文件)                                  ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   加载规则目录 (都加载):                                                      ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  .claude/rules/*.md  → memories.push(each, namespace: "claude")     │   ║
    ║   │  .gen/rules/*.md → memories.push(each, namespace: "gencode")    │   ║
    ║   │                                                                     │   ║
    ║   │  支持 paths frontmatter 条件加载:                                    │   ║
    ║   │  ---                                                                │   ║
    ║   │  paths:                                                             │   ║
    ║   │    - "src/api/**/*.ts"                                              │   ║
    ║   │  ---                                                                │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 6: Load LOCAL Memory (个人项目级，gitignored)                          ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   都加载 (先 claude 后 gencode):                                              ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  if exists: memories.push(load("./CLAUDE.local.md"))                │   ║
    ║   │  if exists: memories.push(load("./.claude/CLAUDE.local.md"))        │   ║
    ║   │  if exists: memories.push(load("./GEN.local.md"))                 │   ║
    ║   │  if exists: memories.push(load("./.gen/GEN.local.md"))        │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 7: Process @imports (递归解析引用)                                     ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║                                                                             ║
    ║   解析 @path/to/file 语法:                                                   ║
    ║   ┌─────────────────────────────────────────────────────────────────────┐   ║
    ║   │  See @README for project overview                                   │   ║
    ║   │  Build commands in @package.json                                    │   ║
    ║   │                                                                     │   ║
    ║   │  规则:                                                               │   ║
    ║   │  • 支持相对路径和绝对路径                                             │   ║
    ║   │  • 最大递归深度: 5 层                                                 │   ║
    ║   │  • 代码块内的 @引用 被忽略                                            │   ║
    ║   └─────────────────────────────────────────────────────────────────────┘   ║
    ║                                                                             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
                              ┌────────▼────────┐
                              │ Concatenate all │
                              │ → LLM context   │
                              └─────────────────┘
```

### Permission Resolution Flow

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                     Permission Check: Bash(npm install)                          │
└─────────────────────────────────────────────────────────────────────────────────┘

    ┌─────────────────┐
    │ Check Managed   │
    │ deny list       │
    └────────┬────────┘
             │
    ┌────────▼────────┐     ┌─────────────────┐
    │ In managed.deny?│────▶│ DENY (enforced) │
    └────────┬────────┘ Yes └─────────────────┘
             │ No
    ┌────────▼────────┐
    │ Check merged    │
    │ allow list      │
    └────────┬────────┘
             │
    ┌────────▼────────┐     ┌─────────────────┐
    │ Matches allow?  │────▶│ ALLOW           │
    └────────┬────────┘ Yes └─────────────────┘
             │ No
    ┌────────▼────────┐
    │ Check merged    │
    │ deny list       │
    └────────┬────────┘
             │
    ┌────────▼────────┐     ┌─────────────────┐
    │ Matches deny?   │────▶│ DENY            │
    └────────┬────────┘ Yes └─────────────────┘
             │ No
    ┌────────▼────────┐
    │ Check merged    │
    │ ask list        │
    └────────┬────────┘
             │
    ┌────────▼────────┐     ┌─────────────────┐
    │ Matches ask?    │────▶│ PROMPT USER     │
    └────────┬────────┘ Yes └─────────────────┘
             │ No
    ┌────────▼────────┐
    │ Default:        │
    │ PROMPT USER     │
    └─────────────────┘
```

---

## Merge Strategy

```
src/config/
├── index.ts           # Module exports
├── types.ts           # Types and constants (updated)
├── levels.ts          # Level detection and path resolution (new)
├── loader.ts          # Load config from directories (new)
├── merger.ts          # Merge config from multiple sources (new)
├── manager.ts         # ConfigManager class (refactored)
└── providers-config.ts # Provider-specific config (existing)

src/memory/
├── types.ts           # Memory types (updated for merge semantics)
└── memory-manager.ts  # Memory loading (updated for merge semantics)
```

---

## Resource Discovery System

GenCode implements a unified resource discovery system for Commands, Skills, and Subagents. Unlike Settings and Memory which use **deep merge** strategies, these resources use a **name-based merge** strategy where resources with the same name from higher priority sources override those from lower priority sources.

### Architecture Overview

```
src/discovery/
├── types.ts           # Core types: DiscoverableResource, ResourceParser, FilePattern
├── path-resolver.ts   # Unified path resolution for all resource levels
├── file-scanner.ts    # File pattern matching (flat, nested, multiple, single)
├── base-loader.ts     # Generic resource discovery with merge logic
└── index.ts           # Public API exports
```

### Resource Types

All resources implement the `DiscoverableResource` interface:

```typescript
interface DiscoverableResource {
  name: string;
  source: ResourceSource; // { path, level, namespace }
}
```

#### Commands
- **Location**: `commands/*.md`
- **Pattern**: Flat (all .md files in commands directory)
- **Format**: Markdown with YAML frontmatter
- **Loading**: User and project levels
- **Example**: `~/.gen/commands/commit.md`, `.gen/commands/test.md`

#### Skills
- **Location**: `skills/*/SKILL.md`
- **Pattern**: Nested (SKILL.md in each subdirectory)
- **Format**: Markdown with YAML frontmatter
- **Loading**: User and project levels
- **Example**: `~/.gen/skills/database/SKILL.md`

#### Subagents (Custom Agents)
- **Location**: `agents/*.{json,md}`
- **Pattern**: Multiple extensions (JSON or Markdown)
- **Format**: JSON config or Markdown with frontmatter
- **Loading**: User and project levels
- **Example**: `~/.gen/agents/ml-engineer.md`, `.gen/agents/code-reviewer.json`

### File Patterns

The unified file scanner supports four pattern types:

| Pattern | Description | Example |
|---------|-------------|---------|
| **flat** | Files with extension in directory | `commands/*.md` |
| **nested** | Specific filename in subdirectories | `skills/*/SKILL.md` |
| **multiple** | Multiple extensions | `agents/*.{json,md}` |
| **single** | Single file | `.mcp.json` |

### Priority Rules

Resources follow the same level and namespace priority as Settings:

**Level Priority** (ascending):
1. `user` - `~/.claude/` and `~/.gen/`
2. `project` - `.claude/` and `.gen/`
3. `local` - `.claude.local/` and `.gen.local/` (gitignored)
4. `managed` - System-wide (future)

**Namespace Priority** (within same level):
- `claude` < `gen` (gen has higher priority)

**Name-based Merge**:
- Resources with the same name: higher priority source wins
- Different names: all resources are loaded
- Example: If both `~/.gen/commands/test.md` and `.gen/commands/test.md` exist, project level wins

### Discovery Flow

```
                              ┌─────────────────┐
                              │  START          │
                              │  resources = {} │
                              └────────┬────────┘
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 1: Scan USER Level (claude then gen)                                  ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║   For each dir in [~/.claude/commands, ~/.gen/commands]:               ║
    ║     - Scan files matching pattern                                           ║
    ║     - Parse each file with ResourceParser                                   ║
    ║     - Add to resources map (gen overwrites claude if same name)             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 2: Scan PROJECT Level (claude then gen)                               ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║   For each dir in [.claude/commands, .gen/commands]:                   ║
    ║     - Scan files matching pattern                                           ║
    ║     - Parse each file with ResourceParser                                   ║
    ║     - Add to resources map (overwrites user level if same name)             ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  STEP 3: Scan LOCAL Level (optional, claude then gen)                       ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║   For each dir in [.claude.local/commands, .gen.local/commands]:       ║
    ║     - Scan files matching pattern                                           ║
    ║     - Parse each file with ResourceParser                                   ║
    ║     - Add to resources map (overwrites project level if same name)          ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
                              ┌────────▼────────┐
                              │ Return resources│
                              │ Map<name, T>    │
                              └─────────────────┘
```

### Settings vs Resources Comparison

| Aspect | Settings/Memory | Commands/Skills/Subagents |
|--------|----------------|--------------------------|
| **Strategy** | Deep merge | Name-based merge |
| **Result** | Single merged object | Map of resources |
| **Conflict** | Later value overwrites | Higher priority source wins |
| **Arrays** | Concatenate & dedupe | N/A (separate resources) |
| **Use Case** | Configuration values | Executable resources |

### Extensibility

Adding new resource types requires three steps:

1. **Define Type** (extends `DiscoverableResource`):
```typescript
export interface PluginDefinition extends DiscoverableResource {
  name: string;
  version: string;
  description: string;
  source: ResourceSource;
}
```

2. **Implement Parser** (implements `ResourceParser`):
```typescript
export class PluginParser implements ResourceParser<PluginDefinition> {
  async parse(filePath, level, namespace): Promise<PluginDefinition | null> {
    // Parse plugin.json or PLUGIN.md
  }
  isValidName(name: string): boolean {
    return /^[a-zA-Z0-9_-]+$/.test(name);
  }
}
```

3. **Use Unified Loader**:
```typescript
const plugins = await discoverResources(projectRoot, {
  resourceType: 'Plugin',
  subdirectory: 'plugins',
  filePattern: { type: 'nested', filename: 'plugin.json' },
  parser: new PluginParser(),
  levels: ['user', 'project'],
});
```

### Benefits

- **Code Reuse**: ~481 lines of duplicate code eliminated
- **Consistency**: Same directory structure and priority rules for all resources
- **Type Safety**: TypeScript generics ensure type-safe resource handling
- **Testability**: Easy to test with isolated project-only mode
- **Extensibility**: New resource types can be added in minutes

### Future Resource Types

When implementing new proposals (e.g., plugins, MCP servers, workflows), prefer using or extending the unified resource discovery system rather than creating separate loading mechanisms. This ensures:

- Consistent user experience (same directories, same priority rules)
- Automatic support for all levels (user, project, local, managed)
- Automatic support for both namespaces (.gen and .claude)
- Less code to write, test, and maintain

## GenCode vs OpenCode vs Claude Code Comparison

### Directory Structure

| Aspect | GenCode | OpenCode | Claude Code |
|--------|---------|----------|-------------|
| **User Config Dir** | `~/.gen/` + `~/.claude/` (merge) | `~/.config/opencode/` (XDG) | `~/.claude/` |
| **Project Config Dir** | `.gen/` + `.claude/` (merge) | `.opencode/` | `.claude/` |
| **Config File Format** | JSON | JSON/JSONC/TOML | JSON |
| **Memory File** | `GEN.md` / `CLAUDE.md` | N/A (uses instructions) | `CLAUDE.md` |
| **Rules Dir** | `rules/*.md` | N/A | `rules/*.md` |

### Loading Semantics

| Aspect | GenCode | OpenCode | Claude Code |
|--------|---------|----------|-------------|
| **Same Level Merge** | Yes (claude + gencode) | No (single namespace) | No (single namespace) |
| **Cross Level Merge** | Deep merge | Deep merge | Deep merge |
| **Array Handling** | Concatenate & dedupe | Concatenate & dedupe | Concatenate |
| **Managed Level** | Yes (enforced deny) | Yes (well-known) | Yes (managed settings) |

### Configuration Hierarchy

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         Hierarchy Comparison                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  GenCode (6 levels):                 OpenCode (6 levels):    Claude Code (5):  │
│  ─────────────────                   ────────────────────    ───────────────   │
│  1. User                             1. Remote (well-known)  1. User           │
│     ~/.claude/ + ~/.gen/         2. Global (~/.config/)  2. Project        │
│  2. Extra (GEN_CONFIG)      3. OPENCODE_CONFIG      3. Local          │
│  3. Project                          4. Project              4. CLI            │
│     .claude/ + .gen/             5. .opencode/           5. Managed        │
│  4. Local                            6. OPENCODE_CONFIG_CONTENT                │
│     *.local.json                                                               │
│  5. CLI                                                                        │
│  6. Managed (enforced)                                                         │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Key Differences

#### 1. Dual Namespace Merge (GenCode Unique)

GenCode loads **both** `.gen/` and `.claude/` directories at each level and merges them:

```typescript
// At User level:
claude  = load("~/.claude/settings.json")     // lower priority
gencode = load("~/.gen/settings.json")    // higher priority
result  = deepMerge(claude, gencode)          // gencode wins on conflicts
```

OpenCode and Claude Code only use their own namespace.

#### 2. Extra Config Dirs (GenCode Feature)

GenCode supports `GEN_CONFIG` for team/organization config:

```bash
export GEN_CONFIG="/team/shared-config:~/my-custom-rules"
```

OpenCode has `OPENCODE_CONFIG` for single file and `OPENCODE_CONFIG_CONTENT` for inline JSON.

#### 3. Memory System (GenCode/Claude Code Feature)

GenCode implements a full memory system like Claude Code:
- `GEN.md` / `CLAUDE.md` files
- `rules/*.md` with path-scoped activation
- `@import` syntax for file references

OpenCode uses `instructions` in config files instead of separate memory files.

#### 4. Managed Settings Location

| Platform | GenCode | OpenCode | Claude Code |
|----------|---------|----------|-------------|
| macOS | `/Library/Application Support/GenCode/` | N/A (uses well-known) | `/Library/Application Support/ClaudeCode/` |
| Linux | `/etc/gencode/` | N/A | `/etc/claude-code/` |
| Windows | `C:\Program Files\GenCode\` | N/A | `C:\Program Files\ClaudeCode\` |

### Merge Strategy Comparison

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           Merge Strategy                                        │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  GenCode (Settings):                                                            │
│  • Scalars: Later replaces earlier                                              │
│  • Arrays: Concatenate and deduplicate                                          │
│  • Objects: Deep merge recursively                                              │
│  • Managed deny: Cannot be removed by any level                                 │
│                                                                                 │
│  GenCode (Memory):                                                              │
│  • All files concatenated in order                                              │
│  • Later content appears later in context (higher LLM priority)                 │
│  • Enterprise files marked as [ENFORCED]                                        │
│                                                                                 │
│  OpenCode:                                                                      │
│  • Uses remeda's mergeDeep                                                      │
│  • plugin and instructions arrays: Concatenate and dedupe                       │
│  • Other arrays: Replace                                                        │
│                                                                                 │
│  Claude Code:                                                                   │
│  • Similar to GenCode                                                           │
│  • Managed settings enforced                                                    │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Feature Matrix

| Feature | GenCode | OpenCode | Claude Code |
|---------|:-------:|:--------:|:-----------:|
| Multi-level config | ✅ | ✅ | ✅ |
| Dual namespace merge | ✅ | ❌ | ❌ |
| Claude Code compat | ✅ | ❌ | ✅ |
| Memory files | ✅ | ❌ | ✅ |
| Rules with paths | ✅ | ❌ | ✅ |
| @import syntax | ✅ | ✅ (file:) | ✅ |
| Extra config dirs | ✅ | ✅ | ❌ |
| Managed settings | ✅ | ✅ (well-known) | ✅ |
| XDG compliance | ❌ | ✅ | ❌ |
| JSONC support | ❌ | ✅ | ❌ |
| TOML support | ❌ | ✅ | ❌ |
| Agent definitions | ❌ | ✅ | ❌ |
| Command definitions | ❌ | ✅ | ❌ |
| Plugin system | ❌ | ✅ | ✅ |
| LSP integration | ❌ | ✅ | ✅ |

## Usage Examples

### Loading Configuration

```typescript
import { ConfigManager } from './config';

const config = new ConfigManager({ cwd: process.cwd() });
await config.load();

// Get merged settings
const settings = config.get();

// Get debug info
console.log(config.getDebugSummary());
// Output:
// Configuration Sources (in priority order):
//   user:claude - ~/.claude/settings.json
//   user:gencode - ~/.gen/settings.json
//   project:claude - .claude/settings.json
//   project:gencode - .gen/settings.json
```

### Loading Memory

```typescript
import { MemoryManager } from './memory';

const memory = new MemoryManager();
await memory.load({ cwd: process.cwd() });

// Get combined context for LLM
const context = memory.getLoaded()?.context;

// Get debug info
console.log(memory.getDebugSummary());
// Output:
// Memory Sources (in load order):
//   user:claude - ~/.claude/CLAUDE.md (1024 bytes)
//   user:gencode - ~/.gen/GEN.md (512 bytes)
//   project:claude - ./CLAUDE.md (2048 bytes)
//   project:gencode - ./GEN.md (1024 bytes)
```

### Using Extra Config Dirs

```bash
# Set up team config
export GEN_CONFIG="/team/shared-config"

# Create team settings
echo '{"provider": "anthropic"}' > /team/shared-config/settings.json
echo '# Team Guidelines' > /team/shared-config/GEN.md

# Run GenCode - it will merge team config
npx gencode
```

## Migration from Claude Code

GenCode is fully backward compatible with Claude Code:

1. Existing `.claude/` directories work automatically
2. `CLAUDE.md` files are loaded alongside `GEN.md`
3. No changes needed for existing Claude Code users

To migrate:
1. Optionally rename `.claude/` to `.gen/`
2. Optionally rename `CLAUDE.md` to `GEN.md`
3. Or keep both - GenCode will merge them

## API Reference

### ConfigManager

```typescript
class ConfigManager {
  constructor(options?: { cwd?: string });

  // Load and merge all config sources
  async load(): Promise<MergedConfig>;

  // Get merged settings
  get(): Settings;

  // Save to specific level
  async saveToLevel(updates: Partial<Settings>, level: 'user' | 'project' | 'local'): Promise<void>;

  // Permission helpers
  isAllowed(pattern: string): boolean;
  shouldAsk(pattern: string): boolean;
  getEffectivePermissions(): PermissionResult;

  // Debug
  getDebugSummary(): string;
  getSources(): ConfigSource[];
}
```

### MemoryManager

```typescript
class MemoryManager {
  constructor(config?: Partial<MemoryConfig>);

  // Load all memory files
  async load(options: MemoryLoadOptions): Promise<LoadedMemory>;

  // Get loaded memory
  getLoaded(): LoadedMemory | null;
  hasMemory(): boolean;

  // Debug
  getDebugSummary(): string;
  getLoadedFileList(): MemorySource[];
}
```

## Architecture Diagrams

### Settings Loading Flow

```
                              ┌─────────────────┐
                              │  Initialize     │
                              │  settings = {}  │
                              └────────┬────────┘
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  LEVEL 1: USER Settings                                                     ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║   claude  = load("~/.claude/settings.json")    // lower priority            ║
    ║   gencode = load("~/.gen/settings.json")   // higher priority           ║
    ║   user    = deepMerge(claude, gencode)                                      ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  LEVEL 2: EXTRA Config Dirs (GEN_CONFIG)                           ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  LEVEL 3: PROJECT Settings                                                  ║
    ╠═════════════════════════════════════════════════════════════════════════════╣
    ║   claude  = load(".claude/settings.json")      // lower priority            ║
    ║   gencode = load(".gen/settings.json")     // higher priority           ║
    ║   project = deepMerge(claude, gencode)                                      ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  LEVEL 4: LOCAL Settings (*.local.json)                                     ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  LEVEL 5: CLI Arguments                                                     ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  LEVEL 6: MANAGED Settings (enforced, cannot be overridden)                 ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
                              ┌────────▼────────┐
                              │ Return Final    │
                              │ Settings        │
                              └─────────────────┘
```

### Memory Loading Flow

```
                              ┌─────────────────┐
                              │  Initialize     │
                              │  memories = []  │
                              └────────┬────────┘
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  1. ENTERPRISE Memory (enforced)                                            ║
    ║     /Library/.../ClaudeCode/CLAUDE.md  →  push                              ║
    ║     /Library/.../GenCode/GEN.md      →  push                              ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  2. USER Memory                                                             ║
    ║     ~/.claude/CLAUDE.md  →  push                                            ║
    ║     ~/.gen/GEN.md  →  push                                            ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  3. EXTRA Memory (GEN_CONFIG)                                      ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  4. PROJECT Memory                                                          ║
    ║     ./CLAUDE.md or .claude/CLAUDE.md  →  push                               ║
    ║     ./GEN.md or .gen/GEN.md   →  push                               ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  5. RULES (.claude/rules/ + .gen/rules/)                                ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
    ╔══════════════════════════════════▼══════════════════════════════════════════╗
    ║  6. LOCAL Memory (*.local.md)                                               ║
    ║     ./CLAUDE.local.md  →  push                                              ║
    ║     ./GEN.local.md   →  push                                              ║
    ╚══════════════════════════════════╤══════════════════════════════════════════╝
                                       │
                              ┌────────▼────────┐
                              │ Concatenate all │
                              │ memories        │
                              └─────────────────┘
```
