# Subagent System Manual Testing Guide

This guide provides comprehensive manual testing instructions for all Phase 2-4 features of the subagent system.

## Prerequisites

```bash
# 1. Build the project
cd /Users/myan/Workspace/ideas/gencode
npm run build

# 2. Start GenCode CLI
npm start
```

## Feature Coverage Checklist

### ✅ Phase 2: Background Execution
- [x] `run_in_background` parameter support in Task tool
- [x] NDJSON event streaming to `~/.gen/tasks/{task-id}/output.log`
- [x] TaskOutput tool with 5 actions (status, list, result, wait, cancel)
- [x] Concurrent task limit (max 10)
- [x] Output size limit (10MB per task)
- [x] Automatic cleanup (24 hours)
- [x] Task metadata tracking (status, duration, token usage)

### ✅ Phase 3: Resume Capability
- [x] `resume` parameter support in Task tool
- [x] Automatic session persistence for all subagents
- [x] Session validation (7-day expiry, 5-resume quota)
- [x] Resume count tracking
- [x] Backward compatible SessionMetadata extension
- [x] SubagentSessionManager implementation

### ✅ Phase 4: Advanced Features
- [x] Parallel execution via `tasks` array parameter
- [x] Custom agent loading from `~/.gen/agents/` and `~/.claude/agents/`
- [x] Merge mechanism (GenCode priority > Claude Code)
- [x] JSON format support (GenCode native)
- [x] Markdown format support (Claude Code compatible)
- [x] Source tracking for each agent
- [x] Inter-agent communication with depth limit (max 3)
- [x] Result caching with TTL (1 hour)

---

## Test 1: Basic Subagent Execution (Baseline)

**Purpose**: Verify foreground execution still works

**Steps**:
1. Start GenCode: `npm start`
2. In the GenCode CLI, type:
   ```
   "Use the Task tool to explore authentication patterns in this codebase"
   ```

**Expected Result**:
- Subagent executes synchronously
- Shows tool calls (Read, Grep, Glob)
- Returns summary of authentication patterns
- Session ID included in metadata

---

## Test 2: Background Execution

**Purpose**: Test Phase 2 background task feature

**Steps**:
1. In GenCode CLI:
   ```
   "Use the Task tool to explore all TypeScript files in src/ directory in the background"
   ```

**What to observe**:
1. Task returns immediately with task ID like `bg-explore-1234567890-abc`
2. Output shows file path: `~/.gen/tasks/bg-explore-*/output.log`
3. Main conversation continues (you can ask other questions)

**Verify files created** (in another terminal):
```bash
ls -la ~/.gen/tasks/

# Should see directory: bg-explore-*
ls -la ~/.gen/tasks/bg-explore-*/

# Should contain:
# - output.log (NDJSON events)
# - metadata.json (task status)
```

**Check task status**:
```
"Use TaskOutput to check the status of task bg-explore-1234567890-abc"
```

**Expected Output**:
```
Task ID: bg-explore-1234567890-abc
Status: running
Progress: 3/15 turns
Duration: 45 seconds
Type: Explore
```

**Get result (blocking)**:
```
"Use TaskOutput to get the result of task bg-explore-1234567890-abc with blocking enabled"
```

**Expected**: Waits until task completes, then shows final summary

---

## Test 3: Task Listing

**Purpose**: Verify TaskOutput list action

**Steps**:
1. Start 2-3 background tasks:
   ```
   "Use Task to explore authentication in background"
   "Use Task to find all test files in background"
   ```

2. List all tasks:
   ```
   "Use TaskOutput to list all tasks"
   ```

**Expected Output**:
```
Background Tasks (3 total):

ID: bg-explore-001
Status: completed
Duration: 2m 30s

ID: bg-explore-002
Status: running
Progress: 5/15

ID: bg-explore-003
Status: pending
```

**Filter by status**:
```
"Use TaskOutput to list only running tasks"
```

---

## Test 4: Task Cancellation

**Purpose**: Test cancel action

**Steps**:
1. Start a long-running background task:
   ```
   "Use Task to explore the entire src/ directory in background"
   ```

2. Get the task ID, then cancel it:
   ```
   "Use TaskOutput to cancel task bg-explore-1234567890-abc"
   ```

**Expected**: Task status changes to "cancelled"

**Verify**:
```bash
# Check metadata:
cat ~/.gen/tasks/bg-explore-*/metadata.json

# Should show: "status": "cancelled"
```

---

## Test 5: Session Resume

**Purpose**: Test Phase 3 resume capability

**Step 1: Create a session**
```
"Use Task to explore the database schema in src/"
```

**Important**: Save the session ID from the output (e.g., `subagent-1234567890-abc123`)

**Step 2: Exit and restart GenCode**
```bash
# Exit GenCode (Ctrl+C)
npm start
```

**Step 3: Resume the session**
```
"Use Task to resume session subagent-1234567890-abc123 and now find all migration files"
```

**Expected Result**:
- Subagent loads previous context
- Continues with new prompt about migrations
- Uses knowledge from previous exploration
- Resume count increments to 1

