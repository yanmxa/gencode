# Feature 6: Hooks System

## Overview

Hooks execute extensibility actions in response to events in the agent lifecycle. The runtime now supports Claude Code-style hook types beyond shell commands: `command`, `prompt`, `agent`, and `http`, plus in-memory runtime/session hook registration, including in-process function hooks. Beyond simple logging, hooks provide **bidirectional control** — external processes can observe events, block actions, modify inputs, change permissions, and, for command hooks, interact with users through a prompt protocol.

**24 event types:**

| Event | When fired | Matcher |
|-------|-----------|---------|
| `SessionStart` | Session initializes | `startup`, `resume` |
| `UserPromptSubmit` | User submits a message | — |
| `PreToolUse` | Before a tool runs | tool name |
| `PermissionRequest` | During permission check (async in TUI) | tool name |
| `PostToolUse` | After successful tool execution | tool name |
| `PostToolUseFailure` | After tool execution error | tool name |
| `Notification` | Notification sent | `notification_type` |
| `SubagentStart` | Subagent starts | agent type |
| `SubagentStop` | Subagent finishes | agent type |
| `Stop` | Session stops normally | — |
| `StopFailure` | Session stops because of an error | — |
| `PermissionDenied` | Tool request is denied | tool name |
| `Setup` | Runtime setup phase begins | `init` |
| `TaskCreated` | Background task is registered | task subject |
| `TaskCompleted` | Background task finishes | task subject |
| `ConfigChange` | A managed config file is persisted | config source |
| `InstructionsLoaded` | An instruction/memory file is loaded into runtime context | file path |
| `CwdChanged` | The active session working directory changes | new cwd |
| `FileChanged` | GenCode persists or applies a known file mutation | file path |
| `PreCompact` | Before compaction | `manual`, `auto` |
| `PostCompact` | After compaction | `manual`, `auto` |
| `WorktreeCreate` | A git worktree is created | worktree name |
| `WorktreeRemove` | A git worktree is removed | worktree path |
| `SessionEnd` | Session ends | reason |

**Hook options:**

| Field | Description |
|-------|-------------|
| `type: "command"` | Run a shell command hook |
| `type: "prompt"` | Run an LLM-backed verifier hook |
| `type: "agent"` | Run a multi-turn verifier hook in the app runtime when an agent runner is available; otherwise fall back to a higher-budget one-shot LLM verifier |
| `type: "http"` | POST hook input JSON to an HTTP endpoint |
| `async: true` | Non-blocking (fire-and-forget) |
| `asyncRewake: true` | Run in background and, if it later blocks, queue a follow-up model wake-up |
| `timeout` | Max execution time in seconds (default: 600) |
| `statusMessage` | Temporary status-bar text while the hook is active |
| `once: true` | Execute only once per session/source/matcher |
| `matcher` | Tool name or event subtype to filter on |
| `if` | Tool-aware permission-rule style filter (e.g. `Bash(git *)`) |
| `shell` | Optional command shell override (`sh` default, `powershell` supported) |
| `headers` / `allowedEnvVars` | HTTP hook request headers with explicit env interpolation allowlist |

**Input:** a hook input JSON object with common fields (`session_id`, `cwd`, `transcript_path`, `hook_event_name`, `permission_mode`) plus event-specific fields (`tool_name`, `tool_input`, `tool_response`, `last_assistant_message`, etc.). `command` hooks receive it on stdin, `prompt`/`agent` hooks receive it embedded in the verifier prompt, and `http` hooks receive it as the POST body.

**Output:** command/prompt/agent/http hooks all normalize to the same hook JSON response model. For command hooks, JSON is read from stdout; for prompt/agent hooks, the model must return only the hook JSON; for http hooks, the response body is parsed as hook JSON. Empty output = no-op.

**Command hook exit codes:** `0` = success, `2` = block (stderr becomes block reason), other = logged and ignored. `prompt` / `agent` / `http` hooks do not use process exit-code semantics.

`asyncRewake: true` is currently implemented for command hooks. GenCode runs the hook in the background; if it later resolves to a blocking outcome, the app queues a notice plus a synthetic user prompt so the model is re-awakened on the next idle tick.

