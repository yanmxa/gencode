package tool

import (
	"github.com/yanmxa/gencode/internal/provider"
)

// Tool name constants used in runtime comparisons across the codebase.
const (
	ToolAgent      = "Agent"
	ToolTaskOutput = "TaskOutput"
	ToolTaskStop   = "TaskStop"

	// Deprecated aliases — kept for backward compatibility with cached model contexts.
	ToolAgentOutput = ToolTaskOutput
	ToolAgentStop   = ToolTaskStop
	ToolSkill         = "Skill"
	ToolEnterPlanMode = "EnterPlanMode"
	ToolExitPlanMode  = "ExitPlanMode"
	ToolTaskCreate    = "TaskCreate"
	ToolTaskGet       = "TaskGet"
	ToolTaskUpdate    = "TaskUpdate"
	ToolTaskList      = "TaskList"
	ToolCronCreate      = "CronCreate"
	ToolCronDelete      = "CronDelete"
	ToolCronList        = "CronList"
	ToolEnterWorktree   = "EnterWorktree"
	ToolExitWorktree    = "ExitWorktree"
	ToolToolSearch      = "ToolSearch"
)

// ToolSchema defines the JSON schema for a tool
type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// GetToolSchemas returns provider.Tool definitions for all registered tools
func GetToolSchemas() []provider.Tool {
	return GetToolSchemasWithMCP(nil)
}