**Verify session metadata**:
```bash
cat ~/.gen/sessions/subagent-1234567890-abc123.json
```

**Should contain**:
```json
{
  "metadata": {
    "isSubagentSession": true,
    "subagentType": "Explore",
    "resumeCount": 1,
    "expiresAt": "2026-01-25T...",
    "lastResumedAt": "2026-01-18T..."
  }
}
```

---

## Test 6: Resume Validation

**Purpose**: Test session validation (expiry, quota)

### Test 6a: Non-existent session
```
"Use Task to resume session invalid-session-id and do something"
```

**Expected**: Error message "Resume failed: Session not found"

### Test 6b: Resume quota (optional - time intensive)
```bash
# Resume the same session 5 times
"Use Task to resume session subagent-XXX and find step 1"
"Use Task to resume session subagent-XXX and find step 2"
# ... repeat 5 times total

# 6th resume should fail:
"Use Task to resume session subagent-XXX and find step 6"
```

**Expected**: Error message "Resume quota exceeded (max 5)"

---

## Test 7: Parallel Execution

**Purpose**: Test Phase 4 parallel task execution

**Steps**:
```
"Use Task tool with parallel execution to:
1. Search for authentication patterns (Explore)
2. Find all test files (Explore)
3. Look for database migrations (Explore)"
```

**Expected Behavior**:
- All 3 subagents launch concurrently (Promise.all)
- Results aggregated when all complete
- Faster than sequential execution

**Expected Output Format**:
```
Parallel Task Results (3 tasks):

Task 1: Search authentication patterns
Status: ✓ Completed
Summary: Found 5 auth-related files...

Task 2: Find test files
Status: ✓ Completed
Summary: Found 23 test files...

Task 3: Database migrations
Status: ✓ Completed
Summary: Found 8 migration files...
```

---

## Test 8: Custom Agents - JSON Format

**Purpose**: Test custom agent loading (GenCode native format)

**Step 1: Install example agent**
```bash
# Copy example to agents directory:
mkdir -p ~/.gen/agents
cp examples/custom-agents/code-reviewer.json ~/.gen/agents/
```

**Step 2: Restart GenCode** (to load new agents)
```bash
npm start
```

**Step 3: Use custom agent**
```
"Use Task with subagent_type code-reviewer to review the authentication implementation in src/auth/"
```

**Expected Result**:
- Custom agent loads successfully
- Uses specified tools (Read, Grep, Glob, WebFetch)
- Follows system prompt (security focus, code quality)
- Reviews code and provides structured feedback

**Verify agent loaded**:
Check console for log message:
```
Loaded custom agent: code-reviewer (source: gencode)
```

---

## Test 9: Custom Agents - Markdown Format

**Purpose**: Test Claude Code compatibility

**Step 1: Install Markdown agent to Claude directory**
```bash
mkdir -p ~/.claude/agents
cp examples/custom-agents/test-architect.md ~/.claude/agents/
```

**Step 2: Restart GenCode**
```bash
npm start
```

**Step 3: Use custom agent**
```
"Use Task with subagent_type test-architect to analyze test coverage for the user service"
```

**Expected Result**:
- Agent loads from Claude Code directory
- Parses Markdown frontmatter correctly
- Uses tools from frontmatter
- Follows system prompt from markdown body

**Verify agent source**:
Check console for:
```
Loaded custom agent: test-architect (source: claude)
```

---

## Test 10: Agent Merge Mechanism

**Purpose**: Test GenCode priority over Claude Code

**Step 1: Create conflicting agents**
```bash
# Create agent in Claude directory:
cat > ~/.claude/agents/my-agent.md << 'EOF'
---
name: my-agent
type: custom
description: Claude Code version
allowedTools: ["Read"]
defaultModel: claude-haiku-4
maxTurns: 5
---

This is the Claude Code version (lower priority).
EOF

# Create agent in GenCode directory (same name):
cat > ~/.gen/agents/my-agent.json << 'EOF'
{
  "name": "my-agent",
  "type": "custom",
  "description": "GenCode version (should win)",
  "allowedTools": ["Read", "Write", "Edit"],
  "defaultModel": "claude-sonnet-4",
  "maxTurns": 10,
  "systemPrompt": "This is the GenCode version (higher priority)."
}
EOF
```

**Step 2: Restart GenCode**
```bash
npm start
```

**Step 3: Use the agent**
```
"Use Task with subagent_type my-agent to read a file"
```

**Expected Result**:
- GenCode version is used (more tools, different model)
- Console shows: `Loaded custom agent: my-agent (source: gencode)`
- Agent has access to Write and Edit tools (not just Read)

**Step 4: Test fallback**
```bash
# Delete GenCode version:
rm ~/.gen/agents/my-agent.json

# Restart GenCode:
npm start

# Use agent again:
"Use Task with subagent_type my-agent to read a file"
```

**Expected Result**:
- Falls back to Claude Code version
- Console shows: `Loaded custom agent: my-agent (source: claude)`
- Agent only has Read tool (limited)

