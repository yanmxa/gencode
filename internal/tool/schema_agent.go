package tool

import "github.com/yanmxa/gencode/internal/core"

// agentToolSchema is the schema for the Agent tool.
var agentToolSchema = core.ToolSchema{
	Name: "Agent",
	Description: `Launch a new agent to handle complex, multi-step tasks. Each agent type has specific capabilities and tools available to it.

Check <available-agents> for available agent types and their when-to-use guidance. Use agent name as subagent_type. If omitted, the general-purpose agent is used.

When NOT to use Agent (use direct tools instead):
- If you want to read a specific file path, use the Read tool or the Glob tool instead
- If searching for code within a specific file or set of 2-3 files, use Read directly
- For simple, directed codebase searches (e.g. for a specific file/class/function) use Glob or Grep directly
- Other tasks that are simple enough for 1-2 direct tool calls

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Launch multiple agents concurrently whenever possible; to do that, use a single message with multiple Agent calls
- Each agent runs in isolated context — the result returned by the agent is not visible to the user. To show the user the result, you should send a text message back with a concise summary.
- **Foreground vs background**: Use foreground (default) when you need the agent's results before you can proceed — e.g., research agents whose findings inform your next steps. Use background when you have genuinely independent work to do in parallel.
- You can optionally run agents in the background using the run_in_background parameter. When an agent runs in the background, you will be automatically notified when it completes — do NOT sleep, poll, or proactively check on its progress. Continue with other work or respond to the user instead.
- Provide clear, detailed prompts — brief the agent like a smart colleague who just walked into the room. It hasn't seen this conversation, doesn't know what you've tried.
- Clearly tell the agent whether you expect it to write code or just to do research (search, file reads, web fetches, etc.), since it is not aware of the user's intent
- Never delegate understanding. Don't write "based on your findings, fix the bug." Include file paths, line numbers, what specifically to change.`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "A short (3-5 word) description of the task",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "The type of specialized agent to use for this task",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Name for the spawned agent",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Set to true to run this agent in the background. You will be notified when it completes.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override. If omitted, inherits from parent conversation.",
				"enum":        []string{"sonnet", "opus", "haiku"},
			},
			"max_turns": map[string]any{
				"type":        "number",
				"description": "Maximum number of conversation turns for the agent.",
			},
			"resume": map[string]any{
				"type":        "string",
				"description": "Agent ID to resume from a previous invocation.",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "Permission mode for spawned agent.",
				"enum":        []string{"acceptEdits", "bypassPermissions", "default", "dontAsk", "auto"},
			},
			"isolation": map[string]any{
				"type":        "string",
				"description": "Isolation mode for the agent.",
				"enum":        []string{"worktree"},
			},
			"fork": map[string]any{
				"type":        "boolean",
				"description": "If true, the agent inherits the parent conversation context. Use when the agent needs to understand what has been discussed so far. Cannot be combined with resume.",
			},
			"team_name": map[string]any{
				"type":        "string",
				"description": "Team name for spawning. Uses current team context if omitted.",
			},
		},
		"required": []string{"description", "prompt"},
	},
}

var continueAgentToolSchema = core.ToolSchema{
	Name: "ContinueAgent",
	Description: `Continue a previously spawned subagent using its saved conversation state.

Use this when a completed or background worker should keep going with follow-up instructions instead of spawning a fresh worker.

Usage notes:
- Prefer task_id when continuing a background worker that was started earlier in this conversation
- Use agent_id only when you already have a resumable agent/session ID
- When using agent_id directly, also provide subagent_type so the runtime can restore the correct agent configuration
- run_in_background=true starts a new background continuation and returns immediately; you will be automatically notified when it completes — do not poll or check progress
- Foreground continuations block until the worker finishes the new instruction`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Background task ID for the worker you want to continue. Preferred when available.",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Resumable agent/session ID to continue directly.",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "Agent type to use for the continuation. Required when using agent_id directly.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The follow-up instruction for the existing worker.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "A short (3-5 word) description of the continuation task.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Optional display name override for the continued agent.",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Set to true to continue the worker in the background. You will be notified when it completes.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override. If omitted, inherits from parent conversation.",
				"enum":        []string{"sonnet", "opus", "haiku"},
			},
			"max_turns": map[string]any{
				"type":        "number",
				"description": "Maximum number of conversation turns for the continuation.",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "Permission mode for the continued agent.",
				"enum":        []string{"acceptEdits", "bypassPermissions", "default", "dontAsk", "auto"},
			},
			"isolation": map[string]any{
				"type":        "string",
				"description": "Isolation mode for the continued agent.",
				"enum":        []string{"worktree"},
			},
		},
		"required": []string{"prompt"},
		"anyOf": []map[string]any{
			{"required": []string{"task_id"}},
			{"required": []string{"agent_id", "subagent_type"}},
		},
	},
}

