# GenCode Extension Architecture

From first principles: GenCode has three extension primitives — **Skill**, **Agent**, **Command**. Each primitive has multiple sources. Plugin is one source among many.

## Overview

```
                             ┌─────────────────────┐
                             │    System Prompt     │
                             │                      │
                             │  <available-skills>  │  ← active skills only
                             │  <available-agents>  │  ← enabled agents only
                             │  (commands: hidden)  │  ← user-initiated only
                             └──────────┬───────────┘
                                        │
               ┌────────────────────────┼────────────────────────┐
               │                        │                        │
        ┌──────┴──────┐          ┌──────┴──────┐          ┌──────┴──────┐
        │   Skill     │          │   Agent     │          │  Command    │
        │  Registry   │          │  Registry   │          │  Registry   │
        └──────┬──────┘          └──────┬──────┘          └──────┬──────┘
               │                        │                        │
     ┌─────────┼─────────┐    ┌────────┼────────┐     ┌─────────┼────────┐
     │         │         │    │        │        │     │         │        │
  Dirs      Plugins   Claude  Built  Dirs   Plugins  Built   Dirs    Plugins
                       Code   -ins            Claude  -ins            Claude
                                              Code                   Code
```

Each primitive loads from **independent sources** with a priority chain. Plugin is just one source that feeds into all three registries.

---

## Skill

### Definition

A directory containing `SKILL.md` with YAML frontmatter:

```markdown
---
name: my-skill
namespace: optional
description: One-line description
allowed-tools: [Read, Bash]
argument-hint: "[args]"
---

Instructions loaded lazily on invocation.
```

### Loading Flow

```
skill.Initialize(cwd)
│
│  Scan 6 sources (low → high priority):
│
│  ┌─ ~/.claude/skills/          (Scope 0: ClaudeUser)
│  ├─ ~/.gen/plugins/*/skills/   (Scope 1: UserPlugin)    ← from plugin
│  ├─ ~/.gen/skills/             (Scope 2: User)
│  ├─ .claude/skills/            (Scope 3: ClaudeProject)
│  ├─ .gen/plugins/*/skills/     (Scope 4: ProjectPlugin) ← from plugin
│  └─ .gen/skills/               (Scope 5: Project)       ← highest
│
├─ For each source directory:
│  └─ Walk → find SKILL.md (case-insensitive)
│     └─ Parse frontmatter only (body is lazy)
│        └─ Create Skill{name, scope, namespace, ...}
│
├─ Dedup by FullName: higher scope wins
│
├─ Apply persisted state:
│  ├─ ~/.gen/skills.json   (user-level state)
│  └─ .gen/skills.json     (project-level state, overrides user)
│
└─ Register into skill.Registry (singleton)
```

### Three States

```
  ┌──────────────┐         ┌──────────────┐         ┌──────────────┐
  │   Disabled   │ ──────→ │   Enabled    │ ──────→ │    Active    │
  │              │ ←────── │              │ ←────── │              │
  └──────────────┘         └──────────────┘         └──────────────┘
   LLM: invisible           LLM: invisible           LLM: sees name
   User: hidden             User: /slash visible      + description
                                                      in system prompt
```

### Invocation

```
                    ┌─────────────────────────┐
                    │ Trigger                  │
                    │  Enabled: user types     │
                    │    /skill-name [args]    │
                    │  Active: LLM calls      │
                    │    Skill tool directly   │
                    └────────────┬────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │ Skill Tool Execute      │
                    │                         │
                    │ 1. Find skill by name   │
                    │ 2. Check IsEnabled      │
                    │ 3. Permission approval  │
                    │ 4. GetInstructions()    │
                    │    (reads SKILL.md body │
                    │     from disk — lazy)   │
                    └────────────┬────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │ Inject into context     │
                    │                         │
                    │ <skill-invocation>      │
                    │   name, args,           │
                    │   scripts/ listing,     │
                    │   references/ listing,  │
                    │   full instructions     │
                    │ </skill-invocation>     │
                    └─────────────────────────┘
```

---

## Agent

### Definition

A `.md` file with YAML frontmatter:

```markdown
---
name: my-agent
description: What this agent does
when-to-use: |
  Guidance for LLM on when to spawn this agent
model: inherit
permission-mode: default
tools: [Read, Write, Bash]
disallowed-tools: [Agent]
max-turns: 100
---

System prompt content (markdown body, lazy-loaded).
```

### Loading Flow

