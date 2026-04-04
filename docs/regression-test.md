# GenCode Regression Test Plan

This document covers regression testing for all gencode features from a functional perspective.

Each feature section defines:
- **Automated tests** — integration tests that can be run headlessly (`go test`)
- **Interactive tests** — step-by-step TUI verification via tmux sessions

---

## Table of Contents

1. [CLI Entry & Startup Modes](#1-cli-entry--startup-modes)
2. [Session & Conversation System](#2-session--conversation-system)
3. [Tool System (38 Tools)](#3-tool-system-38-tools)
4. [Slash Commands (18 Commands)](#4-slash-commands-18-commands)
5. [Provider / LLM System](#5-provider--llm-system)
6. [Hooks System](#6-hooks-system)
7. [Permission System](#7-permission-system)
8. [Plan Mode](#8-plan-mode)
9. [Skills System](#9-skills-system)
10. [Agents / Subagent System](#10-agents--subagent-system)
11. [MCP System](#11-mcp-system)
12. [Plugin System](#12-plugin-system)
13. [Cron / Scheduling System](#13-cron--scheduling-system)
14. [Task / Background Task System](#14-task--background-task-system)
15. [Compact / Conversation Compression](#15-compact--conversation-compression)
16. [Memory System](#16-memory-system)
17. [Worktree System](#17-worktree-system)
18. [Cost / Token Tracking](#18-cost--token-tracking)
19. [TUI Rendering & Interaction](#19-tui-rendering--interaction)
20. [Configuration System](#20-configuration-system)

---

## Environment Setup

```bash
# Build the gen binary
cd /home/cloud-user/workspace/gencode
go build -o /usr/local/bin/gen ./cmd/gen

# Verify tmux is available
tmux -V

# All interactive tests run inside tmux sessions
tmux new-session -s gentest
```

---

## 1. CLI Entry & Startup Modes

### Description

| Flag / Mode | Behavior |
|-------------|----------|
| `gen` | Launch interactive TUI chat |
| `gen -p "prompt"` | Non-interactive print mode — output to stdout, no TUI |
| `gen --plan "task"` | Start in plan mode (read-only exploration) |
| `gen -c` / `--continue` | Resume the most recent session |
| `gen -r` / `--resume` | Pick and resume a session from a list |
| `gen -c --fork` | Fork the most recent session into a new branch |
| `gen -r <id> --fork` | Fork a specific session by ID |
| `gen --plugin-dir PATH` | Load plugins from a directory |
| `gen version` | Print version string |
| `gen help` | Print help |

### Automated Tests

```bash
go test ./tests/integration/session/... -v

# Smoke-test non-interactive mode (requires a configured provider)
echo "say hello" | gen -p 2>&1 | grep -i hello

# Version and help do not require a provider
gen version
gen help
```

**Cases to add:**

```go
// tests/integration/cli/startup_test.go

func TestNonInteractivePrintMode(t *testing.T) {
    // -p must not launch a TUI; response must be on stdout
}

func TestPlanModeFlag_ToolsAreRestricted(t *testing.T) {
    // --plan flag: write tools must be blocked
}

func TestSessionContinue_LoadsHistory(t *testing.T) {
    // -c must restore the previous session's messages
}

func TestSessionFork_IsIndependent(t *testing.T) {
    // --fork must create a new session that doesn't affect the original
}
```

### Interactive Tests (tmux)

```bash
# Test 1: Basic startup
tmux new-session -d -s t_cli -x 200 -y 50
tmux send-keys -t t_cli 'gen' Enter
sleep 2
tmux capture-pane -t t_cli -p
# Expected: TUI appears with input box and status bar

# Test 2: Non-interactive print mode
tmux send-keys -t t_cli 'q' Enter
tmux send-keys -t t_cli 'gen -p "what is 1+1"' Enter
sleep 5
tmux capture-pane -t t_cli -p
# Expected: "2" printed to stdout; no TUI launched

# Test 3: Plan mode startup
tmux send-keys -t t_cli 'gen --plan "analyze this project"' Enter
sleep 2
tmux capture-pane -t t_cli -p
# Expected: TUI with [PLAN MODE] visible in status bar
tmux send-keys -t t_cli 'q' Enter

# Test 4: Session resume picker
tmux send-keys -t t_cli 'gen -r' Enter
sleep 2
tmux capture-pane -t t_cli -p
# Expected: Session selection list sorted by recency

tmux kill-session -t t_cli
```

---

## 2. Session & Conversation System

### Description

- **Persistence**: JSONL format, stored under `~/.gen/sessions/` or `./.gen/sessions/`
- **Message types**: User, Assistant, Tool Use, Tool Result, Notice, Thinking
- **Streaming**: Real-time token-by-token rendering
- **Metadata**: title, provider, model, cwd, created/updated timestamps
- **Resumption**: `-c` (latest), `-r <id>` (specific)
- **Fork**: branch from any session without modifying the original

### Automated Tests

```bash
go test ./tests/integration/session/... -v
go test ./internal/session/... -v
```

**Covered cases:**

```
TestSession_SaveAndLoad
TestSession_MetadataIndex
TestSession_Fork_IsIsolated
TestSession_ListSortedByTime
```

**Cases to add:**

```go
func TestSession_JSONL_Integrity(t *testing.T) {
    // Every line in the JSONL file must be valid JSON
}

func TestSession_ContinueRestoresMessages(t *testing.T) {
    // -c must replay all previous messages in correct order
}
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_sess -x 220 -y 60

# Test 1: Create a session and send messages
tmux send-keys -t t_sess 'gen' Enter
sleep 2
tmux send-keys -t t_sess 'hello, remember the number 42' Enter
sleep 8
# Expected: streaming assistant reply visible in TUI

# Test 2: Exit and resume
tmux send-keys -t t_sess 'q' Enter
sleep 1
tmux send-keys -t t_sess 'gen -c' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: previous session history is visible

# Test 3: Fork session
tmux send-keys -t t_sess 'q' Enter
tmux send-keys -t t_sess 'gen -c --fork' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: new session created; contains original history

# Test 4: Session list picker
tmux send-keys -t t_sess 'q' Enter
tmux send-keys -t t_sess 'gen -r' Enter
sleep 2
tmux capture-pane -t t_sess -p
# Expected: selectable list of sessions ordered by update time

tmux kill-session -t t_sess
```

---

## 3. Tool System (38 Tools)

### Description

Built-in tools by category:

| Category | Tools |
|----------|-------|
| File read | Read, Glob, Grep |
| File write | Write, Edit |
| Execution | Bash |
| Network | WebFetch, WebSearch |
| Task management | TaskCreate, TaskGet, TaskList, TaskUpdate, TaskStop, TaskOutput |
| Plan mode | EnterPlanMode, ExitPlanMode |
| Worktree | EnterWorktree, ExitWorktree |
| Agent | Agent, AskUserQuestion, Skill |
| Scheduling | CronCreate, CronDelete, CronList |
| System | Set, ToolSearch, SendMessage |
| MCP | ListMcpResourcesTool, ReadMcpResourceTool |

### Automated Tests

```bash
go test ./internal/tool/... -v
go test ./internal/app/tool/... -v
go test ./internal/config/... -v -run TestBashAST
```

**Covered cases:**

```
internal/tool/execute_test.go       — tool execution framework
internal/tool/exitplanmode_test.go  — ExitPlanMode approval flow
internal/tool/taskoutput_test.go    — TaskOutput streaming
internal/config/bash_ast_test.go    — dangerous Bash command detection
```

**Cases to add:**

```go
func TestRead_LineLimit_LargeFile(t *testing.T) {
    // Read must respect the line limit parameter on large files
}

func TestEdit_Fails_WhenOldStringNotUnique(t *testing.T) {
    // Edit must return an error if old_string matches more than once
}

func TestGlob_PatternMatching(t *testing.T) {
    // Verify ** and ? wildcard behavior
}

func TestBash_DeniedByPermission(t *testing.T) {
    // Bash tool blocked when deny rule matches the command
}
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_tools -x 220 -y 60

# Test: Bash tool execution with permission prompt
tmux send-keys -t t_tools 'gen -p "run: echo hello world"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: permission dialog appears (or executes if auto-accept); output "hello world"

# Test: Read tool
tmux send-keys -t t_tools 'gen -p "read /etc/hostname"' Enter
sleep 5
tmux capture-pane -t t_tools -p
# Expected: hostname content displayed

# Test: Write tool then verify
tmux send-keys -t t_tools 'gen -p "create /tmp/gentest.txt with content hello"' Enter
sleep 8
cat /tmp/gentest.txt
# Expected: file contains "hello"

# Test: /tools command — toggle tools on/off
tmux send-keys -t t_tools 'gen' Enter
sleep 2
tmux send-keys -t t_tools '/tools' Enter
sleep 2
tmux capture-pane -t t_tools -p
# Expected: full tool list shown with enable/disable toggles

tmux kill-session -t t_tools
rm -f /tmp/gentest.txt
```

---

## 4. Slash Commands (18 Commands)

### Description

| Command | Function |
|---------|----------|
| `/provider` | Switch LLM provider |
| `/model` | List and select a model |
| `/clear` | Clear chat history |
| `/fork` | Fork the current session |
| `/help` | Show available commands |
| `/glob` | Search files by glob pattern |
| `/tools` | Enable / disable tools |
| `/plan` | Enter plan mode |
| `/skills` | Manage skill states |
| `/agents` | Manage agents |
| `/tokenlimit` | View / set token budget |
| `/compact` | Compress conversation history |
| `/init` | Initialize GEN.md and config files |
| `/memory` | View / edit memory files |
| `/mcp` | Manage MCP servers |
| `/plugin` | Manage plugins |
| `/think` | Cycle thinking level (off / normal / high / ultra) |

### Automated Tests

```bash
go test ./internal/app/command/... -v
go test ./internal/app/memory/... -v
```

**Cases to add:**

```go
func TestCommandRegistry_AllCommandsPresent(t *testing.T) {
    // All 18 commands must be registered at startup
}

func TestSlashClear_ResetsConversation(t *testing.T) {
    // /clear must empty the message history
}

func TestSlashInit_CreatesGENmd(t *testing.T) {
    // /init in an empty directory must create .gen/GEN.md
}
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cmds -x 220 -y 60
tmux send-keys -t t_cmds 'gen' Enter
sleep 2

# /help
tmux send-keys -t t_cmds '/help' Enter
sleep 2
tmux capture-pane -t t_cmds -p
# Expected: all 18 commands listed

# /clear
tmux send-keys -t t_cmds 'hello' Enter
sleep 4
tmux send-keys -t t_cmds '/clear' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: history cleared; blank conversation view

# /think — cycle through levels
tmux send-keys -t t_cmds '/think' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: thinking level options shown (off / normal / high / ultra)

# /provider
tmux send-keys -t t_cmds '/provider' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: provider selection UI (anthropic / openai / google / ...)

# /model
tmux send-keys -t t_cmds '/model' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: model list for current provider

# /tokenlimit
tmux send-keys -t t_cmds '/tokenlimit' Enter
sleep 1
tmux capture-pane -t t_cmds -p
# Expected: current token usage and limit displayed

# /glob
tmux send-keys -t t_cmds '/glob *.go' Enter
sleep 2
tmux capture-pane -t t_cmds -p
# Expected: .go files in cwd listed

# /init — test in a fresh directory
tmux send-keys -t t_cmds 'q' Enter
tmux send-keys -t t_cmds 'mkdir -p /tmp/init_test && cd /tmp/init_test && gen' Enter
sleep 2
tmux send-keys -t t_cmds '/init' Enter
sleep 3
tmux capture-pane -t t_cmds -p
ls /tmp/init_test/.gen/
# Expected: GEN.md created under .gen/

tmux kill-session -t t_cmds
rm -rf /tmp/init_test
```

---

## 5. Provider / LLM System

### Description

Supported providers: **Anthropic**, **OpenAI**, **Google**, **Moonshot**, **Alibaba**

Anthropic authentication methods: API Key, Vertex AI, Amazon Bedrock.

**Thinking levels** (Anthropic only):

| Level | Trigger word | Budget tokens |
|-------|-------------|---------------|
| Off | — | 0 |
| Normal | `think` | 5,000 |
| High | `think+` | 32,000 |
| Ultra | `ultrathink` | 128,000 |

### Automated Tests

```bash
go test ./internal/provider/anthropic/... -v
go test ./internal/provider/moonshot/... -v
go test ./internal/provider/streamutil/... -v
go test ./internal/core/... -v
go test ./internal/client/... -v
```

**Covered cases:**

```
internal/provider/anthropic/client_test.go  — streaming response parsing
internal/core/core_test.go                  — LLM loop: request → tool call → continue
internal/client/client_test.go              — client wrapper behavior
```

**Cases to add:**

```go
func TestProvider_ModelListing(t *testing.T) {
    // ListModels must return a non-empty list for configured providers
}

func TestProvider_ThinkingBudget_SetCorrectly(t *testing.T) {
    // budget_tokens in the request must match the selected thinking level
}

func TestProvider_StreamChunk_OrderPreserved(t *testing.T) {
    // Chunks from a streaming response must arrive in order
}
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_prov -x 220 -y 60
tmux send-keys -t t_prov 'gen' Enter
sleep 2

# Switch provider
tmux send-keys -t t_prov '/provider' Enter
sleep 1
tmux capture-pane -t t_prov -p
# Expected: provider list; select one with arrow keys + Enter

# Switch model
tmux send-keys -t t_prov '/model' Enter
sleep 1
tmux capture-pane -t t_prov -p
# Expected: models for current provider

# Enable thinking
tmux send-keys -t t_prov '/think' Enter
sleep 1
# Select "normal"
tmux capture-pane -t t_prov -p
# Expected: status bar indicates thinking is on

tmux send-keys -t t_prov 'what is the sum of the first 100 prime numbers?' Enter
sleep 20
tmux capture-pane -t t_prov -p
# Expected: <thinking> block visible before the answer

tmux kill-session -t t_prov
```

---

## 6. Hooks System

### Description

**13 event types:**

| Event | Matcher support | When fired |
|-------|----------------|-----------|
| `SessionStart` | startup / resume / clear / compact | Session initialization |
| `UserPromptSubmit` | none | User submits a message |
| `PreToolUse` | tool name | Before a tool runs |
| `PermissionRequest` | tool name | Permission check |
| `PostToolUse` | tool name | After successful execution |
| `PostToolUseFailure` | tool name | After execution error |
| `Notification` | notification_type | Notification sent |
| `SubagentStart` | agent_type | Subagent starts |
| `SubagentStop` | agent_type | Subagent finishes |
| `Stop` | none | Session stops |
| `PreCompact` | manual / auto | Before compaction |
| `PostCompact` | manual / auto | After compaction |
| `SessionEnd` | reason | Session ends |

**Hook options:**

| Field | Description |
|-------|-------------|
| `type: "command"` | Execute a shell command |
| `async: true` | Fire-and-forget (non-blocking) |
| `timeout` | Max execution time in ms |
| `once: true` | Execute only once per session |

**I/O protocol:**
- stdin: JSON (`session_id`, `tool_name`, `tool_input`, …)
- stdout: JSON decision (`continue` / `block` / `updatedInput`)
- `exit 2` → block the tool call

### Automated Tests

```bash
go test ./tests/integration/hooks/... -v
go test ./internal/hooks/... -v
```

**Covered cases:**

```
TestHooks_BlockToolCall         — PreToolUse exit 2 blocks the tool
TestHooks_ModifyToolInput       — PreToolUse returns updatedInput
TestHooks_PostToolUse           — PostToolUse callback fires after success
TestHooks_AsyncExecution        — async hook does not block the main loop
TestHooks_SessionStart          — SessionStart event fires on startup
TestHooks_UserPromptSubmit      — UserPromptSubmit event fires
TestHooks_PermissionRequest     — PermissionRequest influences decision
```

**Cases to add:**

```go
func TestHooks_Timeout_TerminatesHook(t *testing.T) {
    // A hook exceeding its timeout must be killed; main loop continues
}

func TestHooks_Once_ExecutesExactlyOnce(t *testing.T) {
    // once:true hook must not fire on subsequent triggers
}

func TestHooks_Matcher_ToolNameWildcard(t *testing.T) {
    // Matcher pattern "Bash" must not match "BashTask"
}

func TestHooks_InputContains_SessionContext(t *testing.T) {
    // Hook stdin must include session_id and cwd
}
```

### Interactive Tests (tmux)

```bash
mkdir -p /tmp/hook_test/.gen

# Logging hook config
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "SessionStart": [{
      "hooks": [{"type": "command",
        "command": "echo '[hook] session started' >> /tmp/hook_log.txt"}]
    }],
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '[hook] bash pre-use' >> /tmp/hook_log.txt"}]
    }]
  }
}
EOF

tmux new-session -d -s t_hooks -x 220 -y 60
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 3
cat /tmp/hook_log.txt
# Expected: "[hook] session started" entry

tmux send-keys -t t_hooks 'run ls /tmp using bash' Enter
sleep 6
cat /tmp/hook_log.txt
# Expected: "[hook] bash pre-use" entry

# Blocking hook
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo 'blocked by policy' >&2; exit 2"}]
    }]
  }
}
EOF

tmux send-keys -t t_hooks 'run ls /tmp using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: tool blocked; "blocked by policy" shown to user

tmux kill-session -t t_hooks
rm -rf /tmp/hook_test /tmp/hook_log.txt
```

---

## 7. Permission System

### Description

**Permission modes:**

| Mode | Behavior |
|------|----------|
| `Normal` | Prompt user before every tool execution |
| `AutoAccept` | Auto-approve reads and edits |
| `Plan` | Only read-only tools allowed |
| `BypassPermissions` | Auto-approve all (bypass-immune checks still apply) |
| `DontAsk` | Convert prompts to automatic denials |

**Rule syntax:** `Tool(pattern)` — e.g. `Bash(npm:*)`, `Read(**/.env)`

- `allow` list — auto-approve matching calls
- `deny` list — block matching calls
- `ask` list — prompt user for matching calls

Working directory enforcement restricts edit operations to allowed paths.

### Automated Tests

```bash
go test ./internal/config/... -v -run TestPermission
go test ./internal/permission/... -v
go test ./tests/integration/permission/... -v
```

**Covered cases:**

```
TestPermission_AllowRule_AutoApproves
TestPermission_DenyRule_Blocks
TestPermission_BashAST_DangerousCommand
TestPermission_WorkDir_OutsideCwd_Blocked
```

**Cases to add:**

```go
func TestPermission_DontAskMode_DeniesAllPrompts(t *testing.T) {
    // DontAsk mode must convert every prompt into a denial
}

func TestPermission_BypassPermissions_BypassImmune_Enforced(t *testing.T) {
    // bypass-immune paths must still be blocked in BypassPermissions mode
}

func TestPermission_GlobPattern_MatchesCorrectly(t *testing.T) {
    // ** pattern must match nested paths; *.env must not match sub-dirs
}
```

### Interactive Tests (tmux)

```bash
mkdir -p /tmp/perm_test

# Test 1: Normal mode — confirmation dialog
tmux new-session -d -s t_perm -x 220 -y 60
tmux send-keys -t t_perm 'cd /tmp/perm_test && gen' Enter
sleep 2
tmux send-keys -t t_perm 'create file hello.txt with content world' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: permission dialog — press y to approve
tmux send-keys -t t_perm 'y' Enter
sleep 3
cat /tmp/perm_test/hello.txt
# Expected: "world"

# Test 2: Allow list — no prompt
cat > /tmp/perm_test/.gen/settings.json << 'EOF'
{"permissions": {"allow": ["Write(/tmp/perm_test/*)"]}}
EOF
tmux send-keys -t t_perm 'create file auto.txt with content ok' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: file created without any prompt

# Test 3: Deny list — blocked
cat > /tmp/perm_test/.gen/settings.json << 'EOF'
{"permissions": {"deny": ["Bash(rm*)"]}}
EOF
tmux send-keys -t t_perm 'run: rm -f /tmp/perm_test/hello.txt' Enter
sleep 5
tmux capture-pane -t t_perm -p
# Expected: Bash tool blocked by deny rule

tmux kill-session -t t_perm
rm -rf /tmp/perm_test
```

---

## 8. Plan Mode

### Description

- Only read-only tools are available: Read, Glob, Grep, WebFetch, WebSearch
- Write, Edit, Bash (write-capable), and Skills are disabled
- Exiting plan mode via `ExitPlanMode` requires explicit user approval
- Reachable via `gen --plan "..."` flag or `/plan` slash command

### Automated Tests

```bash
go test ./internal/tool/... -v -run TestExitPlanMode
go test ./internal/plan/... -v
```

**Covered cases:**

```
TestExitPlanMode_RequiresApproval
TestExitPlanMode_ApprovalGranted
TestExitPlanMode_ApprovalDenied_StaysInPlanMode
```

**Cases to add:**

```go
func TestPlanMode_BlocksWriteTools(t *testing.T) {
    // Write and Edit must return an error when plan mode is active
}

func TestPlanMode_AllowsReadTools(t *testing.T) {
    // Read, Glob, Grep must execute normally in plan mode
}

func TestPlanMode_StatusBar_ReflectsMode(t *testing.T) {
    // The UI model's mode field must equal Plan when active
}
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_plan -x 220 -y 60

# Start in plan mode via flag
tmux send-keys -t t_plan 'gen --plan "analyze the project layout"' Enter
sleep 2
tmux capture-pane -t t_plan -p
# Expected: status bar shows [PLAN MODE]

# Attempt a write — must be rejected
tmux send-keys -t t_plan 'create a file called out.txt' Enter
sleep 5
tmux capture-pane -t t_plan -p
# Expected: message that writes are not allowed in plan mode

# Read operation — must succeed
tmux send-keys -t t_plan 'read /etc/hostname' Enter
sleep 5
tmux capture-pane -t t_plan -p
# Expected: hostname content shown

# Enter plan mode via /plan command
tmux send-keys -t t_plan 'q' Enter
tmux send-keys -t t_plan 'gen' Enter
sleep 2
tmux send-keys -t t_plan '/plan' Enter
sleep 1
tmux capture-pane -t t_plan -p
# Expected: status bar switches to plan mode

# Exit plan mode (requires approval)
tmux send-keys -t t_plan 'done planning, please exit plan mode' Enter
sleep 5
tmux capture-pane -t t_plan -p
# Expected: plan summary displayed; user prompted to approve exit

tmux kill-session -t t_plan
```

---

## 9. Skills System

### Description

Skills are reusable prompts / workflows stored as Markdown files with YAML frontmatter.

**States:**

| State | Behavior |
|-------|----------|
| `Disable` | Hidden from user and model |
| `Enable` | Available as `/command`; model is unaware |
| `Active` | Included in system prompt; model is aware |

**Load scopes** (lowest → highest priority):

1. `~/.claude/skills/` — Claude user
2. `~/.gen/plugins/*/skills/` — User plugins
3. `~/.gen/skills/` — GenCode user
4. `./.claude/skills/` — Claude project
5. `./.gen/plugins/*/skills/` — Project plugins
6. `./.gen/skills/` — GenCode project

**Frontmatter fields:**

```yaml
---
name: review
namespace: git           # → invoked as /git:review
description: Review the last commit
allowed-tools: [Bash, Read]
argument-hint: <pr-number>
---
```

### Automated Tests

```bash
go test ./internal/skill/... -v
go test ./tests/integration/skill/... -v
```

**Covered cases:**

```
TestSkill_LoadFromDirectory
TestSkill_FrontmatterParsing
TestSkill_NamespaceResolution      — "git:review" → namespace=git name=review
TestSkill_StateToggle
TestSkill_LazyLoading              — skills loaded on demand, not at startup
TestSkill_Integration_Invoke
TestSkill_Integration_AllowedTools — tool calls outside allowed-tools are blocked
```

**Cases to add:**

```go
func TestSkill_ScopePriority_ProjectOverridesUser(t *testing.T) {
    // Project-level skill with same name must shadow user-level skill
}

func TestSkill_Active_AppearsInSystemPrompt(t *testing.T) {
    // An Active skill's content must appear in the system prompt sent to the LLM
}
```

### Interactive Tests (tmux)

```bash
mkdir -p /tmp/skill_test/.gen/skills/greet

cat > /tmp/skill_test/.gen/skills/greet/skill.md << 'EOF'
---
name: greet
description: Greet the user warmly
allowed-tools: []
---

Greet the user with enthusiasm. Say "Hello! Hope you're having a great day!" and nothing else.
EOF

tmux new-session -d -s t_skills -x 220 -y 60
tmux send-keys -t t_skills 'cd /tmp/skill_test && gen' Enter
sleep 2

# View skill state
tmux send-keys -t t_skills '/skills' Enter
sleep 1
tmux capture-pane -t t_skills -p
# Expected: "greet" skill listed

# Invoke the skill
tmux send-keys -t t_skills '/greet' Enter
sleep 5
tmux capture-pane -t t_skills -p
# Expected: assistant replies "Hello! Hope you're having a great day!"

# Namespaced skill
mkdir -p /tmp/skill_test/.gen/skills/git-review
cat > /tmp/skill_test/.gen/skills/git-review/skill.md << 'EOF'
---
name: review
namespace: git
description: Summarize the last git commit
allowed-tools: [Bash]
---

Run `git log -1 --stat` and summarize the most recent commit.
EOF

tmux send-keys -t t_skills '/git:review' Enter
sleep 8
tmux capture-pane -t t_skills -p
# Expected: Bash tool runs git log; commit summary displayed

tmux kill-session -t t_skills
rm -rf /tmp/skill_test
```

---

## 10. Agents / Subagent System

### Description

Agents are defined in AGENT.md files and can run headlessly or be spawned from within the TUI.

**Agent manifest fields:**

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
skills: []
system-prompt: "You are a senior code reviewer."
max-turns: 50
mcp-servers: []
---
```

**Permission modes for agents:**

| Mode | Behavior |
|------|----------|
| `default` | Interactive prompts |
| `acceptEdits` | Auto-accept edits |
| `dontAsk` | Convert prompts to denials |
| `plan` | Read-only |
| `bypassPermissions` | Auto-approve all |
| `auto` | Autonomous decision-making |

**Headless execution:** `gen agent run --type AgentName --prompt "task"`

### Automated Tests

```bash
go test ./internal/agent/... -v
go test ./tests/integration/agent/... -v
```

**Covered cases:**

```
TestAgent_LazyLoading
TestAgent_Integration_Headless
TestAgent_Integration_MaxTurns_Respected
TestAgent_Integration_ToolRestriction
TestAgent_Integration_ModelOverride
```

**Cases to add:**

```go
func TestAgent_PlanPermissionMode_BlocksWrites(t *testing.T) {
    // Agent with permission-mode: plan must not write files
}

func TestAgent_ProgressCallback_Fires(t *testing.T) {
    // Progress callback must fire for each turn during agent execution
}

func TestAgent_SubagentHooks_Fire(t *testing.T) {
    // SubagentStart and SubagentStop hooks must fire for agent execution
}
```

### Interactive Tests (tmux)

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

# Headless agent execution
gen agent run --type FileReader --prompt "read /tmp/agent_test/sample.txt and summarize" 2>&1
# Expected: agent reads file, outputs summary, exits cleanly

# Agent invoked from TUI via the Agent tool
tmux new-session -d -s t_agent -x 220 -y 60
tmux send-keys -t t_agent 'cd /tmp/agent_test && gen' Enter
sleep 2
tmux send-keys -t t_agent 'use the FileReader agent to read sample.txt' Enter
sleep 15
tmux capture-pane -t t_agent -p
# Expected: SubagentStart visible; agent output shown; SubagentStop fires

tmux kill-session -t t_agent
rm -rf /tmp/agent_test
```

---

## 11. MCP System

### Description

MCP (Model Context Protocol) connects gencode to external tool servers.

**Transport types:**

| Type | Configuration |
|------|--------------|
| STDIO | Local subprocess (`-- <command>`) |
| HTTP | REST endpoint (`--transport http <url>`) |
| SSE | Server-Sent Events |

**Config scopes:**
- `~/.gen/mcp.json` — user-level
- `./.gen/mcp.json` — project-level
- `./.gen/mcp.local.json` — local (git-ignored)

**CLI commands:**

```bash
gen mcp add <name> -- <command>              # STDIO
gen mcp add --transport http <name> <url>    # HTTP
gen mcp list
gen mcp get <name>
gen mcp edit <name>
gen mcp remove <name>
```

### Automated Tests

```bash
go test ./internal/mcp/... -v
go test ./tests/integration/mcp/... -v
```

**Covered cases:**

```
TestMCP_ConfigLoad
TestMCP_ScopeMerge
TestMCP_Registry_Connect
TestMCP_Registry_ListTools
TestMCP_STDIO_Transport
TestMCP_STDIO_JsonRPC
TestMCP_Integration_STDIO_Server
TestMCP_Integration_ToolExecution
```

**Cases to add:**

```go
func TestMCP_HTTP_Transport_Connect(t *testing.T) {
    // HTTP transport must connect and list tools correctly
}

func TestMCP_ResourceListing(t *testing.T) {
    // ListMcpResourcesTool must return resources from connected server
}
```

### Interactive Tests (tmux)

```bash
# Requires Node.js for the reference MCP server
tmux new-session -d -s t_mcp -x 220 -y 60

# Add a STDIO MCP server
tmux send-keys -t t_mcp 'gen mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem /tmp' Enter
sleep 5
tmux capture-pane -t t_mcp -p
# Expected: "filesystem" server added successfully

# List MCP servers
tmux send-keys -t t_mcp 'gen mcp list' Enter
sleep 2
tmux capture-pane -t t_mcp -p
# Expected: "filesystem" listed with STDIO transport

# Use MCP server from TUI
tmux send-keys -t t_mcp 'gen' Enter
sleep 2
tmux send-keys -t t_mcp 'list files in /tmp using the filesystem MCP tool' Enter
sleep 12
tmux capture-pane -t t_mcp -p
# Expected: /tmp directory listing via MCP filesystem server

# /mcp command in TUI
tmux send-keys -t t_mcp '/mcp' Enter
sleep 2
tmux capture-pane -t t_mcp -p
# Expected: MCP management UI listing configured servers

# Cleanup
tmux send-keys -t t_mcp 'q' Enter
tmux send-keys -t t_mcp 'gen mcp remove filesystem' Enter
sleep 2

tmux kill-session -t t_mcp
```

---

## 12. Plugin System

### Description

Plugins bundle skills, agents, hooks, MCP servers, and LSP servers into a single distributable unit.

**Plugin structure:**

```
my-plugin/
├── plugin.json          # Manifest
├── skills/              # Skill directories
├── agents/              # Agent definitions
├── hooks.json           # Hook configurations
├── mcp.json             # MCP servers
└── lsp.json             # LSP servers
```

**plugin.json:**

```json
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Short description",
  "author": { "name": "Author", "email": "", "url": "" }
}
```

**Load scopes:**
- `~/.gen/plugins/` — user-level
- `./.gen/plugins/` — project-level
- `./.gen/plugins-local/` — local (git-ignored)

**CLI commands:**

```bash
gen plugin list
gen plugin validate [path]
gen plugin install <plugin>@<marketplace>
gen plugin uninstall <plugin>
gen plugin enable <plugin>
gen plugin disable <plugin>
gen plugin info <plugin>
```

### Automated Tests

```bash
go test ./internal/plugin/... -v
go test ./tests/integration/plugin/... -v
```

**Covered cases:**

```
TestPlugin_ManifestParsing
TestPlugin_Validation
TestPlugin_ScopeLoading
TestPlugin_Integration_Enable
TestPlugin_Integration_Disable
TestPlugin_Integration_SkillFromPlugin
```

### Interactive Tests (tmux)

```bash
mkdir -p /tmp/my-plugin/skills/hello

cat > /tmp/my-plugin/plugin.json << 'EOF'
{
  "name": "my-plugin",
  "version": "1.0.0",
  "description": "Test plugin"
}
EOF

cat > /tmp/my-plugin/skills/hello/skill.md << 'EOF'
---
name: hello
description: Greet from plugin
allowed-tools: []
---

Say: "Hello from my-plugin!" and nothing else.
EOF

# Validate the plugin
gen plugin validate /tmp/my-plugin
# Expected: validation passes (no errors)

# Load via --plugin-dir
tmux new-session -d -s t_plugin -x 220 -y 60
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin' Enter
sleep 2
tmux send-keys -t t_plugin '/hello' Enter
sleep 5
tmux capture-pane -t t_plugin -p
# Expected: "Hello from my-plugin!" shown in response

# List plugins
tmux send-keys -t t_plugin 'q' Enter
tmux send-keys -t t_plugin 'gen plugin list' Enter
sleep 2
tmux capture-pane -t t_plugin -p

# /plugin command in TUI
tmux send-keys -t t_plugin 'gen --plugin-dir /tmp/my-plugin' Enter
sleep 2
tmux send-keys -t t_plugin '/plugin' Enter
sleep 2
tmux capture-pane -t t_plugin -p
# Expected: plugin management UI

tmux kill-session -t t_plugin
rm -rf /tmp/my-plugin
```

---

## 13. Cron / Scheduling System

### Description

- **Format**: 5-field cron — `minute hour day-of-month month day-of-week`
- **Syntax**: `*/5`, `1-10/2`, `1,3,5`, `*`, named values (`jan`, `mon`, …)
- **Persistence**: `.gen/scheduled_tasks.json`
- **Auto-expiry**: recurring jobs expire after 7 days

**Tool interface:**

```
CronCreate  schedule="*/5 * * * *"  prompt="check status"  once=false
CronDelete  job_id="<id>"
CronList    (no parameters)
```

### Automated Tests

```bash
go test ./internal/cron/... -v
```

**Covered cases:**

```
TestCron_ParseExpression
TestCron_NamedMonths
TestCron_NamedWeekdays
TestCron_StepValues
TestCron_RangeValues
TestCron_NextFireTime
TestCron_Persistence
```

**Cases to add:**

```go
func TestCron_Expiry_After7Days(t *testing.T) {
    // A recurring job older than 7 days must be automatically removed
}

func TestCron_Once_RemovedAfterFiring(t *testing.T) {
    // A once=true job must be deleted from the store after it fires
}

func TestCron_InvalidExpression_ReturnsError(t *testing.T) {
    // Malformed cron strings must return a descriptive parse error
}
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cron -x 220 -y 60
tmux send-keys -t t_cron 'gen' Enter
sleep 2

# Create a recurring job
tmux send-keys -t t_cron 'schedule a job every minute that says "tick"' Enter
sleep 5
tmux capture-pane -t t_cron -p
# Expected: CronCreate tool called; job ID returned

# List jobs
tmux send-keys -t t_cron 'list all cron jobs' Enter
sleep 3
tmux capture-pane -t t_cron -p
# Expected: CronList shows the scheduled job with next-fire time

# Wait for it to fire (~1 minute)
sleep 65
tmux capture-pane -t t_cron -p
# Expected: "tick" prompt was fired by the scheduler

# Delete all jobs
tmux send-keys -t t_cron 'delete all cron jobs' Enter
sleep 3
tmux capture-pane -t t_cron -p
# Expected: CronDelete called; no remaining jobs

tmux kill-session -t t_cron
```

---

## 14. Task / Background Task System

### Description

**Task types:**

| Type | Description |
|------|-------------|
| `BashTask` | Background shell command execution |
| `AgentTask` | Background agent execution |

**Task lifecycle:** Running → Completed / Failed / Killed

**Fields per task:** ID, Type, Description, Status, StartTime, EndTime, Duration, Output, Error, ExitCode (Bash), AgentName, TurnCount, TokenUsage (Agent)

**Ctrl+T** toggles the task panel in the TUI.

### Automated Tests

```bash
go test ./internal/task/... -v
go test ./internal/tool/... -v -run TestTaskOutput
```

**Covered cases:**

```
TestBashTask_Execute
TestBashTask_ExitCode
TestBashTask_Cancel
TestTaskManager_Create
TestTaskManager_List
TestTaskManager_Stop
TestTaskOutput_StreamsOutput
TestTaskOutput_CompletedTask
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_task -x 220 -y 60
tmux send-keys -t t_task 'gen' Enter
sleep 2

# Create a background task
tmux send-keys -t t_task 'run a background task: sleep 30 and then echo done' Enter
sleep 5
tmux capture-pane -t t_task -p
# Expected: TaskCreate called; task ID shown in response

# List tasks
tmux send-keys -t t_task 'list all tasks' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: task shown with "Running" status

# Toggle task panel with Ctrl+T
tmux send-keys -t t_task '' ''
sleep 1
tmux capture-pane -t t_task -p
# Expected: task panel appears/disappears at bottom of screen

# Stop the task
tmux send-keys -t t_task 'stop the background task' Enter
sleep 3
tmux capture-pane -t t_task -p
# Expected: task status becomes "Killed"

tmux kill-session -t t_task
```

---

## 15. Compact / Conversation Compression

### Description

- **Manual**: `/compact` command
- **Automatic**: triggered when context approaches the token limit
- **Effect**: summarises old messages, retains recent turns
- **Hooks**: `PreCompact` and `PostCompact` events fire around compaction

### Automated Tests

```bash
go test ./tests/integration/compact/... -v
```

**Covered cases:**

```
TestCompact_ManualTrigger
TestCompact_ReducesMessageCount
TestCompact_PreservesRecentMessages
TestCompact_UpdatesSessionMetadata
TestCompact_PreCompact_Hook
TestCompact_PostCompact_Hook
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_compact -x 220 -y 60
tmux send-keys -t t_compact 'gen' Enter
sleep 2

# Build up conversation history
for i in {1..5}; do
  tmux send-keys -t t_compact "message $i: tell me an interesting fact" Enter
  sleep 6
done

# Manual compact
tmux send-keys -t t_compact '/compact' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: compression summary shown; message history replaced by a summary

# Conversation continues normally after compact
tmux send-keys -t t_compact 'what were we talking about?' Enter
sleep 6
tmux capture-pane -t t_compact -p
# Expected: assistant references the summary context

tmux kill-session -t t_compact
```

---

## 16. Memory System

### Description

gencode reads project instructions from Markdown files at startup.

**Files:**
- `GEN.md` — project-level instructions (`./.gen/GEN.md`)
- `~/.gen/GEN.md` — user-level instructions
- `@import other.md` — inline import syntax

**Load scopes:**
1. User: `~/.gen/`
2. Project: `./.gen/`
3. Local: `./.gen/local/` (git-ignored)

**`/memory` command:** view and edit the loaded memory files.

### Automated Tests

```bash
go test ./internal/app/memory/... -v
go test ./internal/system/... -v
```

**Covered cases:**

```
TestMemory_LoadGENmd
TestMemory_ImportSyntax
TestMemory_ScopeMerge
TestMemory_Caching
```

**Cases to add:**

```go
func TestMemory_ProjectOverridesUser_SameName(t *testing.T) {
    // Project GEN.md must take precedence over user GEN.md
}

func TestMemory_ImportChain(t *testing.T) {
    // @import A, A imports B — both must be loaded into system prompt
}

func TestMemory_MissingFile_NoError(t *testing.T) {
    // Missing GEN.md must not crash startup
}
```

### Interactive Tests (tmux)

```bash
mkdir -p /tmp/mem_test/.gen
cat > /tmp/mem_test/.gen/GEN.md << 'EOF'
# Project Instructions

- Always respond in exactly one sentence.
- End every response with "(end)".
EOF

tmux new-session -d -s t_mem -x 220 -y 60
tmux send-keys -t t_mem 'cd /tmp/mem_test && gen' Enter
sleep 2
tmux send-keys -t t_mem 'hello' Enter
sleep 6
tmux capture-pane -t t_mem -p
# Expected: response is one sentence ending with "(end)"

# /memory command
tmux send-keys -t t_mem '/memory' Enter
sleep 2
tmux capture-pane -t t_mem -p
# Expected: GEN.md content displayed

tmux kill-session -t t_mem
rm -rf /tmp/mem_test
```

---

## 17. Worktree System

### Description

- Creates an isolated git worktree with its own branch and file system
- Each worktree has an independent context
- `ExitWorktree` accepts `keep=true` (retain) or `keep=false` (delete)

**Tool interface:**

```
EnterWorktree  branch="feature/x"  path="/tmp/wt-feature"
ExitWorktree   keep=true|false
```

### Automated Tests

```bash
go test ./internal/worktree/... -v
```

**Cases to add:**

```go
func TestWorktree_CreateAndEnter(t *testing.T) {
    // EnterWorktree must produce a valid git worktree at the specified path
}

func TestWorktree_Exit_Remove(t *testing.T) {
    // ExitWorktree keep=false must delete the worktree and its branch
}

func TestWorktree_Exit_Keep(t *testing.T) {
    // ExitWorktree keep=true must leave the worktree intact
}

func TestWorktree_RequiresGitRepo(t *testing.T) {
    // EnterWorktree outside a git repo must return a descriptive error
}
```

### Interactive Tests (tmux)

```bash
cd /home/cloud-user/workspace/gencode

tmux new-session -d -s t_wt -x 220 -y 60
tmux send-keys -t t_wt 'cd /home/cloud-user/workspace/gencode && gen' Enter
sleep 2

# Create a worktree
tmux send-keys -t t_wt 'enter a git worktree for branch test-worktree-branch' Enter
sleep 8
tmux capture-pane -t t_wt -p
# Expected: EnterWorktree tool called; worktree created

git worktree list
# Expected: new worktree listed

# Exit and remove the worktree
tmux send-keys -t t_wt 'exit the worktree and remove it' Enter
sleep 5
tmux capture-pane -t t_wt -p
git worktree list
# Expected: worktree no longer listed

tmux kill-session -t t_wt
```

---

## 18. Cost / Token Tracking

### Description

- **Tracked per turn**: input tokens, output tokens
- **Session totals**: cumulative across all turns
- **Displayed**: status bar shows running totals
- **Model-aware pricing**: cost calculated per model's rate card

### Automated Tests

```bash
go test ./internal/provider/... -v -run TestTokenUsage
go test ./internal/client/... -v -run TestCostTracking
```

**Cases to add:**

```go
func TestCost_PerTurnAccumulation(t *testing.T) {
    // Token counts must accumulate correctly across multiple turns
}

func TestCost_SessionTotal_MatchesSumOfTurns(t *testing.T) {
    // Session total must equal the sum of all per-turn counts
}

func TestCost_AnthropicPricing_CalculatedCorrectly(t *testing.T) {
    // Cost in USD must reflect current Anthropic model pricing
}
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_cost -x 220 -y 60
tmux send-keys -t t_cost 'gen' Enter
sleep 2

# Send a message and inspect status bar
tmux send-keys -t t_cost 'what is 2+2?' Enter
sleep 6
tmux capture-pane -t t_cost -p
# Expected: status bar updates with input/output token counts

# View token limit info
tmux send-keys -t t_cost '/tokenlimit' Enter
sleep 2
tmux capture-pane -t t_cost -p
# Expected: current usage and context limit displayed

# Accumulate across multiple turns
for i in {1..3}; do
  tmux send-keys -t t_cost "question $i: give me a short fact" Enter
  sleep 6
done
tmux capture-pane -t t_cost -p
# Expected: token count in status bar increases after each turn

tmux kill-session -t t_cost
```

---

## 19. TUI Rendering & Interaction

### Description

**Components:**

| Component | Description |
|-----------|-------------|
| Input box | Multi-line textarea with message history (↑/↓) |
| Output area | Markdown rendering with syntax highlighting |
| Status bar | Token counts, provider/model name, permission mode |
| Progress indicator | Spinner during streaming |
| Task panel | Ctrl+T toggles a bottom task list panel |

**Keyboard shortcuts:**

| Key | Action |
|-----|--------|
| `Enter` | Submit message |
| `Shift+Enter` | Insert newline |
| `↑` / `↓` | Navigate input history |
| `Ctrl+T` | Toggle task panel |
| `Ctrl+C` | Interrupt streaming / exit |

**Markdown features:** fenced code blocks with syntax highlighting, bold/italic, ordered/unordered lists, links.

### Automated Tests

```bash
go test ./internal/app/render/... -v
go test ./internal/app/input/... -v
```

**Covered cases:**

```
TestRender_Markdown_CodeBlock
TestRender_Markdown_BoldItalic
TestRender_Markdown_List
TestMarkdown_SyntaxHighlight
TestInput_MultilineEntry
TestInput_HistoryNavigation
```

### Interactive Tests (tmux)

```bash
tmux new-session -d -s t_tui -x 220 -y 60
tmux send-keys -t t_tui 'gen' Enter
sleep 2

# Markdown rendering — code block
tmux send-keys -t t_tui 'show me a python hello world example' Enter
sleep 8
tmux capture-pane -t t_tui -p
# Expected: ```python block rendered with syntax highlighting

# Multi-line input via Shift+Enter
tmux send-keys -t t_tui 'first line' ''
sleep 0.3
tmux send-keys -t t_tui 'second line' Enter
sleep 6
tmux capture-pane -t t_tui -p
# Expected: both lines sent as a single message

# Input history — up arrow
tmux send-keys -t t_tui '' ''
sleep 0.3
tmux capture-pane -t t_tui -p
# Expected: previous input appears in the input box

# Task panel toggle — Ctrl+T
tmux send-keys -t t_tui '' ''
sleep 0.3
tmux capture-pane -t t_tui -p
# Expected: task panel shown at bottom; Ctrl+T again hides it

# Interrupt streaming — Ctrl+C
tmux send-keys -t t_tui 'write a 1000-word essay about mountains' Enter
sleep 2
tmux send-keys -t t_tui '' ''
sleep 1
tmux capture-pane -t t_tui -p
# Expected: streaming interrupted; partial response visible

# Status bar content
tmux capture-pane -t t_tui -p | tail -3
# Expected: last lines include provider name, model, and token counts

tmux kill-session -t t_tui
```

---

## 20. Configuration System

### Description

**Load priority** (lowest → highest, higher wins):

| Priority | File |
|----------|------|
| 1 | `~/.claude/settings.json` (Claude user — compatibility) |
| 2 | `~/.gen/settings.json` (GenCode user) |
| 3 | `./.claude/settings.json` (Claude project — compatibility) |
| 4 | `./.gen/settings.json` (GenCode project) |
| 5 | `./.claude/settings.local.json` |
| 6 | `./.gen/settings.local.json` |
| 7 | CLI arguments / environment variables |
| 8 | `managed-settings.json` (system-level, read-only) |

**settings.json schema:**

```json
{
  "permissions": {
    "allow": ["Read(**)", "Glob(**)"],
    "deny": ["Bash(rm -rf*)"],
    "ask": ["Write(**)"]
  },
  "model": "claude-sonnet-4-6",
  "hooks": { "PreToolUse": [...] },
  "env": { "MY_VAR": "value" },
  "enabledPlugins": { "my-plugin": true },
  "disabledTools": { "WebSearch": true },
  "theme": "dark"
}
```

### Automated Tests

```bash
go test ./internal/config/... -v
```

**Covered cases:**

```
TestConfig_PermissionPriority
TestConfig_MergeMultipleScopes
TestConfig_WorkDirRestriction
TestConfig_Suggestion
```

**Cases to add:**

```go
func TestConfig_LocalOverridesProject(t *testing.T) {
    // settings.local.json must override settings.json at the same scope
}

func TestConfig_Env_InjectedIntoBashEnvironment(t *testing.T) {
    // Variables in "env" must be available when Bash tool executes commands
}

func TestConfig_DisabledTools_HiddenFromModel(t *testing.T) {
    // Tools listed in disabledTools must not appear in the LLM's tool list
}

func TestConfig_ManagedSettings_ReadOnly(t *testing.T) {
    // managed-settings.json values must not be overridden by user settings
}
```

### Interactive Tests (tmux)

```bash
mkdir -p /tmp/cfg_test/.gen

# Project-level overrides user-level env var
cat > ~/.gen/settings.json << 'EOF'
{"env": {"SCOPE": "user"}}
EOF
cat > /tmp/cfg_test/.gen/settings.json << 'EOF'
{"env": {"SCOPE": "project"}, "disabledTools": {"WebSearch": true}}
EOF

tmux new-session -d -s t_cfg -x 220 -y 60
tmux send-keys -t t_cfg 'cd /tmp/cfg_test && gen' Enter
sleep 2

# Verify env override
tmux send-keys -t t_cfg 'run: echo $SCOPE' Enter
sleep 5
tmux capture-pane -t t_cfg -p
# Expected: output is "project" (project config wins over user config)

# Verify disabled tool
tmux send-keys -t t_cfg '/tools' Enter
sleep 2
tmux capture-pane -t t_cfg -p
# Expected: WebSearch shown as disabled

tmux send-keys -t t_cfg 'q' Enter
tmux kill-session -t t_cfg
rm -rf /tmp/cfg_test
```

---

## Running All Automated Tests

```bash
cd /home/cloud-user/workspace/gencode

# All unit tests
go test ./internal/... -v 2>&1 | tee /tmp/unit_results.txt

# All integration tests
go test ./tests/integration/... -v 2>&1 | tee /tmp/integration_results.txt

# Per-package targets
go test ./internal/hooks/...      -v
go test ./internal/config/...     -v
go test ./internal/skill/...      -v
go test ./internal/agent/...      -v
go test ./internal/mcp/...        -v
go test ./internal/plugin/...     -v
go test ./internal/session/...    -v
go test ./internal/task/...       -v
go test ./internal/cron/...       -v
go test ./internal/permission/...  -v
go test ./internal/core/...       -v
go test ./internal/provider/...   -v
go test ./internal/app/render/... -v
go test ./internal/app/input/...  -v
go test ./internal/app/memory/... -v

# Coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

---

## Coverage Matrix

| Feature | Unit tests | Integration tests | Interactive (tmux) |
|---------|-----------|------------------|-------------------|
| CLI startup modes | partial | partial | **required** |
| Session system | complete | complete | recommended |
| Tool system (38) | partial | partial | **required** |
| Slash commands (18) | partial | none | **required** |
| Provider / LLM | complete | complete | recommended |
| Hooks system | complete | complete | recommended |
| Permission system | complete | complete | recommended |
| Plan mode | partial | none | **required** |
| Skills system | complete | complete | recommended |
| Agents system | partial | complete | recommended |
| MCP system | complete | complete | recommended |
| Plugin system | complete | complete | recommended |
| Cron scheduling | complete | none | **required** |
| Background tasks | complete | none | **required** |
| Compact | none | complete | recommended |
| Memory system | partial | none | **required** |
| Worktree | none | none | **required** |
| Cost / token tracking | partial | none | recommended |
| TUI rendering | complete | none | **required** |
| Configuration system | complete | none | recommended |