// GetToolSchemasWithMCP returns tool schemas including MCP tools if a getter is provided
func GetToolSchemasWithMCP(mcpToolsGetter func() []provider.Tool) []provider.Tool {
	tools := []provider.Tool{
		{
			Name: "Read",
			Description: `Reads a file from the local filesystem. You can access any file directly by using this tool.
Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.

Usage:
- The file_path parameter must be an absolute path, not a relative path
- By default, it reads up to 2000 lines starting from the beginning of the file
- You can optionally specify a line offset and limit (especially handy for long files), but it's recommended to read the whole file by not providing these parameters
- Results are returned with line numbers starting at 1
- This tool can only read files, not directories. To read a directory, use an ls command via the Bash tool.
- You will regularly be asked to read screenshots. If the user provides a path to a screenshot, ALWAYS use this tool to view the file at the path.
- If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "The absolute path to the file to read",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "The line number to start reading from (1-based). Only provide if the file is too large to read at once.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "The number of lines to read. Only provide if the file is too large to read at once.",
					},
				},
				"required": []string{"file_path"},
			},
		},
		{
			Name: "Glob",
			Description: `Fast file pattern matching tool that works with any codebase size.
- Supports glob patterns like "**/*.go" or "src/**/*.ts"
- Returns matching file paths sorted by modification time (newest first)
- Use this tool when you need to find files by name patterns
- When you are doing an open-ended search that may require multiple rounds of globbing and grepping, use the Agent tool instead`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "The glob pattern to match files against",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "The directory to search in. Defaults to current working directory.",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name: "Grep",
			Description: `A search tool built on ripgrep for searching file contents.
- ALWAYS use Grep for content search tasks. NEVER invoke grep or rg as a Bash command.
- Supports full regex syntax (e.g., "log.*Error", "function\\s+\\w+")
- Filter files with include parameter (e.g., "*.go", "*.py")
- Returns matching lines with file paths and line numbers
- Use Agent tool for open-ended searches requiring multiple rounds`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "The regular expression pattern to search for in file contents",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "File or directory to search in. Defaults to current working directory.",
					},
					"include": map[string]any{
						"type":        "string",
						"description": "File pattern to include (e.g., '*.go', '*.py')",
					},
					"case_sensitive": map[string]any{
						"type":        "boolean",
						"description": "If true, search is case-sensitive. Default is false (case-insensitive).",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name: "WebFetch",
			Description: `Fetches content from a specified URL and converts HTML to Markdown for readability.

Usage notes:
- The URL must be a fully-formed valid URL
- HTTP URLs will be automatically upgraded to HTTPS
- This tool is read-only and does not modify any files
- Results may be truncated if the content is very large
- For GitHub URLs, prefer using the gh CLI via Bash instead (e.g., gh pr view, gh issue view, gh api)`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{
						"type":        "string",
						"description": "The URL to fetch content from",
					},
					"format": map[string]any{
						"type":        "string",
						"description": "Output format: 'markdown' (default) or 'raw'",
					},
				},
				"required": []string{"url"},
			},
		},
		{
			Name: "WebSearch",
			Description: `Search the web for up-to-date information. Returns a list of relevant results with titles, URLs, and snippets.
When searching for current information, always use the present year rather than previous years.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query",
					},
					"num_results": map[string]any{
						"type":        "integer",
						"description": "Number of results to return (default: 10)",
					},
					"allowed_domains": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Only include results from these domains",
					},
					"blocked_domains": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Exclude results from these domains",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name: "Edit",
			Description: `Performs exact string replacements in files.

Usage:
- You must use your Read tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file.
- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. Never include any part of the line number prefix in the old_string or new_string.
- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.
- The edit will FAIL if old_string is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use replace_all to change every instance of old_string.
- Use replace_all for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "The absolute path to the file to modify",
					},
					"old_string": map[string]any{
						"type":        "string",
						"description": "The text to replace (must be different from new_string)",
					},
					"new_string": map[string]any{
						"type":        "string",
						"description": "The text to replace it with (must be different from old_string)",
					},
					"replace_all": map[string]any{
						"type":        "boolean",
						"description": "Replace all occurrences of old_string (default false)",
						"default":     false,
					},
				},
				"required": []string{"file_path", "old_string", "new_string"},
			},
		},
		{
			Name: "Write",
			Description: `Writes a file to the local filesystem.

Usage:
- This tool will overwrite the existing file if there is one at the provided path.
- If this is an existing file, you MUST use the Read tool first to read the file's contents. This tool will fail if you did not read the file first.
- Prefer the Edit tool for modifying existing files — it only sends the diff. Only use this tool to create new files or for complete rewrites.
- NEVER create documentation files (*.md) or README files unless explicitly requested by the User.
- Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "The absolute path to the file to write (must be absolute, not relative)",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "The content to write to the file",
					},
				},
				"required": []string{"file_path", "content"},
			},
		},
		{
			Name: "Bash",
			Description: `Executes a given bash command and returns its output.

The working directory persists between commands, but shell state does not.

IMPORTANT: Avoid using this tool to run grep, find, cat, head, tail, sed, awk, or echo commands. Instead, use the appropriate dedicated tool:
- File search: Use Glob (NOT find or ls)
- Content search: Use Grep (NOT grep or rg)
- Read files: Use Read (NOT cat/head/tail)
- Edit files: Use Edit (NOT sed/awk)
- Write files: Use Write (NOT echo/cat with redirection)

You may specify an optional timeout in milliseconds (up to 600000ms / 10 minutes). By default, your command will timeout after 120000ms (2 minutes).
You can use the run_in_background parameter to run the command in the background. You will be notified when it finishes.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The command to execute",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Clear, concise description of what this command does in active voice",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Optional timeout in milliseconds (max 600000)",
					},
					"run_in_background": map[string]any{
						"type":        "boolean",
						"description": "Set to true to run this command in the background. You will be notified when it completes.",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			Name:        "TaskOutput",
			Description: "Retrieve output from a running or completed background task. Use this to check on background agents started with Agent run_in_background=true.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "string",
						"description": "The ID of the background task to get output from",
					},
					"block": map[string]any{
						"type":        "boolean",
						"description": "If true (default), wait for task completion. If false, return current output immediately.",
						"default":     true,
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Maximum time to wait in milliseconds when block=true (default: 30000, max: 600000)",
						"default":     30000,
					},
				},
				"required": []string{"task_id"},
			},
		},
		{
			Name:        "TaskStop",
			Description: "Stops a running background task by its ID. Returns a success or failure status. Use this tool when you need to terminate a long-running background agent or command.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"task_id": map[string]any{
						"type":        "string",
						"description": "The ID of the background task to stop",
					},
				},
				"required": []string{"task_id"},
			},
		},
		{
			Name: "AskUserQuestion",
			Description: `Ask the user questions to gather preferences, clarify requirements, or get decisions on implementation choices. Use when you need user input to proceed.

Plan mode note: In plan mode, use this tool to clarify requirements or choose between approaches BEFORE finalizing your plan. Do NOT use this tool to ask "Is my plan ready?" or "Should I proceed?" — use ExitPlanMode for plan approval. Do not reference "the plan" in your questions because the user cannot see the plan until you call ExitPlanMode.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"questions": map[string]any{
						"type":        "array",
						"description": "Questions to ask the user (1-4 questions)",
						"minItems":    1,
						"maxItems":    4,
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"question": map[string]any{
									"type":        "string",
									"description": "The complete question to ask the user",
								},
								"header": map[string]any{
									"type":        "string",
									"maxLength":   12,
									"description": "Very short label displayed as a chip/tag (max 12 chars)",
								},
								"options": map[string]any{
									"type":        "array",
									"description": "The available choices (2-4 options). 'Other' option is added automatically.",
									"minItems":    2,
									"maxItems":    4,
									"items": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"label": map[string]any{
												"type":        "string",
												"description": "The display text for this option (1-5 words)",
											},
											"description": map[string]any{
												"type":        "string",
												"description": "Explanation of what this option means",
											},
										},
										"required": []string{"label", "description"},
									},
								},
								"multiSelect": map[string]any{
									"type":        "boolean",
									"default":     false,
									"description": "Set to true to allow multiple options to be selected",
								},
							},
							"required": []string{"question", "header", "options", "multiSelect"},
						},
					},
				},
				"required": []string{"questions"},
			},
		},
	}

	// Add EnterPlanMode to normal mode tools
	tools = append(tools, EnterPlanModeSchema)

	// Add Skill tool
	tools = append(tools, SkillToolSchema)

	// Add Agent tool
	tools = append(tools, AgentToolSchema)

	// Add ToolSearch (always available — enables progressive disclosure)
	tools = append(tools, ToolSearchSchema)

	// Add Todo tools
	tools = append(tools, TodoToolSchemas...)

	// Add Cron tools
	tools = append(tools, CronToolSchemas...)

	// Add Worktree tools
	tools = append(tools, WorktreeToolSchemas...)

	// Add MCP tools if getter is provided
	if mcpToolsGetter != nil {
		tools = append(tools, mcpToolsGetter()...)
	}

	return tools
}

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