```
subagent.Initialize(cwd)
│
├─ 1. Register built-in agents:
│     ┌────────────────────────────────────────────┐
│     │  explore         read-only codebase search │
│     │  plan            architecture planning     │
│     │  general-purpose all tools (default)       │
│     │  code-simplifier auto-accept edits         │
│     │  code-reviewer   read-only review          │
│     └────────────────────────────────────────────┘
│
├─ 2. Load custom agents (later overrides earlier):
│
│     ┌─ ~/.claude/agents/*.md        (ClaudeUser)
│     ├─ ~/.gen/agents/*.md           (User)
│     ├─ Plugin agent paths           (Plugin)         ← from plugin
│     ├─ .claude/agents/*.md          (ClaudeProject)
│     └─ .gen/agents/*.md             (Project)        ← highest
│
│     For each .md file:
│     └─ Parse YAML frontmatter
│        └─ Create AgentConfig{name, tools, model, ...}
│           └─ Body stored as SourceFile (lazy)
│              └─ registry.Register(config)
│                 (can override built-in with same name)
│
└─ 3. Apply state:
      ├─ ~/.gen/agents/disabled.json  (user)
      └─ .gen/agents/disabled.json    (project)
      (agents enabled by default, explicitly disabled)
```

### Invocation

```
  ┌──────────────────────────────────────────────────┐
  │ LLM calls Agent tool                              │
  │   Agent(subagent_type="name", prompt="...", ...)  │
  └──────────────────────┬───────────────────────────┘
                         │
                         ▼
  ┌──────────────────────────────────────────────────┐
  │ Resolve AgentConfig                               │
  │   subagent_type → registry lookup                 │
  │   (default: "general-purpose")                    │
  └──────────────────────┬───────────────────────────┘
                         │
                         ▼
  ┌──────────────────────────────────────────────────┐
  │ Build Subagent                                    │
  │                                                   │
  │  System prompt:                                   │
  │    parent's user/project instructions             │
  │    + agent body from .md (lazy-loaded)            │
  │                                                   │
  │  Tool set:                                        │
  │    Tools whitelist (null = all)                    │
  │    - DisallowedTools blacklist                     │
  │    + WithPermission(mode) wrapper                  │
  │                                                   │
  │  Model:                                           │
  │    agent config model (or inherit from parent)    │
  └──────────────────────┬───────────────────────────┘
                         │
               ┌─────────┴──────────┐
               │                    │
               ▼                    ▼
  ┌────────────────────┐  ┌────────────────────┐
  │ Foreground         │  │ Background         │
  │                    │  │                    │
  │ subagent.Run()     │  │ RunBackground()    │
  │ ThinkAct() sync    │  │ goroutine          │
  │ return Result      │  │ return taskId      │
  │ to parent context  │  │ notify on complete │
  └────────────────────┘  └────────────────────┘
```

---

## Command

### Definition

A `.md` file with YAML frontmatter:

```markdown
---
name: my-command
namespace: optional
description: What this command does
---

Command instructions (body loaded on invocation).
```

### Loading Flow

```
command.Initialize(cwd)
│
├─ 1. Built-in commands (hardcoded):
│     ┌──────────────────────────────────────────┐
│     │  /model  /clear  /fork  /resume  /help   │
│     │  /glob  /tools  /skills  /agents         │
│     │  /compact  /init  /memory  /mcp          │
│     │  /plugin  /reload-plugins  /think        │
│     │  /loop  /search  /tokenlimit             │
│     └──────────────────────────────────────────┘
│
├─ 2. Dynamic commands (from providers):
│     └─ Skill commands: each enabled skill → /skill-name
│
├─ 3. Custom commands from disk (later overrides earlier):
│
│     ┌─ ~/.gen/commands/             (User)
│     ├─ ~/.gen/plugins/*/commands/   (UserPlugin)     ← from plugin
│     ├─ .gen/plugins/*/commands/     (ProjectPlugin)  ← from plugin
│     └─ .gen/commands/               (Project)        ← highest
│
│     For each *.md file:
│     └─ Parse frontmatter (or use filename as name)
│        └─ Create CustomCommand{name, namespace, path}
│           └─ Body stored as path (lazy)
│
└─ Register into command.Registry (singleton)
      No state persistence — all commands always available
```

### Invocation

```
  ┌────────────────────────────────┐
  │ User types /command-name args  │
  │ (commands are never LLM-initiated) │
  └──────────────┬─────────────────┘
                 │
                 ▼
  ┌────────────────────────────────┐
  │ TUI ParseCommand(input)       │
  │                                │
  │ ├─ built-in? → TUI handler    │
  │ ├─ dynamic (skill)?           │
  │ │  → Skill tool invocation    │
  │ └─ custom?                    │
  │    → GetInstructions()        │
  │    → execute body             │
  └────────────────────────────────┘
```

---

## Plugin: One Source Among Many

A plugin is a **container** that contributes to multiple registries simultaneously:

```
                          plugin.Initialize(cwd)
                                  │
              Scan plugin directories:
              ├─ ~/.gen/plugins/        (User)
              ├─ .gen/plugins/          (Project)
              ├─ .gen/plugins-local/    (Local)
              ├─ ~/.claude/plugins/     (Claude compat)
              └─ .claude/plugins/       (Claude compat)
                                  │
                                  ▼
                     ┌──────────────────────┐
                     │  Load plugin.json    │
                     │  (.gen-plugin/ or    │
                     │   .claude-plugin/)   │
                     └──────────┬───────────┘
                                │
                     ResolveComponents()
                                │
          ┌──────────┬──────────┼──────────┬──────────┐
          │          │          │          │          │
          ▼          ▼          ▼          ▼          ▼
       skills/    agents/   commands/   mcpServers   hooks
          │          │          │          │          │
          ▼          ▼          ▼          ▼          ▼
       Skill      Agent     Command     MCP        Hook
       Registry   Registry  Registry   Registry   Engine
```

