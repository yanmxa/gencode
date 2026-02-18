# Skills & Agents Context Loading Mechanism

## Overview

GenCode's Skills and Agents both use a **Progressive Loading** strategy. This design ensures:

1. **At startup**: Only load lightweight metadata
2. **In LLM prompt**: Only inject name + description
3. **At execution**: Load the full SystemPrompt on-demand

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Application Startup                            │
├─────────────────────────────────────────────────────────────────────────┤
│  1. plugin.DefaultRegistry.Load()    → Load plugins                      │
│  2. agent.Init()                     → Load agent metadata               │
│     └── LoadCustomAgents()           → Parse .md files                   │
│         └── parseAgentFile()         → Extract YAML frontmatter only     │
│             ├── name, description, model, permission-mode ✓              │
│             └── SystemPrompt (body) ✗ NOT loaded                         │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                            Main Loop Context                             │
├─────────────────────────────────────────────────────────────────────────┤
│  buildExtraContext() builds per-turn Extra content:                      │
│                                                                          │
│  extra := []string{                                                      │
│      skill.GetAvailableSkillsPrompt(),   // <available-skills>...</>    │
│      agent.GetAgentPromptForLLM(),       // <available-agents>...</>    │
│  }                                                                       │
│                                                                          │
│  System Prompt Composition:                                              │
│  ┌────────────────────────────────────────────────────────────────────┐ │
│  │ base.txt          │ Base AI assistant identity                     │ │
│  │ tools.txt         │ Tool usage instructions                        │ │
│  │ provider.txt      │ Provider-specific prompts                      │ │
│  │ environment       │ cwd, platform, date, git status                │ │
│  │ memory (CLAUDE.md)│ User memory                                    │ │
│  │ Extra:            │                                                │ │
│  │   ├── <available-skills>                                           │ │
│  │   │   - skill-name: description                                    │ │
│  │   │   - ...                                                        │ │
│  │   └── <available-agents>         ← Only name + description         │ │
│  │       - agent-name: description                                    │ │
│  │       - code-simplifier:code-simplifier: Simplifies and refines... │ │
│  └────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                     LLM decides to call Task tool (spawn agent)
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                        Agent Execution (Isolated Loop)                   │
├─────────────────────────────────────────────────────────────────────────┤
│  TaskTool.execute() → Executor.Run()                                     │
│                                                                          │
│  1. Get AgentConfig                                                      │
│     config := agent.DefaultRegistry.Get(agentType)                       │
│                                                                          │
│  2. Build Agent System Prompt (buildSystemPrompt)                        │
│     ┌────────────────────────────────────────────────────────────────┐  │
│     │ ## Agent Type: code-simplifier:code-simplifier                 │  │
│     │ Simplifies and refines code...                                 │  │
│     │                                                                │  │
│     │ ## Your Task                                                   │  │
│     │ {req.Prompt - user-specified task}                             │  │
│     │                                                                │  │
│     │ ## Mode: Read-Only/Autonomous (based on permission-mode)       │  │
│     │                                                                │  │
│     │ ## Additional Instructions  ← Lazy loaded! First GetSystemPrompt()│
│     │ {full agent markdown body}                                     │  │
│     │ You are an expert code simplification specialist...           │  │
│     │ 1. **Preserve Functionality**...                               │  │
│     │ 2. **Apply Project Standards**...                              │  │
│     │ ...                                                            │  │
│     │                                                                │  │
│     │ ## Environment                                                 │  │
│     │ - Working directory: /path/to/project                         │  │
│     │ - Platform: darwin                                             │  │
│     │ - Date: 2026-02-18                                            │  │
│     │                                                                │  │
│     │ ## Guidelines                                                  │  │
│     │ - Focus on completing your assigned task efficiently          │  │
│     │ - Return a clear summary when your task is complete           │  │
│     └────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│  3. Create isolated core.Loop                                            │
│     loop := &core.Loop{                                                  │
│         System: &system.System{                                          │
│             Client: c,                                                   │
│             Cwd:    e.cwd,                                               │
│             Extra:  []string{agentPrompt},  // Agent-specific prompt     │
│         },                                                               │
│         Client:     c,                       // May use different model  │
│         Tool:       &tool.Set{Access: ...},  // Restricted tools        │
│         Permission: agentPermission(...),    // Independent permissions │
│     }                                                                    │
│                                                                          │
│  4. Add user message and run                                             │
│     loop.AddUser(req.Prompt, nil)                                        │
│     result, err := loop.Run(ctx, core.RunOptions{MaxTurns: maxTurns})    │
└─────────────────────────────────────────────────────────────────────────┘
```

## Key Code Paths

### 1. Startup Loading (Metadata Only)

```go
// internal/agent/loader.go
func parseAgentFile(content, filePath string) (*AgentConfig, error) {
    frontmatter, body := extractFrontmatter(content)

    var config AgentConfig
    yaml.Unmarshal([]byte(frontmatter), &config)  // Only parse YAML metadata

    // Don't load body! Save file path for lazy loading
    _ = body  // Body will be loaded lazily via GetSystemPrompt()
    config.SourceFile = filePath

    return &config, nil
}
```

### 2. Main Loop Agent Metadata Injection

```go
// internal/agent/registry.go
func (r *Registry) GetAgentPromptForLLM() string {
    var lines []string
    for name, config := range r.agents {
        if r.isDisabledInternal(name) {
            continue
        }
        // Only include name + description
        lines = append(lines, "- "+config.Name+": "+config.Description)
    }

    var sb strings.Builder
    sb.WriteString("<available-agents>\n")
    sb.WriteString("Available agent types for the Task tool:\n\n")
    for _, line := range lines {
        sb.WriteString(line + "\n")
    }
    sb.WriteString("</available-agents>")
    return sb.String()
}
```

### 3. Lazy Loading at Agent Execution

```go
// internal/agent/types.go
func (c *AgentConfig) GetSystemPrompt() string {
    if c.systemPromptLoaded || c.SourceFile == "" {
        return c.SystemPrompt
    }
    // Load from file on first call
    c.loadSystemPromptFromFile()
    return c.SystemPrompt
}

