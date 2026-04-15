package core

import "context"

// Tool is a single capability an agent can execute.
//
// Tools are pure: they don't know about hooks, permissions, or conversation history.
// The agent loop handles interception (via hooks) and result recording (via Message).
//
// Execute returns plain text. The agent loop wraps it into a ToolResult and
// appends it to the conversation as a Message with Role=RoleTool.
type Tool interface {
	Name() string
	Description() string
	Schema() ToolSchema

	// Execute runs the tool with the given input.
	// Returns the result text on success, or an error on failure.
	// The agent wraps errors as ToolResult{IsError: true}.
	Execute(ctx context.Context, input map[string]any) (string, error)
}

// ToolSchema is a typed tool definition sent to the LLM.
type ToolSchema struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"input_schema,omitempty"` // JSON Schema object
}

// Tools is a mutable, queryable collection of tools.
//
// Can change dynamically: hooks add/remove tools, plan mode restricts
// to read-only, parent agents filter child tool sets.
type Tools interface {
	Get(name string) Tool
	All() []Tool
	Add(tool Tool)
	Remove(name string)
	Schemas() []ToolSchema
}