`statusMessage` is tracked by the hook runtime and rendered from the active-hook set. This is most visible for hooks that execute off the main event loop, such as `PermissionRequest` hooks and background async hooks.

`SessionStart`, `CwdChanged`, and `FileChanged` hooks may return `hookSpecificOutput.watchPaths` (absolute paths). GenCode registers those paths with a lightweight polling watcher and emits `FileChanged` with `file_path` plus `event=add|change|unlink` when they change.

`FileChanged` currently combines two sources: direct GenCode-attributed writes (`Write`, `Edit`, memory file saves, plugin/MCP config writes) and watched external file mutations from `watchPaths`. It is still not a full recursive filesystem watcher.

## Reverse Control (Hook → GenCode)

Hooks can control GenCode in **8 distinct ways**, from simple blocking to full bidirectional interaction:

### 1. Block/Continue Control

Exit code 2 or JSON `{"continue": false}` blocks the current operation.

```bash
# Block via exit code (stderr = reason shown to user)
echo "rm commands are forbidden" >&2; exit 2

# Block via JSON output
echo '{"continue": false, "reason": "blocked by policy"}'
```

Works on: `PreToolUse`, `PermissionRequest`, `UserPromptSubmit`

### 2. Modify Tool Input (`updatedInput`)

Change tool parameters before execution — the tool runs with modified args.

```bash
# Force --dry-run on all npm commands
INPUT=$(cat)
CMD=$(echo "$INPUT" | jq -r '.tool_input.command')
echo "{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"updatedInput\":{\"command\":\"$CMD --dry-run\"}}}"
```

Works on: `PreToolUse`, `PermissionRequest`

### 3. Inject System Context (`systemMessage`)

Add context to the conversation before the next model step. In the current implementation this is appended into the conversation as an extra chat message, not a true system-role message.

```bash
# Inject code review guidelines before tool execution
echo '{"systemMessage": "Remember: all file edits must include unit tests"}'
```

Works on: `PreToolUse` (via `additionalContext` in hookSpecificOutput)

### 4. Session Bootstrap (`initialUserMessage`)

`SessionStart` hooks can seed the first user turn for a fresh session.

```bash
echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","initialUserMessage":"Inspect the repository and summarize the highest-risk areas first."}}'
```

GenCode stores this as the pending initial prompt and submits it through the normal startup path.

Works on: `SessionStart`

### 5. PreToolUse Permission Decision

Grant, deny, or force-ask permission at the PreToolUse stage, before the normal permission check.

Three `permissionDecision` values:

| Value | Effect |
|-------|--------|
| `"allow"` | Skip permission prompt (subject to safety invariant) |
| `"deny"` | Block tool execution with reason |
| `"ask"` | Force permission prompt even for normally auto-allowed tools |

```bash
# Auto-allow Read tool for all files
echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow"}}'

# Deny with reason
echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"audit policy"}}'

# Force permission prompt (even if tool would normally auto-execute)
echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask"}}'
```

Works on: `PreToolUse` only. Subject to safety invariant (deny rules > bypass-immune > ask rules > hook allow).

**Note:** PreToolUse hooks cannot inject `updatedPermissions` (setMode, addRules, addDirectories) — that capability is exclusive to PermissionRequest hooks (RC6 below).

### 6. PermissionRequest Decision with Permission Updates

The most powerful reverse control — a PermissionRequest hook can allow/deny AND change session permissions:

```bash
# Allow once
echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}'

# Allow + activate bypass mode for the session
echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedPermissions":[{"type":"setMode","mode":"bypassPermissions","destination":"session"}]}}}'

# Allow + add a persistent rule
echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedPermissions":[{"type":"addRules","rules":[{"toolName":"Bash","ruleContent":"git"}],"behavior":"allow","destination":"persistent"}]}}}'

# Allow + grant directory access
echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedPermissions":[{"type":"addDirectories","directories":["/tmp"],"destination":"session"}]}}}'

# Deny
echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"not allowed by admin"}}}'
```

