package progress

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type update struct {
	Index   int
	Message string
}

// UpdateMsg carries a task progress update from an agent.
type UpdateMsg struct {
	Index   int
	Message string
}

// CheckTickMsg triggers a check for new progress updates.
type CheckTickMsg struct{}

// Hub is an instance-scoped progress transport.
type Hub struct {
	ch chan update
}

// NewHub creates a new progress hub with the given buffer size.
func NewHub(buffer int) *Hub {
	if buffer <= 0 {
		buffer = 100
	}
	return &Hub{ch: make(chan update, buffer)}
}

// Send enqueues a progress message for the default agent index.
func (h *Hub) Send(msg string) {
	h.SendForAgent(0, msg)
}

// SendForAgent enqueues a progress message for a specific agent index.
func (h *Hub) SendForAgent(index int, msg string) {
	select {
	case h.ch <- update{Index: index, Message: msg}:
	default:
	}
}

// Check returns a tea.Cmd that polls this hub for the next update.
func (h *Hub) Check() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		select {
		case u := <-h.ch:
			return UpdateMsg(u)
		default:
			return CheckTickMsg{}
		}
	})
}

// Drain pulls all pending updates into taskProgress.
func (h *Hub) Drain(taskProgress map[int][]string) map[int][]string {
	for {
		select {
		case u := <-h.ch:
			if taskProgress == nil {
				taskProgress = make(map[int][]string)
			}
			taskProgress[u.Index] = append(taskProgress[u.Index], u.Message)
			if len(taskProgress[u.Index]) > 5 {
				taskProgress[u.Index] = taskProgress[u.Index][1:]
			}
		default:
			return taskProgress
		}
	}
}