// internal/agent/loader.go
func LoadAgentSystemPrompt(filePath string) string {
    content, _ := os.ReadFile(filePath)
    _, body := extractFrontmatter(string(content))
    return strings.TrimSpace(body)
}
```

## Context Comparison

| Dimension | Main Loop | Agent Loop |
|-----------|-----------|------------|
| **System Prompt** | base + tools + provider + env + memory + skills/agents metadata | Agent-specific prompt (simplified) |
| **Agent Info** | `<available-agents>` with name:description only | Full SystemPrompt (lazy loaded) |
| **Tools** | All tools | Restricted (based on agent config) |
| **Permission** | TUI permission system | Independent (plan/acceptEdits/dontAsk) |
| **Model** | User-selected model | Can differ (inherit/sonnet/opus/haiku) |
| **Messages** | Full conversation history | Only agent task prompt |
| **MaxTurns** | Unlimited | Limited (default 10) |

## Memory Efficiency

```
Loaded at startup (per agent):
├── name: ~20 bytes
├── description: ~100 bytes
├── model: ~10 bytes
├── permission-mode: ~10 bytes
├── SourceFile: ~100 bytes
└── Total: ~240 bytes

Loaded at execution (lazy):
└── SystemPrompt: ~500-3000 bytes (on-demand)
```

## Skills Loading (Same Pattern)

Skills follow the same progressive loading pattern:

```go
// internal/skill/loader.go - At startup
skill.loaded = false  // Don't load instructions yet

// internal/skill/types.go - On invocation
func (s *Skill) GetInstructions() string {
    if !s.loaded && s.FilePath != "" {
        // Lazy load instructions from file
        if instructions, err := loadInstructions(s.FilePath); err == nil {
            s.Instructions = instructions
            s.loaded = true
        }
    }
    return s.Instructions
}
```

### Skills vs Agents Execution Model

| Aspect | Skills | Agents |
|--------|--------|--------|
| **Execution** | Main Loop (injected) | Isolated Loop (new instance) |
| **History** | Keeps conversation | Fresh start |
| **Injection** | `<skill-invocation>` tag (one turn) | Separate system prompt |
| **Cleanup** | Removed next turn | N/A (separate loop) |

## Benefits

1. **Fast Startup**: No need to read full content of all skill/agent files
2. **Memory Savings**: Unused skills/agents don't load Instructions/SystemPrompt
3. **Token Savings**: Main Loop system prompt only contains brief metadata
4. **On-Demand Loading**: Only invoked skills/executed agents load full content