**Three `updatedPermissions` types:**

| Type | Fields | Effect |
|------|--------|--------|
| `setMode` | `mode`: `bypassPermissions`, `acceptEdits`, `dontAsk`, `plan`, `normal` | Change session permission mode |
| `addRules` | `rules[]`: `{toolName, ruleContent}`, `behavior`, `destination` | Add allow/deny patterns |
| `addDirectories` | `directories[]`, `destination` | Whitelist dirs for Edit/Write |

`destination`: `"session"` (default, current session only) or `"persistent"` (saved to settings file).

**Async execution**: In the TUI, PermissionRequest hooks run asynchronously (via `tea.Cmd`) so the UI stays responsive while waiting for external responses (e.g., FIFO-based monitor terminals).

### 7. PermissionDenied Retry

`PermissionDenied` hooks can tell GenCode to continue the model loop after a denial so the assistant can choose a different plan or tool.

```bash
echo '{"hookSpecificOutput":{"hookEventName":"PermissionDenied","retry":true}}'
```

Current behavior: GenCode still appends the denied tool result, then immediately resumes the assistant turn.

Works on: `PermissionDenied`

### 8. Bidirectional Prompt Protocol

`command` hooks can request user input through a multi-turn stdin/stdout protocol, enabling interactive decision-making:

```
Hook stdout → {"prompt": "confirm_deploy", "message": "Deploy to prod?", "options": [{"key": "yes", "label": "Deploy"}, {"key": "no", "label": "Cancel"}]}
GenCode stdin → {"prompt_response": "confirm_deploy", "selected": "yes"}
Hook stdout → (final JSON output or another prompt)
```

The command-hook protocol:
1. GenCode writes the initial hook input JSON + `\n` to the hook's stdin
2. The hook writes a `PromptRequest` JSON object to stdout
3. GenCode presents it to the user, then writes `PromptResponse` back to the hook's stdin
4. Repeat for multi-turn interaction until the hook writes final `HookOutput` JSON and exits

First-line async detection: if a command hook's first stdout line is `{"async": true}`, GenCode detaches the hook to run in the background.

`asyncRewake` differs from first-line `{"async":true}` detach: it is configured declaratively in settings and only triggers a re-wake when the background hook finishes with a blocking result.

## Settings Priority

Hook configurations follow "last write wins per event" for settings files:
- Project `.gen/settings.json` overrides user `~/.gen/settings.json` **per event type**
- If a project defines `Stop` hooks, ALL user-level `Stop` hooks are replaced
- Events not defined at project level inherit from user level

In addition to persisted settings and plugin hooks, the runtime also supports:
- **Runtime hooks**: registered in memory for the current process
- **Session hooks**: registered in memory for the current engine/session instance and cleared after `SessionEnd`
- **Function hooks**: in-memory callbacks registered through the hook engine; these are runtime/session-scoped only and are not persisted to settings

When loading config, native GenCode hooks take precedence per event. Claude-compat hooks are used only when no native hook exists for that event, with one important exception: Claude-compat `PermissionRequest` hooks are skipped because their interactive protocol does not match GenCode's TUI approval flow.

**Implication**: to add a project hook without losing user hooks for the same event, include both in the project config.

## UI Interactions

- Blocked tool calls show an error message with the hook's stderr output or JSON reason.
- Modified tool input (via `updatedInput`) is applied silently before execution.
- `SessionStart.initialUserMessage` becomes the pending startup prompt when no explicit CLI prompt is already queued.
- PermissionRequest hooks run async — the TUI remains responsive during external approval.
- Permission mode changes (via `setMode`) take effect immediately for subsequent tools.
- `PermissionDenied.retry` resumes the assistant turn after recording the denied tool result.
- `statusMessage` is shown from currently active hooks; the newest active hook wins.
- Async hooks (`async: true`) do not affect UI response time.
- `asyncRewake: true` background hooks can enqueue an idle-time model wake-up when they later block.

## Automated Tests

```bash
go test ./internal/hook ./internal/app/... ./internal/tool ./internal/worktree ./internal/plugin ./internal/mcp ./tests/integration/hooks ./tests/integration/plugin
```