---

## Test 11: Inter-Agent Communication (Depth Limit)

**Purpose**: Test nested subagent execution with depth limit

**Step 1: Create a meta-agent**
```bash
cat > ~/.gen/agents/meta-analyzer.json << 'EOF'
{
  "name": "meta-analyzer",
  "type": "custom",
  "description": "Analyzes code by spawning other subagents",
  "allowedTools": ["Task", "Read"],
  "defaultModel": "claude-sonnet-4",
  "maxTurns": 10,
  "systemPrompt": "You analyze code by delegating to specialized subagents. Use the Task tool to spawn Explore and Plan subagents for different aspects of analysis."
}
EOF
```

**Step 2: Test execution**
```bash
npm start
```

```
"Use Task with subagent_type meta-analyzer to analyze the entire codebase structure"
```

**Expected**:
- Meta-agent spawns child subagents (depth 1)
- Child subagents can spawn grandchild subagents (depth 2)
- At depth 3, further nesting is blocked

**Error at max depth**:
```
Max subagent depth (3) exceeded
```

---

## Test 12: Result Caching

**Purpose**: Test result cache to avoid redundant work

**Step 1: Run same query twice**
```
"Use Task Explore to find all authentication patterns"

# Wait for completion, then run EXACTLY the same query:
"Use Task Explore to find all authentication patterns"
```

**Expected**:
- Second query returns immediately from cache
- No actual subagent execution
- Output shows: "(cached result)"

**Verify cache directory**:
```bash
ls -la ~/.gen/cache/subagents/

# Should contain hash-named files:
# abc123def456...json
```

**Step 2: Test cache expiry**

Wait 1 hour (or modify TTL in code to 10 seconds for testing), then run same query again:

```
"Use Task Explore to find all authentication patterns"
```

**Expected**: Cache expired, runs fresh query

---

## Test 13: Cleanup and Limits

**Purpose**: Test resource management

### Test concurrent limit

Start 11 background tasks rapidly:
```
"Use Task to explore in background task 1"
"Use Task to explore in background task 2"
# ... repeat 11 times
```

**Expected**: 11th task should fail or queue with message:
```
Maximum concurrent tasks (10) reached
```

### Test cleanup

```bash
# Check tasks directory:
ls -la ~/.gen/tasks/

# Manually set a task's timestamp to >24 hours ago:
# Edit metadata.json: "startedAt": "2026-01-17T..."

# Trigger cleanup (may need to restart GenCode or wait for automatic cleanup)
```

**Expected**: Old tasks are deleted automatically

---

## Final Verification Checklist

After completing all tests, verify:

- [ ] Background tasks execute without blocking
- [ ] Task IDs are unique and descriptive
- [ ] NDJSON logs are created and readable
- [ ] TaskOutput actions all work (status, list, result, wait, cancel)
- [ ] Session resume preserves context
- [ ] Resume validation enforces expiry and quota
- [ ] Parallel execution runs concurrently
- [ ] JSON custom agents load from `~/.gen/agents/`
- [ ] Markdown custom agents load from `~/.claude/agents/`
- [ ] GenCode agents override Claude Code agents
- [ ] Deleting GenCode agent falls back to Claude Code version
- [ ] Agent source is tracked and reported correctly
- [ ] Depth limit prevents infinite recursion
- [ ] Result cache improves performance for duplicate queries
- [ ] Concurrent task limit is enforced
- [ ] Old tasks are cleaned up automatically

---

## Quick Smoke Test

For a quick end-to-end verification:

```bash
#!/bin/bash

# 1. Build
npm run build

# 2. Install example agents
mkdir -p ~/.gen/agents ~/.claude/agents
cp examples/custom-agents/code-reviewer.json ~/.gen/agents/
cp examples/custom-agents/test-architect.md ~/.claude/agents/

# 3. Start GenCode
npm start

# Then manually test in the CLI:
# - "Use Task to explore src/ in background"
# - "Use TaskOutput to list all tasks"
# - "Use Task code-reviewer to review src/auth/"
# - "Use Task test-architect to analyze test coverage"
```

---

## Troubleshooting

### Agent not loading
- Check console for error messages
- Verify JSON syntax is valid
- Ensure all required fields are present
- Check file permissions

### Background task not starting
- Check concurrent task limit (max 10)
- Verify `~/.gen/tasks/` directory exists and is writable
- Check console for error messages

### Session resume failing
- Verify session ID is correct
- Check session hasn't expired (7 days)
- Ensure resume quota not exceeded (5 resumes)
- Verify session file exists in `~/.gen/sessions/`

### Cache not working
- Check `~/.gen/cache/subagents/` directory exists
- Verify prompt is identical (hash-based matching)
- Check cache hasn't expired (1 hour TTL)

---

## Next Steps

After manual testing:
1. Document any issues found
2. Run automated tests: `npm test`
3. Review implementation summary: `IMPLEMENTATION_SUMMARY.md`
4. Check custom agent documentation: `docs/custom-agents.md`
