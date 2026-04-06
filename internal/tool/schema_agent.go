package tool

import "github.com/yanmxa/gencode/internal/provider"

// AgentToolSchema returns the schema for the Agent tool
var AgentToolSchema = provider.Tool{
	Name: "Agent",
	Description: `Launch a subagent to handle complex, multi-step tasks autonomously.

Check <available-agents> for available agent types. Use agent name as subagent_type. If omitted, the general-purpose agent is used.

When NOT to use Agent (use direct tools instead):
- If you want to read a specific file path, use the Read tool or the Glob tool instead
- If searching for code within a specific file or set of 2-3 files, use Read directly
- Other tasks that are simple enough for 1-2 direct tool calls

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Launch multiple agents concurrently whenever possible; to do that, use a single message with multiple Agent calls
- Each agent runs in isolated context — the result returned by the agent is not visible to the user. To show the user the result, you should send a text message back with a concise summary.
- Foreground (default): blocks until agent completes, use when you need results before proceeding
- Background (run_in_background=true): returns task_id, you will be automatically notified on completion — do NOT sleep, poll, or proactively check progress
- Provide clear, detailed prompts so the agent can work autonomously and return exactly the information you need
- Clearly tell the agent whether you expect it to write code or just to do research`,
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
				"enum":        []string{"acceptEdits", "bypassPermissions", "default", "dontAsk", "plan", "auto"},
			},
			"isolation": map[string]any{
				"type":        "string",
				"description": "Isolation mode for the agent.",
				"enum":        []string{"worktree"},
			},
			"team_name": map[string]any{
				"type":        "string",
				"description": "Team name for spawning. Uses current team context if omitted.",
			},
		},
		"required": []string{"description", "prompt"},
	},
}

// SkillToolSchema returns the schema for the Skill tool
var SkillToolSchema = provider.Tool{
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

// EnterPlanModeSchema returns the schema for EnterPlanMode tool
var EnterPlanModeSchema = provider.Tool{
	Name: "EnterPlanMode",
	Description: `Request to enter plan mode for tasks that need exploration before implementation. The user must approve entering plan mode.

When to use:
- Adding meaningful new features (e.g., new endpoints, form validation, auth flows)
- Multiple valid approaches exist and you need to explore first
- Changes affect existing behavior or code structure
- Architectural decisions are required
- Tasks will modify more than 2-3 files
- Requirements are unclear and need exploration

When NOT to use:
- User already gave clear, specific instructions (use TaskCreate to track steps instead)
- User provided a numbered list of tasks (just execute them)
- Pure research or exploration (use Agent with Explore instead)
- Simple fixes like typos, obvious bugs, or single functions with clear requirements
- Straightforward multi-file changes where the approach is obvious
- Answering questions about the codebase`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message": map[string]any{
				"type":        "string",
				"description": "Optional message explaining why plan mode is needed for this task.",
			},
		},
		"required": []string{},
	},
}

// ExitPlanModeSchema returns the schema for ExitPlanMode tool
var ExitPlanModeSchema = provider.Tool{
	Name:        "ExitPlanMode",
	Description: "Exit plan mode and submit your implementation plan for user approval. Call this when you have finished exploring the codebase and created a complete implementation plan.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan": map[string]any{
				"type":        "string",
				"description": "The complete implementation plan in Markdown format. Should include: Context, Implementation Steps (with file paths and line references), Critical Files, and Verification.",
			},
		},
		"required": []string{"plan"},
	},
}

// ToolSearchSchema defines the schema for the ToolSearch tool.
var ToolSearchSchema = provider.Tool{
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

// PlanModeAgentSchema is an Agent tool schema for plan mode.
// It omits run_in_background (foreground-only) and restricts to read-only agents.
var PlanModeAgentSchema = provider.Tool{
	Name: "Agent",
	Description: `Launch a subagent to research the codebase.

Available agent types in plan mode:
- Explore: Fast codebase exploration. Use to find files, search code, and answer questions. (Tools: Read, Glob, Grep, WebFetch, WebSearch)
- Plan: Software architect for designing implementation plans. (Tools: Read, Glob, Grep, WebFetch, WebSearch)

Usage notes:
- Launch multiple agents by making multiple Agent calls in a single message
- Always include a short description (3-5 word) summarizing what the agent will do
- Provide each agent with specific questions, not just "explore X"`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "The type of agent to spawn (Explore or Plan).",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "A short (3-5 word) description of the task",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Override model: sonnet, opus, haiku. If not specified, inherits from parent.",
				"enum":        []string{"sonnet", "opus", "haiku"},
			},
		},
		"required": []string{"subagent_type", "prompt"},
	},
}
