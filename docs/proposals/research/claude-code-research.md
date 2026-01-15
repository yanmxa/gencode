# Claude Code Research Summary

This document preserves research findings from analyzing Claude Code's implementation, used to inform mycode's enhancement proposals.

## Table of Contents

1. [Memory System Research](#1-memory-system-research)
2. [Context Management Research](#2-context-management-research)
3. [Session Management Research](#3-session-management-research)
4. [Cross-Tool Comparison](#4-cross-tool-comparison)
5. [References](#5-references)

---

## 1. Memory System Research

### CLAUDE.md File Hierarchy

Claude Code uses a **4-level hierarchical memory system** (lowest to highest priority):

| Level | Location | Purpose | Version Control |
|-------|----------|---------|-----------------|
| 1. User/Global | `~/.claude/CLAUDE.md` | Personal preferences across all projects | Personal, not shared |
| 2. Project | `./CLAUDE.md` or `./.claude/CLAUDE.md` | Team-shared project standards | Checked into git |
| 3. Rules | `./.claude/rules/*.md` | Modular rule organization | Checked into git |
| 4. Enterprise | Organization-defined location | Organization-wide policies | Managed by org |

### /init Command Internal Mechanism

The /init command uses a two-part system:

**1. Context Gathering Phase** - Uses BatchTool with GlobTool:
```json
{
  "type": "tool_use",
  "name": "BatchTool",
  "input": {
    "description": "Gather repository information",
    "invocations": [
      { "tool_name": "GlobTool", "input": { "pattern": "package*.json" } },
      { "tool_name": "GlobTool", "input": { "pattern": "*.md" } },
      { "tool_name": "GlobTool", "input": { "pattern": ".cursor/rules/**" } },
      { "tool_name": "GlobTool", "input": { "pattern": ".cursorrules/**" } },
      { "tool_name": "GlobTool", "input": { "pattern": ".github/copilot-instructions.md" } }
    ]
  }
}
```

**2. Generation Phase** - Uses this prompt (384 tokens):
```
Please analyze this codebase and create a CLAUDE.md file, which will be
given to future instances of Claude Code to operate in this repository.

Include:
1. Commands that will be commonly used, such as how to build, lint, and
   run tests. Include the necessary commands to develop in this codebase,
   such as how to run a single test.

2. High-level code architecture and structure so that future instances
   can be productive more quickly. Focus on the "big picture" architecture
   that requires reading multiple files to understand.

Guidelines:
- If there's already a CLAUDE.md, suggest improvements to it.
- Do NOT repeat obvious instructions like "Write unit tests for all new
  utilities" or "Never include sensitive information in code".
- Do NOT list every component or file structure that can be easily discovered.
- If there is a README.md, include the important parts.
- Do NOT make up sections like "Tips for Development" unless this is
  expressly included in other files.
- If there are Cursor rules (.cursor/rules/ or .cursorrules) or Copilot
  rules (.github/copilot-instructions.md), make sure to include them.

The file must begin with:
"This file provides guidance to Claude Code (claude.ai/code) when working
with code in this repository."
```

### Context Injection Method

**Critical Finding**: Memory content is NOT directly prepended to the system prompt.

Instead, Claude Code uses a specific pattern:
1. Memory content is wrapped in `<system-reminder>` tags
2. Placed as the **first user message** in the conversation (after system prompt)
3. Includes explicit instruction that content "may or may not be relevant"

```xml
<system-reminder>
As you answer the user's questions, you can use the following context:
# claudeMd
Codebase and user instructions are shown below. Be sure to adhere to these instructions.
IMPORTANT: These instructions OVERRIDE any default behavior and you MUST follow them exactly as written.

Contents of /path/to/CLAUDE.md (project instructions, checked into the codebase):

[CLAUDE.md content here]

IMPORTANT: this context may or may not be relevant to your tasks.
You should not respond to this context unless it is highly relevant to your task.
</system-reminder>
```

### Import Syntax Rules

- Syntax: `@path/to/file` at beginning of a line
- Supports relative paths, absolute paths, and `~` expansion
- Max import depth: 5 levels (prevents circular references)
- Imports are NOT evaluated inside markdown code blocks or inline code spans
- Imported files can recursively import other files

### Size Limits

| Metric | Recommended Limit |
|--------|-------------------|
| Total memory content | < 40,000 characters |
| Warning threshold | ~44.7k characters |
| Individual rule files | < 500 lines |

---

## 2. Context Management Research

### Memory Tool API (Beta)

Claude Code's Memory Tool (beta API `context-management-2025-06-27`) enables agents to store and retrieve information across conversations.

**Commands:**

| Command | Description | Input |
|---------|-------------|-------|
| `view` | Shows directory or file contents | `path`, optional `view_range` |
| `create` | Creates a new file | `path`, `file_text` |
| `str_replace` | Replaces text in file | `path`, `old_str`, `new_str` |
| `insert` | Inserts at line | `path`, `insert_line`, `insert_text` |
| `delete` | Deletes file/directory | `path` |
| `rename` | Renames/moves file | `old_path`, `new_path` |

**System Prompt Injection:**
```
IMPORTANT: ALWAYS VIEW YOUR MEMORY DIRECTORY BEFORE DOING ANYTHING ELSE.
MEMORY PROTOCOL:
1. Use the `view` command of your `memory` tool to check for earlier progress.
2. ... (work on the task) ...
   - As you make progress, record status / progress / thoughts etc in your memory.
ASSUME INTERRUPTION: Your context window might be reset at any moment, so you
risk losing any progress that is not recorded in your memory directory.
```

### Context Editing Configuration

```typescript
context_management: {
  edits: [
    {
      type: "clear_tool_uses_20250919",
      trigger: {
        type: "input_tokens",
        value: 100000  // Trigger at 100K tokens
      },
      keep: {
        type: "tool_uses",
        value: 3  // Keep last 3 tool results
      },
      exclude_tools: ["memory"]  // Never clear memory operations
    }
  ]
}
```

### Context Compaction SDK Configuration

```typescript
compaction_control: {
  enabled: true,
  context_token_threshold: 100000,  // Default: 100k tokens
  summary_prompt: "Custom summary instructions...",
  model: "claude-haiku-4-5"  // Use cheaper model for summarization
}
```

**Threshold Guidelines:**

| Threshold | Use Case |
|-----------|----------|
| Low (5k-20k) | Sequential entity processing with clear boundaries |
| Medium (50k-100k) | Multi-phase workflows with fewer checkpoints |
| High (100k-150k) | Tasks requiring substantial historical context |

### Token Cost Analysis

**Before (No Context Management):**

| Turn | Input Tokens | Cost (Claude) |
|------|--------------|---------------|
| 1 | 1,000 | $0.003 |
| 10 | 10,000 | $0.03 |
| 25 | 50,000 | $0.15 |
| 50 | 100,000 | $0.30 |
| 100 | 200,000 | Context Error |

**After (With Context Editing - 84% reduction):**

| Turn | Input Tokens | Notes |
|------|--------------|-------|
| 1 | 1,000 | |
| 10 | 10,000 | |
| 25 | 50,000 | |
| 50 | 30,000 | Cleared at turn 40 |
| 100 | 40,000 | Cleared at turn 80 |

---

## 3. Session Management Research

### Session Resume Options

```bash
claude -c              # Continue most recent conversation
claude -r "abc123"     # Resume specific session by ID
claude --resume        # Interactive conversation picker
```

### Fork Behavior

| Behavior | `forkSession: false` (default) | `forkSession: true` |
|----------|-------------------------------|---------------------|
| **Session ID** | Same as original | New session ID generated |
| **History** | Appends to original | Creates new branch from resume point |
| **Original Session** | Modified | Preserved unchanged |

### Permission Persistence

File creation permissions, previously approved commands, and MCP servers enabled in the original session remain active when resuming.

**Key Insight**: Permission decisions are session-scoped, not conversation-turn-scoped.

### Todo Persistence

Claude has access to a persistent todo list that survives:
- Context compaction (included in summary)
- Session resume
- Multiple compaction cycles

### Known Limitations

From GitHub issue discussions:

1. **Context Loss on Crash**: Unlike web chat, CLI sessions can lose context on interruption
2. **Summary Quality Degradation**: Multiple compactions can cause "off the rails" behavior
3. **Missing Debug Information**: Auto-compact summaries may omit critical debugging details

---

## 4. Cross-Tool Comparison

### Memory/Rules Comparison

| Feature | Claude Code | Cursor | GitHub Copilot | mycode (Target) |
|---------|-------------|--------|----------------|-----------------|
| **Config File** | `CLAUDE.md` | `.cursorrules` | `.github/copilot-instructions.md` | `MYCODE.md` / `CLAUDE.md` |
| **User Global** | `~/.claude/CLAUDE.md` | `~/.cursor/rules/` | N/A | `~/.mycode/MYCODE.md` |
| **Rules Directory** | `.claude/rules/` | `.cursor/rules/` | `.github/instructions/` | `.mycode/rules/` |
| **Path Scoping** | `paths:` frontmatter | `globs:` frontmatter | `applyTo:` frontmatter | Add `paths:` |
| **Quick Add** | `#` prefix | N/A | N/A | Add `#` prefix |
| **AI Init** | `/init` with LLM | N/A | N/A | Add `--ai` flag |
| **Import Syntax** | `@path/to/file` | N/A | N/A | Add `@path` |

### Context Compaction Comparison

| Tool | Trigger | Mechanism | Distinct Feature |
|------|---------|-----------|------------------|
| **Claude Code** | Manual `/compact` or auto at ~95% | LLM summary, fresh start | Custom instructions support |
| **OpenAI Codex CLI** | Token threshold (180k-244k) | Summary + recent messages (~20k tokens) | Preserves recent messages alongside summary |
| **OpenCode (SST)** | Auto on overflow | Summary marked as assistant message | Separate pruning of old tool outputs |
| **Amp (Sourcegraph)** | Manual only | Fork, handoff, thread reference | Philosophy: "keep conversations short" |

**Best Practice**: Trigger at 85-90% (earlier than Claude Code's 95%) with user warnings and disable options.

---

## 5. References

### Official Documentation
- [Claude Code Memory Documentation](https://code.claude.com/docs/en/memory)
- [Session Management - Claude Docs](https://platform.claude.com/docs/en/agent-sdk/sessions)
- [Automatic Context Compaction - Claude Cookbook](https://platform.claude.com/cookbook/tool-use-automatic-context-compaction)
- [Memory Tool Documentation](https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool)

### Engineering Blog Posts
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
- [Context Engineering for AI Agents](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
- [Using CLAUDE.MD Files](https://claude.com/blog/using-claude-md-files)

### Community Resources
- [Build your own /init command](https://kau.sh/blog/build-ai-init-command/)
- [Reverse engineering Claude Code](https://kirshatrov.com/posts/claude-code-internals)
- [The Complete Guide to CLAUDE.md](https://www.builder.io/blog/claude-md-guide)
- [Claude Code System Prompts Repository](https://github.com/Piebald-AI/claude-code-system-prompts)
- [Context Compaction Research](https://gist.github.com/badlogic/cd2ef65b0697c4dbe2d13fbecb0a0a5f)

### GitHub Issues
- [Local Session History and Context Persistence #12646](https://github.com/anthropics/claude-code/issues/12646)
- [Persistent Memory Between Sessions #14227](https://github.com/anthropics/claude-code/issues/14227)
- [Pre-Compact Auto-Save and Improved Summarization #13239](https://github.com/anthropics/claude-code/issues/13239)
- [Path Resolution Issues #4754](https://github.com/anthropics/claude-code/issues/4754)
