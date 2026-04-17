package input

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/llm"
)

// ConvWriter is the shared base for overlay Runtime interfaces that
// need to append messages and commit them for rendering.
type ConvWriter interface {
	AppendMessage(msg core.ChatMessage)
	CommitMessages() []tea.Cmd
}

// Runtime composes all overlay Runtime interfaces.
// The root app model satisfies this by implementing each sub-interface.
type Runtime interface {
	MCPRuntime
	MemoryRuntime
	PluginRuntime
	ProviderRuntime
	SessionRuntime
	SearchRuntime
}

// MCPRuntime defines the callbacks the MCP handler needs from the parent app model.
type MCPRuntime interface {
	ConvWriter
	SetInputText(text string)
}

// MemoryRuntime defines the callbacks the memory feature needs from the parent app model.
type MemoryRuntime interface {
	ConvWriter
	GetCwd() string
	ClearCachedInstructions()
	RefreshMemoryContext(trigger string)
	FireFileChanged(path, tool string)
}

// PluginRuntime defines the callbacks the plugin selector needs from the parent app model.
type PluginRuntime interface {
	GetCwd() string
	ReloadPluginBackedState() error
}

// ProviderRuntime defines the callbacks the provider selector needs from the parent app model.
type ProviderRuntime interface {
	ConvWriter
	SwitchProvider(p llm.Provider)
	SetCurrentModel(m *llm.CurrentModelInfo)
}

// SessionRuntime defines the callbacks the session selector needs from the parent app model.
type SessionRuntime interface {
	LoadSession(id string) error
	AddNotice(text string)
	ResetCommitIndex()
	CommitAllMessages() []tea.Cmd
}

// SearchRuntime defines the callbacks the search selector needs from the parent app model.
type SearchRuntime interface {
	SetProviderStatusMessage(msg string)
}