// TodoToolSchemas defines the schemas for task management tools
var TodoToolSchemas = []provider.Tool{
	{
		Name: "TaskCreate",
		Description: `Create a task to track progress on multi-step work.

When to use:
- Complex tasks requiring 3+ distinct steps
- User provides multiple tasks at once
- After receiving new instructions — capture requirements as tasks

When NOT to use:
- Single straightforward task or trivial fix
- Purely conversational or informational exchange

Granularity: one task per logical deliverable (a file, a feature, a test suite).
Don't create tasks for sub-steps within a single file or for "planning"/"summarizing".

Tips:
- Prefer sending ALL TaskCreate calls in a single message (parallel tool calls) for speed
- Use imperative subjects ("Fix bug", "Add tests")
- Provide activeForm for spinner display ("Fixing bug", "Adding tests")
- Check TaskList first to avoid duplicates
- Task IDs are sequential integers starting from 1. Use addBlockedBy to set dependencies (e.g. addBlockedBy=["1"])`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject": map[string]any{
					"type":        "string",
					"description": "A brief, actionable title in imperative form (e.g., 'Fix authentication bug')",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Detailed description of what needs to be done, including context and acceptance criteria",
				},
				"activeForm": map[string]any{
					"type":        "string",
					"description": "Present continuous form shown in spinner when in_progress (e.g., 'Fixing authentication bug'). If omitted, the spinner shows the subject instead.",
				},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Arbitrary metadata to attach to the task",
				},
				"addBlockedBy": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Task IDs that must complete before this task can start",
				},
			},
			"required": []string{"subject", "description"},
		},
	},
	{
		Name: "TaskGet",
		Description: `Retrieve full task details by ID (description, status, dependencies).

When to use:
- Before starting work on a task — get full requirements
- To check task dependencies (what it blocks, what blocks it)

Tip: Verify blockedBy is empty before beginning work.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"taskId": map[string]any{
					"type":        "string",
					"description": "The ID of the task to retrieve",
				},
			},
			"required": []string{"taskId"},
		},
	},
	{
		Name: "TaskUpdate",
		Description: `Update a task's status, details, or dependencies.

Status: pending → in_progress → completed. Use "deleted" to remove.
- Set in_progress BEFORE starting work
- ONLY mark completed when FULLY done (not if tests fail or partial)
- After completing, call TaskList for next task
- If blocked, keep as in_progress and create a new task for the blocker
- When you see a <task-reminder>, review and update stale tasks immediately`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"taskId": map[string]any{
					"type":        "string",
					"description": "The ID of the task to update",
				},
				"status": map[string]any{
					"type":        "string",
					"description": "New status: pending, in_progress, completed, or deleted",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "New subject for the task",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "New description for the task",
				},
				"activeForm": map[string]any{
					"type":        "string",
					"description": "Present continuous form shown in spinner when in_progress (e.g., 'Fixing authentication bug')",
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "New owner for the task (agent name)",
				},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Metadata keys to merge into the task (set a key to null to delete it)",
				},
				"addBlocks": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Task IDs that this task blocks",
				},
				"addBlockedBy": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Task IDs that block this task",
				},
			},
			"required": []string{"taskId"},
		},
	},
	{
		Name: "TaskList",
		Description: `List all tasks with their status and dependencies.

When to use:
- Check overall progress
- After completing a task — find next available work
- Find blocked tasks that need dependencies resolved

Returns summary per task: id, status, owner. Use TaskGet for full details.
Prefer working on tasks in ID order (lowest first).`,
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
}


// CronToolSchemas defines the schemas for cron/scheduler tools.
var CronToolSchemas = []provider.Tool{
	{
		Name: "CronCreate",
		Description: `Schedule a prompt on a cron schedule. Uses standard 5-field cron: minute hour day-of-month month day-of-week.
Recurring jobs (default) auto-expire after 7 days. One-shot jobs (recurring=false) fire once then auto-delete.
Jobs only fire while the REPL is idle. Returns a job ID for CronDelete.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cron": map[string]any{
					"type":        "string",
					"description": "5-field cron expression in local time (e.g., '*/5 * * * *', '0 9 * * 1-5')",
				},
				"prompt": map[string]any{
					"type":        "string",
					"description": "The prompt to enqueue at each fire time",
				},
				"recurring": map[string]any{
					"type":        "boolean",
					"description": "true (default) = fire repeatedly. false = fire once then auto-delete.",
				},
				"durable": map[string]any{
					"type":        "boolean",
					"description": "If true, job persists across sessions (saved to ~/.gen/scheduled_tasks.json). Default: false (session-only).",
				},
			},
			"required": []string{"cron", "prompt"},
		},
	},
	{
		Name:        "CronDelete",
		Description: "Cancel a scheduled cron job by its ID.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "The job ID returned by CronCreate",
				},
			},
			"required": []string{"id"},
		},
	},
	{
		Name:        "CronList",
		Description: "List all scheduled cron jobs with their status, next fire time, and prompt.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
}