Current automated coverage is split across engine tests, app-runtime tests, observer wiring tests, and integration tests.

### Engine Coverage (`internal/hook/hooks_test.go`)

```
TestEngineNoHooks                               — no configured hooks is a no-op
TestEngineHasHooks                              — persisted/runtime/session hook detection
TestEngineRuntimeAndSessionHooks                — runtime + session hook execution
TestEngineSessionFunctionHook                   — in-memory function hooks execute
TestEngineRemoveSessionFunctionHook             — function hooks can be removed
TestEngineBlockingHook                          — exit code 2 blocks execution
TestHooks_Timeout_TerminatesHook                — timeout kills long-running command hook
TestHooks_Once_ExecutesExactlyOnce              — once:true executes once per event/source/matcher
TestHooks_InputContains_SessionContext          — session_id / cwd / transcript_path passed through
TestHooks_PermissionModeIncludedOnlyForRelevantEvents — permission_mode only on relevant events
TestHooks_InjectAdditionalContext               — additionalContext merges into outcome
TestHooks_ExtractWatchPaths                     — watchPaths extracted from hook output
TestHooks_ExtractInitialUserMessage             — SessionStart initialUserMessage extracted
TestHooks_CurrentStatusMessageTracksActiveHook  — active statusMessage tracked while hook runs
TestHooks_ExtractPermissionDeniedRetry          — PermissionDenied retry extracted
TestHooks_InjectSystemMessage                   — systemMessage mapped to additional context
TestHooks_PreToolUse_PermissionAllow            — PreToolUse allow
TestHooks_PreToolUse_PermissionDeny             — PreToolUse deny with reason
TestHooks_PreToolUse_PermissionAsk              — PreToolUse ask forces prompt
TestHooks_PreToolUse_DecisionFieldIgnored       — PreToolUse ignores PermissionRequest-style decision payloads
TestHooks_PromptHook                            — prompt hook execution
TestHooks_AgentHook                             — one-shot/fallback agent hook execution
TestHooks_AgentHook_UsesInjectedRunner          — injected multi-turn runner path
TestHooks_HTTPHook                              — http hook execution
TestHooks_PermissionRequest_AllowSimple         — PermissionRequest allow
TestHooks_PermissionRequest_Deny                — PermissionRequest deny
TestHooks_PermissionRequest_AllowWithSetMode    — setMode update
TestHooks_PermissionRequest_AllowWithAddRules   — addRules update
TestHooks_PermissionRequest_AllowWithAddDirectories — addDirectories update
TestHooks_PermissionRequest_AllowWithUpdatedInput — allow + updatedInput
TestHooks_BidirectionalPrompt_SingleRound       — one prompt request/response cycle
TestHooks_BidirectionalPrompt_UserDeclines      — decline path
TestHooks_BidirectionalPrompt_Cancelled         — cancelled prompt path
TestHooks_BidirectionalPrompt_MultiRound        — multi-round prompt protocol
TestHooks_BidirectionalPrompt_AsyncDetach       — first-line {"async":true} detach
TestHooks_AsyncRewakeCallback                   — asyncRewake callback fires on background block
TestHooks_SessionStartOmitsPermissionMode       — SessionStart omits permission_mode
TestMatchesEvent / TestGetMatchValue / TestEventSupportsMatcher — matcher semantics
```

### App Runtime Coverage (`internal/app/model_test.go`)

```go
TestFireSessionEndClearsSessionHooks            — SessionEnd teardown clears session hooks
TestInitFiresSetupHook                          — Setup(init) fires during Init
TestRefreshMemoryContextFiresInstructionsLoaded — InstructionsLoaded fires while loading memory
TestChangeCwdFiresCwdChanged                    — CwdChanged fires on real cwd transition
TestApplyToolResultSideEffectsFiresFileChanged  — Write/Edit side effects emit FileChanged
TestInitRegistersWatchPathsFromSessionStart     — SessionStart watchPaths register watcher state
TestFileWatcherFiresFileChangedForWatchedPath   — watched external file mutation emits FileChanged
TestApplyRuntimeHookOutcomeSetsInitialPrompt    — initialUserMessage stored as startup prompt
TestPermissionDeniedRetryContinuesStream        — retry resumes assistant turn after denial
TestAsyncHookTickRewakesModel                   — asyncRewake queue injects notice + user prompt
TestAsyncHookTickRefreshesHookStatus            — hook status is refreshed into app state
```

