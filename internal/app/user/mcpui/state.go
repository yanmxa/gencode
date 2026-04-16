package mcpui

import "github.com/yanmxa/gencode/internal/extension/mcp"

// State holds all MCP-related state for the TUI model.
type State struct {
	Selector      Model
	EditingFile   string    // temp file being edited
	EditingServer string    // server name being edited
	EditingScope  mcp.Scope // scope of the server being edited
}

// EditorFinishedMsg is sent when the external MCP config editor closes.
type EditorFinishedMsg struct {
	Err error
}
