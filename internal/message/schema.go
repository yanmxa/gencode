package message

// ToolSchema represents a tool definition sent to LLMs (JSON Schema format).
type ToolSchema struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}