// WorktreeToolSchemas defines the schemas for git worktree tools.
var WorktreeToolSchemas = []provider.Tool{
	{
		Name: "EnterWorktree",
		Description: `Switch the current conversation into a git worktree for safe experimentation.
Creates an isolated copy of the repository where you can make changes without affecting the main working tree.
Use ExitWorktree to return to the original directory when done.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Optional slug for the worktree directory name (letters, digits, dots, underscores, dashes; max 64 chars)",
				},
			},
		},
	},
	{
		Name: "ExitWorktree",
		Description: `Exit the current worktree session and return to the original working directory.
Use action "keep" to preserve the worktree for later, or "remove" (default) to clean it up.
If removing with uncommitted changes, set discard_changes=true.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "What to do with the worktree: 'keep' (preserve for later) or 'remove' (clean up). Default: 'remove'.",
					"enum":        []string{"keep", "remove"},
				},
				"discard_changes": map[string]any{
					"type":        "boolean",
					"description": "If true, discard uncommitted changes when removing. Required when action='remove' and changes exist.",
				},
			},
		},
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

// GetToolSchemasFiltered returns tool schemas excluding disabled tools
func GetToolSchemasFiltered(disabled map[string]bool) []provider.Tool {
	all := GetToolSchemas()
	if len(disabled) == 0 {
		return all
	}
	filtered := make([]provider.Tool, 0, len(all))
	for _, t := range all {
		if !disabled[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// PlanModeAgentSchema is an Agent tool schema for plan mode.
// It omits run_in_background (foreground-only) and restricts to read-only agents.
var PlanModeAgentSchema = provider.Tool{
	Name: "Agent",
	Description: `Launch a subagent to research the codebase.

Available agent types in plan mode:
- Explore: Fast codebase exploration. Use to find files, search code, and answer questions. (Tools: Read, Glob, Grep, Bash, WebFetch, WebSearch)
- Plan: Software architect for designing implementation plans. (Tools: Read, Glob, Grep, Bash, WebFetch, WebSearch)

Usage notes:
- Launch multiple agents by making multiple Agent calls in a single message
- Always include a short description (3-5 words) summarizing what the agent will do
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

// GetPlanModeToolSchemas returns only the tools available in plan mode
// Plan mode restricts to read-only tools, the plan-specific Task tool, plus ExitPlanMode
func GetPlanModeToolSchemas() []provider.Tool {
	// Read-only tools allowed in plan mode
	allowedTools := map[string]bool{
		"Read":      true,
		"Glob":      true,
		"Grep":      true,
		"WebFetch":  true,
		"WebSearch": true,
	}

	// Filter to allowed read-only tools
	allTools := GetToolSchemas()
	tools := make([]provider.Tool, 0, len(allowedTools)+2)

	for _, t := range allTools {
		if allowedTools[t.Name] {
			tools = append(tools, t)
		}
	}

	// Add plan-mode Agent schema (no run_in_background, restricted agent types)
	tools = append(tools, PlanModeAgentSchema)

	// Add ExitPlanMode
	tools = append(tools, ExitPlanModeSchema)

	return tools
}

// GetPlanModeToolSchemasFiltered returns plan mode tools excluding disabled tools
func GetPlanModeToolSchemasFiltered(disabled map[string]bool) []provider.Tool {
	all := GetPlanModeToolSchemas()
	if len(disabled) == 0 {
		return all
	}
	filtered := make([]provider.Tool, 0, len(all))
	for _, t := range all {
		if !disabled[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
