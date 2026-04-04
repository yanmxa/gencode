# Feature 10: Agents / Subagent System

## Overview

Agents are defined in AGENT.md files. They run their own `core.Loop` with an independent system prompt, tool set, and permission mode. They can be invoked headlessly or spawned from within the TUI via the `Agent` tool.

**AGENT.md frontmatter:**

```yaml
---
name: CodeReviewer
description: Reviews code changes for quality issues
model: inherit          # inherit | sonnet | opus | haiku
permission-mode: default
tools:
  - Read
  - Glob
  - Grep
max-turns: 50
mcp-servers: []
---
```

**Permission modes:**

| Mode | Behavior |
|------|----------|
| `default` | Interactive prompts |
| `acceptEdits` | Auto-accept edits |
| `dontAsk` | Convert prompts to denials |
| `plan` | Read-only |
| `bypassPermissions` | Auto-approve all |
| `auto` | Autonomous |

**Headless execution:**

```bash
gen agent run --type AgentName --prompt "task"
```

## UI Interactions

- **`/agents`**: picker to enable/disable agents.
- **Agent tool call**: TUI shows `SubagentStart` notification; progress indicator runs while the agent is active.
- **Agent output**: streamed back to the parent conversation as a tool result.
- **Background agents**: tracked in the task panel (Ctrl+T).

## Automated Tests

```bash
go test ./internal/agent/... -v
go test ./tests/integration/agent/... -v
```

Covered:

```
TestAgent_LazyLoading
TestAgent_Integration_Headless
TestAgent_Integration_MaxTurns_Respected
TestAgent_Integration_ToolRestriction
TestAgent_Integration_ModelOverride
```

Cases to add:

```go
func TestAgent_PlanPermissionMode_BlocksWrites(t *testing.T) {
    // Agent with permission-mode: plan must not write files
}

func TestAgent_ProgressCallback_Fires(t *testing.T) {
    // Progress callback must fire for each turn
}

func TestAgent_SubagentHooks_Fire(t *testing.T) {
    // SubagentStart and SubagentStop hooks must fire
}
```

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/agent_test/.gen/agents
echo "hello from agent test" > /tmp/agent_test/sample.txt

cat > /tmp/agent_test/.gen/agents/FileReader.md << 'EOF'
---
name: FileReader
description: Reads and summarizes text files
model: inherit
permission-mode: default
tools:
  - Read
  - Glob
max-turns: 10
---

You are a file reading agent. Read the requested file and provide a concise summary.
EOF

# Headless execution
gen agent run --type FileReader --prompt "read /tmp/agent_test/sample.txt and summarize" 2>&1
# Expected: agent reads file, outputs summary, exits cleanly

# Agent invoked from TUI
tmux new-session -d -s t_agent -x 220 -y 60
tmux send-keys -t t_agent 'cd /tmp/agent_test && gen' Enter
sleep 2
tmux send-keys -t t_agent 'use the FileReader agent to read sample.txt' Enter
sleep 15
tmux capture-pane -t t_agent -p
# Expected: SubagentStart shown; agent output in conversation; SubagentStop fires

tmux kill-session -t t_agent
rm -rf /tmp/agent_test
```
