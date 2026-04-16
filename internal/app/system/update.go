package system

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yanmxa/gencode/internal/cron"
	"github.com/yanmxa/gencode/internal/hooks"
)

const cronTickInterval = 30 * time.Second
const asyncHookTickInterval = 500 * time.Millisecond
const maxCronQueueSize = 100

// CronTickMsg is sent periodically to check for due cron jobs.
type CronTickMsg struct{}

// AsyncHookTickMsg is sent periodically to check for async hook rewakes.
type AsyncHookTickMsg struct{}

// TriggerCronTickNow returns a command that immediately checks cron jobs once.
func TriggerCronTickNow() tea.Cmd {
	return func() tea.Msg { return CronTickMsg{} }
}

// StartCronTicker returns a command that sends periodic CronTickMsg.
func StartCronTicker() tea.Cmd {
	return tea.Tick(cronTickInterval, func(time.Time) tea.Msg {
		return CronTickMsg{}
	})
}

// StartAsyncHookTicker returns a command that sends periodic AsyncHookTickMsg.
func StartAsyncHookTicker() tea.Cmd {
	return tea.Tick(asyncHookTickInterval, func(time.Time) tea.Msg {
		return AsyncHookTickMsg{}
	})
}

// CronResult holds the result of processing a cron tick.
type CronResult struct {
	// InjectPrompt is a cron prompt to inject as a user message (empty if none).
	InjectPrompt string
	// Notices are informational messages (e.g., "cron fired but no provider").
	Notices []string
}

// HandleCronTick checks for due cron jobs and returns what action to take.
// isIdle indicates whether the REPL is idle (no active stream or tool execution).
func (s *State) HandleCronTick(isIdle bool) CronResult {
	var result CronResult

	// Skip when no jobs exist and queue is empty
	if cron.DefaultStore.Empty() && len(s.CronQueue) == 0 {
		return result
	}

	fired := cron.DefaultStore.Tick()
	injected := false

	for i, f := range fired {
		if !isIdle || i > 0 {
			// Queue if busy, or if another cron prompt already started
			if len(s.CronQueue) < maxCronQueueSize {
				s.CronQueue = append(s.CronQueue, f.Prompt)
			}
		} else {
			result.InjectPrompt = f.Prompt
			injected = true
		}
	}

	// Drain one queued prompt if idle and no prompt was already injected this tick
	if isIdle && !injected && len(s.CronQueue) > 0 {
		result.InjectPrompt = s.CronQueue[0]
		s.CronQueue = s.CronQueue[1:]
	}

	return result
}

// HandleAsyncHookTick checks for pending async hook rewakes and returns what action to take.
// hookEngine may be nil.
func (s *State) HandleAsyncHookTick(hookEngine *hooks.Engine, isIdle bool) *AsyncHookRewake {
	if hookEngine != nil {
		s.HookStatus = hookEngine.CurrentStatusMessage()
	} else {
		s.HookStatus = ""
	}

	if !isIdle {
		return nil
	}

	item, ok := s.AsyncHookQueue.Pop()
	if !ok {
		return nil
	}
	return &item
}
