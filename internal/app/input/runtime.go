package input

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/app/conv"
	appruntime "github.com/yanmxa/gencode/internal/app/runtime"
	"github.com/yanmxa/gencode/internal/llm"
)

// OverlayDeps holds all dependencies needed by overlay selector handlers.
type OverlayDeps struct {
	State   *Model
	Conv    *conv.ConversationModel
	Runtime *appruntime.Env
	Cwd     string

	CommitMessages    func() []tea.Cmd
	CommitAllMessages func() []tea.Cmd

	SwitchProvider    func(llm.Provider)
	FireFileChanged   func(path, tool string)
	ReloadPluginState func() error
	LoadSession       func(string) error
}