### Observer / Bridge Coverage

```go
internal/task/hooks_test.go                     — TaskCreated / TaskCompleted observer wiring
internal/worktree/hooks_test.go                 — WorktreeCreate / WorktreeRemove observer wiring
internal/plugin/hooks_test.go                   — plugin config change observer wiring
internal/mcp/hooks_test.go                      — MCP config change observer wiring
```

### Integration Coverage

```go
tests/integration/hooks                         — end-to-end persisted hook loading/execution
tests/integration/plugin                        — plugin-provided hook loading and execution
```

### Remaining Gaps

The documentation now only claims the tests above as automated coverage. There is still no dedicated automated test file for every event/output combination. In particular, `Stop`, `StopFailure`, `PostCompact`, `PostToolUseFailure`, and some subagent-specific paths are primarily validated indirectly through runtime wiring rather than by one focused hook test each.

## Interactive Tests (tmux)

```bash
mkdir -p /tmp/hook_test/.gen

# ── Test 1: Logging — SessionStart and PreToolUse ──
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
# Expected: "[hook] session started"

tmux send-keys -t t_hooks 'run ls /tmp using bash' Enter
sleep 6
cat /tmp/hook_log.txt
# Expected: "[hook] bash pre-use" appended

# ── Test 2: Block/Continue — PreToolUse exit 2 ──
tmux send-keys -t t_hooks C-c
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
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run ls /tmp using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: tool blocked; "blocked by policy" shown to user

# ── Test 3: Modify Input — updatedInput ──
tmux send-keys -t t_hooks C-c
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"updatedInput\":{\"command\":\"echo modified\"}}}'"}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo original using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: "modified" output instead of "original"

# ── Test 4: Inject Context — systemMessage ──
tmux send-keys -t t_hooks C-c
rm -f /tmp/hook_log.txt
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "*",
      "hooks": [{"type": "command",
        "command": "echo '{\"systemMessage\":\"IMPORTANT: always use --dry-run\"}'"}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo hello using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: tool executes normally; LLM receives injected context

# ── Test 5: PreToolUse Permission Decision — auto-allow ──
tmux send-keys -t t_hooks C-c
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"allow\"}}'"}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo auto-allowed using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: Bash executes WITHOUT permission prompt (hook pre-allowed)

# ── Test 6: PreToolUse Permission Decision — deny ──
tmux send-keys -t t_hooks C-c
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '{\"hookSpecificOutput\":{\"hookEventName\":\"PreToolUse\",\"permissionDecision\":\"deny\",\"permissionDecisionReason\":\"audit: bash disabled\"}}'"}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo blocked using bash' Enter
sleep 5
tmux capture-pane -t t_hooks -p
# Expected: tool blocked; "audit: bash disabled" shown

# ── Tests 7–8: PermissionRequest — multi-pane FIFO monitor ──
#
# These tests use a split-pane layout so you can watch the full hook event
# details in a monitor pane while gen runs in a separate pane.
#
# Layout (3 panes in one tmux window):
#   ┌──────────────┬──────────────────────┐
#   │              │  Monitor (top-right) │
#   │  (your       │  shows hook events   │
#   │   terminal)  ├──────────────────────┤
#   │              │  Gen TUI (bot-right) │
#   │              │  runs gencode        │
#   └──────────────┴──────────────────────┘

tmux send-keys -t t_hooks C-c
sleep 1
rm -f /tmp/hook_pr_fifo /tmp/hook_pr_event.json
mkfifo /tmp/hook_pr_fifo

# Hook script: captures full event JSON for the monitor to display,
# then waits for the monitor's allow/deny/bypass decision via FIFO.
cat > /tmp/hook_test/pr_hook.sh << 'HOOKEOF'
#!/bin/bash
INPUT=$(cat)
echo "$INPUT" > /tmp/hook_pr_event.json
echo "EVENT_READY" > /tmp/hook_pr_fifo
RESP=$(cat /tmp/hook_pr_fifo)
if [ "$RESP" = "allow" ]; then
  echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}'
elif [ "$RESP" = "bypass" ]; then
  echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow","updatedPermissions":[{"type":"setMode","mode":"bypassPermissions","destination":"session"}]}}}'
else
  echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"deny","message":"'"$RESP"'"}}}'
fi
HOOKEOF
chmod +x /tmp/hook_test/pr_hook.sh

# Monitor script: loops waiting for hook events, displays event details,
# and prompts for a decision (allow / deny <reason> / bypass).
cat > /tmp/hook_test/monitor.sh << 'MONEOF'
#!/bin/bash
echo "╔══════════════════════════════════════════════╗"
echo "║   Permission Request Monitor                ║"
echo "║   Waiting for hook events on FIFO...         ║"
echo "╚══════════════════════════════════════════════╝"
echo ""
while true; do
  SIGNAL=$(cat /tmp/hook_pr_fifo 2>/dev/null)
  [ -z "$SIGNAL" ] && { echo "[monitor] FIFO closed."; break; }
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  echo "PermissionRequest event received!"
  echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
  if [ -f /tmp/hook_pr_event.json ]; then
    echo ""
    echo "-- Event Fields --"
    python3 -c "
import json
with open('/tmp/hook_pr_event.json') as f:
    data = json.load(f)
for key, label in [('hook_event_name','Event'),('tool_name','Tool'),('session_id','Session'),('cwd','CWD'),('permission_mode','Perm Mode')]:
    if key in data: print(f'  {label:20s}: {data[key]}')
if 'tool_input' in data:
    print(f'  {\"Tool Input\":20s}:')
    ti = data['tool_input']
    if isinstance(ti, dict):
        for k, v in ti.items():
            val = str(v)[:60]
            print(f'    {k:18s}: {val}')
if 'permission_suggestions' in data and data['permission_suggestions']:
    print(f'  {\"Suggestions\":20s}:')
    for s in data['permission_suggestions']:
        print(f'    - {s}')
" 2>/dev/null
    echo ""
  fi
  echo "Enter decision (allow / deny <reason> / bypass):"
  read -r DECISION
  echo ">>> Sending: $DECISION"
  echo "$DECISION" > /tmp/hook_pr_fifo
  echo ""
done
MONEOF
chmod +x /tmp/hook_test/monitor.sh

cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PermissionRequest": [{
      "matcher": "*",
      "hooks": [{"type": "command",
        "command": "/tmp/hook_test/pr_hook.sh", "timeout": 60}]
    }]
  }
}
EOF

# Create the split-pane layout:
#   Split t_hooks horizontally, then split the right half vertically.
tmux split-window -h -t t_hooks -l '55%'
sleep 0.3
tmux split-window -v -t t_hooks.right -l '50%'
sleep 0.3
# After splitting:  pane 0 = left,  pane 1 = top-right,  pane 2 = bottom-right

# Start monitor in top-right pane
tmux send-keys -t t_hooks.1 '/tmp/hook_test/monitor.sh' Enter
sleep 1

# Start gen in bottom-right pane
tmux send-keys -t t_hooks.2 'cd /tmp/hook_test && gen' Enter
sleep 3

# ── Test 7a: FIFO allow ──
tmux send-keys -t t_hooks.2 'write hello to /tmp/hook_pr_test.txt' Enter
sleep 10
# Expected in monitor pane:
#   Event: PermissionRequest, Tool: Write,
#   Tool Input: content=hello, file_path=/tmp/hook_pr_test.txt
# Expected in gen pane: permission modal with diff preview
# Type "allow" in monitor pane:
tmux send-keys -t t_hooks.1 'allow' Enter
sleep 5
cat /tmp/hook_pr_test.txt
# Expected: "hello"

# ── Test 7b: FIFO deny ──
tmux send-keys -t t_hooks.2 'write secret to /tmp/hook_deny_test.txt' Enter
sleep 10
# Monitor shows new event with content=secret
# Type deny with reason:
tmux send-keys -t t_hooks.1 'deny sensitive content blocked' Enter
sleep 5
tmux capture-pane -t t_hooks.2 -p
# Expected in gen pane: "Blocked by hook: deny sensitive content blocked"
# Expected: /tmp/hook_deny_test.txt does NOT exist

# ── Test 8: FIFO bypass mode activation + verification ──
tmux send-keys -t t_hooks.2 'write bp1 to /tmp/bp1.txt' Enter
sleep 10
# Monitor shows event with content=bp1
# Type "bypass" to activate bypassPermissions mode:
tmux send-keys -t t_hooks.1 'bypass' Enter
sleep 5
tmux capture-pane -t t_hooks.2 -p
# Expected in gen pane:
#   - File written successfully
#   - Bottom indicator shows: ⏩ bypass permissions on

# Now verify bypass is active — subsequent writes must NOT trigger hook:
tmux send-keys -t t_hooks.2 'write bp2 to /tmp/bp2.txt' Enter
sleep 8
tmux send-keys -t t_hooks.2 'write bp3 to /tmp/bp3.txt' Enter
sleep 8
cat /tmp/bp1.txt /tmp/bp2.txt /tmp/bp3.txt
# Expected: all three files created with correct content
tmux capture-pane -t t_hooks.1 -p | grep -c 'PermissionRequest event received'
# Expected: exactly 1 (only bp1 triggered the hook; bp2 and bp3 bypassed)
tmux capture-pane -t t_hooks.2 -p
# Expected: ⏩ bypass permissions on — still visible, no permission modals appeared

# Close the extra panes before continuing to test 9
tmux send-keys -t t_hooks.2 C-c
sleep 1
tmux kill-pane -t t_hooks.2
tmux kill-pane -t t_hooks.1
sleep 1
rm -f /tmp/hook_pr_fifo /tmp/hook_pr_event.json /tmp/hook_pr_test.txt /tmp/hook_deny_test.txt /tmp/bp*.txt

# ── Test 9: PostToolUse hook fires ──
tmux send-keys -t t_hooks C-c
rm -f /tmp/hook_log.txt /tmp/hook_pr_fifo
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PostToolUse": [{
      "hooks": [{"type": "command",
        "command": "echo '[hook] post-tool-use' >> /tmp/hook_log.txt"}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo hello using bash' Enter
sleep 5
cat /tmp/hook_log.txt
# Expected: "[hook] post-tool-use"

# ── Test 10: Once hook fires only once ──
tmux send-keys -t t_hooks C-c
rm -f /tmp/hook_log.txt
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '[hook] once' >> /tmp/hook_log.txt",
        "once": true}]
    }]
  }
}
EOF
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run echo first using bash' Enter
sleep 5
tmux send-keys -t t_hooks 'run echo second using bash' Enter
sleep 5
wc -l /tmp/hook_log.txt
# Expected: exactly 1 line (hook fired only once)

# ── Test 11: PermissionDenied hook ──
tmux send-keys -t t_hooks C-c
cat > /tmp/hook_test/.gen/settings.json << 'EOF'
{
  "permissions": {"deny": ["Bash(rm*)"]},
  "hooks": {
    "PermissionDenied": [{
      "matcher": "Bash",
      "hooks": [{"type": "command",
        "command": "echo '[hook] permission denied' >> /tmp/hook_log.txt"}]
    }]
  }
}
EOF
rm -f /tmp/hook_log.txt
tmux send-keys -t t_hooks 'cd /tmp/hook_test && gen' Enter
sleep 2
tmux send-keys -t t_hooks 'run rm -f /tmp/x using bash' Enter
sleep 4
cat /tmp/hook_log.txt
# Expected: "[hook] permission denied"

tmux kill-session -t t_hooks
rm -rf /tmp/hook_test /tmp/hook_log.txt /tmp/hook_pr_fifo /tmp/hook_pr_event.json \
       /tmp/hook_pr_test.txt /tmp/hook_deny_test.txt /tmp/bp*.txt
```