var sendMessageToolSchema = core.ToolSchema{
	Name: "SendMessage",
	Description: `Send a follow-up message to an existing subagent worker.

Use this when you want to keep steering the same worker instead of spawning a fresh one.

Current runtime behavior:
- Completed or resumable workers: supported
- Currently running workers: supported; the message is queued and delivered at the worker's next safe turn boundary without interrupting the active turn

Usage notes:
- Prefer task_id when you have a background worker from this conversation
- Use agent_id when you already know the resumable session/agent ID
- When using agent_id directly, also provide subagent_type so the correct agent configuration can be restored
- run_in_background=true resumes the worker asynchronously and returns immediately; you will be automatically notified when it completes — do not poll or check progress`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Background task ID for the worker you want to core. Preferred when available.",
			},
			"agent_id": map[string]any{
				"type":        "string",
				"description": "Resumable agent/session ID to continue directly.",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "Agent type to use when resuming by agent_id directly.",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The follow-up message to send to the worker.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "A short (3-5 word) description of what this follow-up asks the worker to do.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Optional display name override for the continued worker.",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Set to true to continue the worker in the background. You will be notified when it completes.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Optional model override. If omitted, inherits from parent conversation.",
				"enum":        []string{"sonnet", "opus", "haiku"},
			},
			"max_turns": map[string]any{
				"type":        "number",
				"description": "Maximum number of conversation turns for the resumed run.",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "Permission mode for the resumed worker.",
				"enum":        []string{"acceptEdits", "bypassPermissions", "default", "dontAsk", "auto"},
			},
			"isolation": map[string]any{
				"type":        "string",
				"description": "Isolation mode for the resumed worker.",
				"enum":        []string{"worktree"},
			},
		},
		"required": []string{"message"},
		"anyOf": []map[string]any{
			{"required": []string{"task_id"}},
			{"required": []string{"agent_id", "subagent_type"}},
		},
	},
}

// skillToolSchema is the schema for the Skill tool.
var skillToolSchema = core.ToolSchema{
	Name: "Skill",
	Description: `Execute a skill within the main conversation.

When users ask to perform tasks, check if available skills can help.
Skills provide specialized capabilities and domain knowledge.

When users reference "/<skill-name>" (e.g., "/commit", "/review-pr"), use this tool to invoke it.

Example:
  User: "run /commit"
  Assistant: [Calls Skill tool with skill: "commit"]

How to invoke:
- skill: "pdf" - invoke the pdf skill
- skill: "commit", args: "-m 'Fix bug'" - invoke with arguments
- skill: "git:pr" - invoke using namespace:name format

Important:
- Available skills are listed in system-reminder messages in the conversation
- When a skill matches the user's request, this is a BLOCKING REQUIREMENT: invoke the relevant Skill tool BEFORE generating any other response about the task
- NEVER mention a skill without actually calling this tool
- Do not invoke a skill that is already running
- Do not use this tool for built-in CLI commands (like /help, /clear, etc.)
- If you see a <command-name> tag in the current conversation turn, the skill has ALREADY been loaded - follow the instructions directly instead of calling this tool again`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill": map[string]any{
				"type":        "string",
				"description": "The skill name (e.g., 'commit', 'git:pr', 'pdf')",
			},
			"args": map[string]any{
				"type":        "string",
				"description": "Optional arguments for the skill",
			},
		},
		"required": []string{"skill"},
	},
}

// toolSearchSchema defines the schema for the ToolSearch tool.
var toolSearchSchema = core.ToolSchema{
	Name: "ToolSearch",
	Description: `Fetches full schema definitions for deferred tools so they can be called.

Deferred tools appear by name in <available-deferred-tools> messages. Until fetched, only the name is known — there is no parameter schema, so the tool cannot be invoked. This tool takes a query, matches it against the deferred tool list, and returns the matched tools' complete JSONSchema definitions inside a <functions> block. Once a tool's schema appears in that result, it is callable exactly like any tool defined at the top of the prompt.

Result format: each matched tool appears as one <function>{"description": "...", "name": "...", "parameters": {...}}</function> line inside the <functions> block.

Query forms:
- "select:CronCreate,CronDelete" — fetch these exact tools by name
- "cron schedule" — keyword search, up to max_results best matches
- "+worktree" — require "worktree" in the name, rank by remaining terms`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": `Query to find deferred tools. Use "select:<tool_name>" for direct selection, or keywords to search.`,
			},
			"max_results": map[string]any{
				"type":        "number",
				"description": "Maximum number of results to return (default: 5)",
				"default":     5,
			},
		},
		"required": []string{"query"},
	},
}
