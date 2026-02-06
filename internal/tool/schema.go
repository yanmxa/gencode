package tool

import (
	"github.com/yanmxa/gencode/internal/provider"
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
			Name:        "Read",
			Description: "Read file contents. Use this to read source code, configuration files, or any text file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "The path to the file to read (absolute or relative to current directory)",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "Line number to start reading from (1-based). Default is 1.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of lines to read. Default is 2000.",
					},
				},
				"required": []string{"file_path"},
			},
		},
		{
			Name:        "Glob",
			Description: "Find files matching a glob pattern. Supports ** for recursive matching. Results are sorted by modification time (newest first).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Glob pattern to match files (e.g., '**/*.go', 'src/**/*.ts')",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Base directory to search in. Default is current directory.",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "Grep",
			Description: "Search for patterns in files using regular expressions. Returns matching lines with file paths and line numbers.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Regular expression pattern to search for",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "File or directory to search in. Default is current directory.",
					},
					"include": map[string]any{
						"type":        "string",
						"description": "File pattern to include (e.g., '*.go', '*.py')",
					},
				},
				"required": []string{"pattern"},
			},
		},
		{
			Name:        "WebFetch",
			Description: "Fetch content from a URL. Converts HTML to Markdown for better readability.",
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
			Name:        "WebSearch",
			Description: "Search the web for up-to-date information. Returns a list of relevant results with titles, URLs, and snippets.",
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
			Name:        "Edit",
			Description: "Edit file contents using string replacement. The old_string must be unique in the file unless replace_all is true.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "The path to the file to edit (absolute or relative to current directory)",
					},
					"old_string": map[string]any{
						"type":        "string",
						"description": "The text to replace. Must be unique in the file unless replace_all is true.",
					},
					"new_string": map[string]any{
						"type":        "string",
						"description": "The replacement text. Can be empty to delete old_string.",
					},
					"replace_all": map[string]any{
						"type":        "boolean",
						"description": "If true, replace all occurrences. Default is false (replace first occurrence only).",
					},
				},
				"required": []string{"file_path", "old_string", "new_string"},
			},
		},
		{
			Name:        "Write",
			Description: "Write content to a file. Creates parent directories if needed. Overwrites existing file if present.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "The path to the file to write (absolute or relative to current directory)",
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
			Name:        "Bash",
			Description: "Execute shell commands. Use for running git commands, build tools, package managers, or any system operations. Commands run in bash with the current working directory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The shell command to execute",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Brief description of what this command does (shown in permission prompt)",
					},
					"timeout": map[string]any{
						"type":        "integer",
						"description": "Timeout in milliseconds (default: 120000, max: 600000)",
					},
					"run_in_background": map[string]any{
						"type":        "boolean",
						"description": "Run command in background (default: false)",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			Name:        "TaskOutput",
			Description: "Retrieve output from a running or completed background task. Use this to check on background tasks started with Bash run_in_background=true.",
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
			Description: "Stops a running background task by its ID. Takes a task_id parameter identifying the task to stop. Returns a success or failure status. Use this tool when you need to terminate a long-running task.",
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
			Name:        "AskUserQuestion",
			Description: "Ask the user questions to gather preferences, clarify requirements, or get decisions on implementation choices. Use when you need user input to proceed.",
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

	// Add Task tool
	tools = append(tools, TaskToolSchema)

	// Add MCP tools if getter is provided
	if mcpToolsGetter != nil {
		tools = append(tools, mcpToolsGetter()...)
	}

	return tools
}

// TaskToolSchema returns the schema for the Task tool
var TaskToolSchema = provider.Tool{
	Name: "Task",
	Description: `Launch a subagent to handle complex, multi-step tasks autonomously.

The Task tool launches specialized agents that autonomously handle complex tasks. Each agent type has specific capabilities and tools available to it.

Available agent types and the tools they have access to:
- Bash: Command execution specialist for running bash commands. Use this for git operations, command execution, and other terminal tasks. (Tools: Bash, Read, Glob, Grep)
- Explore: Fast agent specialized for exploring codebases. Use this when you need to quickly find files by patterns, search code for keywords, or answer questions about the codebase. (Tools: Read, Glob, Grep, WebFetch, WebSearch)
- Plan: Software architect agent for designing implementation plans. Use this when you need to plan the implementation strategy for a task. Returns step-by-step plans, identifies critical files, and considers architectural trade-offs. (Tools: Read, Glob, Grep, WebFetch, WebSearch)
- Review: Code review specialist for analyzing code changes, identifying issues, and suggesting improvements. (Tools: Read, Glob, Grep, Bash)
- general-purpose: General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks. When you are searching for something and are not confident that you will find the right match quickly, use this agent. (Tools: all except Task)

Usage notes:
- Always include a short description (3-5 words) summarizing what the agent will do
- Launch multiple agents concurrently whenever possible using run_in_background=true
- Use TaskOutput to check on background agents, TaskStop to stop them
- Agents can be resumed using the resume parameter with a previous agent ID
- Each agent runs in isolated context - only final result returns to main conversation`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "The type of agent to spawn (Bash, Explore, Plan, Review, general-purpose, or custom agent name)",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The task for the agent to perform",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "A short (3-5 word) description of the task",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "Run the agent in background (default: false). Returns task_id immediately.",
				"default":     false,
			},
			"resume": map[string]any{
				"type":        "string",
				"description": "Optional agent ID to resume from a previous execution. When resumed, agent continues with full previous context preserved.",
			},
			"model": map[string]any{
				"type":        "string",
				"description": "Override model: sonnet, opus, haiku. If not specified, inherits from parent conversation.",
				"enum":        []string{"sonnet", "opus", "haiku"},
			},
			"max_turns": map[string]any{
				"type":        "integer",
				"description": "Maximum number of conversation turns before stopping",
			},
		},
		"required": []string{"subagent_type", "prompt"},
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
- Invoke this tool IMMEDIATELY when a skill is relevant
- Do not invoke a skill that is already running`,
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
	Name:        "EnterPlanMode",
	Description: "Request to enter plan mode for complex implementation tasks. Use this proactively when starting non-trivial tasks that require exploration and planning before making changes. The user must approve entering plan mode.",
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
				"description": "The complete implementation plan in Markdown format. Should include: Summary, Analysis, Implementation Steps, Testing Strategy, and Risks.",
			},
		},
		"required": []string{"plan"},
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

// GetPlanModeToolSchemas returns only the tools available in plan mode
// Plan mode restricts to read-only tools plus ExitPlanMode
func GetPlanModeToolSchemas() []provider.Tool {
	// Read-only tools allowed in plan mode
	allowedTools := map[string]bool{
		"Read":      true,
		"Glob":      true,
		"Grep":      true,
		"WebFetch":  true,
		"WebSearch": true,
	}

	// Filter to allowed tools
	allTools := GetToolSchemas()
	tools := make([]provider.Tool, 0, len(allowedTools)+1)

	for _, t := range allTools {
		if allowedTools[t.Name] {
			tools = append(tools, t)
		}
	}

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