Plugin name becomes the **namespace** prefix: `git:commit`, `jira:create`.

### Claude Code Plugin Compatibility

```
                  GenCode plugin            Claude Code plugin
                  ──────────────            ──────────────────
  Manifest dir    .gen-plugin/              .claude-plugin/
  Manifest file   plugin.json              plugin.json (same format)
  Skill dir       skills/                  skills/
  Agent dir       agents/                  agents/
  Command dir     commands/                commands/
  MCP config      mcpServers (in json)     mcpServers (in json)
  Env vars        GEN_PLUGIN_ROOT          CLAUDE_PLUGIN_ROOT
                  (both set at runtime for child processes)

  Loading:   .gen-plugin/plugin.json tried first
             .claude-plugin/plugin.json as fallback
```

---

## MCP: Tools as a Separate Axis

MCP servers provide **tools** (not skills/agents/commands). They sit alongside built-in tools in the Tool Registry:

```
  Tool Registry
  ├─ Built-in: Read, Write, Edit, Bash, Glob, Grep,
  │            Agent, Skill, TaskCreate, ...
  │
  └─ MCP tools: mcp__<server>__<tool>
                 │
     Sources:    ├─ ~/.gen/mcp.json          (user config)
                 ├─ .gen/mcp.json            (project config)
                 ├─ .gen/mcp.local.json      (local, gitignored)
                 └─ plugin mcpServers         (from plugin manifest)
                         │
                         ▼
                    Connect via transport
                    (stdio / http / sse)
                         │
                         ▼
                    Discover tool schemas
                         │
                         ▼
                    Register as mcp__server__tool
                    in Tool Registry
```

---

## Initialization Order

```
app.initInfrastructure()

  ┌─────────────────────────────────────────────────────────────────┐
  │ Phase 1: Foundation                                             │
  │   setting.Initialize()  →  llm.Initialize()                    │
  └──────────────────────────────┬──────────────────────────────────┘
                                 │
  ┌──────────────────────────────▼──────────────────────────────────┐
  │ Phase 2: Plugin (must be first — others pull from it)          │
  │   plugin.Initialize(cwd)                                       │
  │     → scan scopes → load manifests → resolve component paths   │
  └──────────────────────────────┬──────────────────────────────────┘
                                 │
     ┌───────────────┬───────────┼───────────┬───────────────┐
     ▼               ▼           ▼           ▼               ▼
  ┌────────┐   ┌──────────┐  ┌────────┐  ┌──────┐    ┌──────────┐
  │ Skill  │   │ Subagent │  │Command │  │ MCP  │    │   Hook   │
  │ Init   │   │  Init    │  │ Init   │  │ Init │    │   Init   │
  │        │   │          │  │        │  │      │    │          │
  │ plugin │   │ plugin   │  │ plugin │  │plugin│    │  plugin  │
  │ paths  │   │ paths    │  │ paths  │  │configs    │  hooks   │
  │  ↓     │   │  ↓       │  │  ↓     │  │  ↓   │    │   ↓     │
  │ +dirs  │   │ +built-in│  │ +built │  │+cfgs │    │  merge  │
  │ +claude│   │ +dirs    │  │  -in   │  │      │    │         │
  └────────┘   │ +claude  │  │ +dirs  │  └──────┘    └──────────┘
               └──────────┘  │ +dyn.  │
                             └────────┘
```

---

## Comparison

| | Skill | Agent | Command |
|--|-------|-------|---------|
| **File format** | `SKILL.md` in directory | `*.md` file | `*.md` file |
| **Sources** | 6 scoped dirs + plugins | built-ins + 4 dirs + plugins | built-ins + dynamic + 4 dirs + plugins |
| **Priority rule** | Higher scope wins | Later source overrides | Later source overrides |
| **State model** | 3 states: disable/enable/active | 2 states: enabled/disabled (default enabled) | No state (always available) |
| **State storage** | `skills.json` (user + project) | `disabled.json` (user + project) | None |
| **In system prompt** | Active only (name + description) | Enabled (name + description + when-to-use) | Never |
| **LLM can invoke** | Yes (Skill tool) | Yes (Agent tool) | No (user-initiated only) |
| **Body loading** | Lazy (on invocation) | Lazy (on first spawn) | Lazy (on invocation) |
| **Namespacing** | `namespace:name` | `namespace:name` | `namespace:name` |
| **Plugin source** | `plugin.json` → `skills/` | `plugin.json` → `agents/` | `plugin.json` → `commands/` |
