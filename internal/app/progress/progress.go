package progress

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yanmxa/gencode/internal/tool"
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

type questionUpdate struct {
	Index   int
	Request *tool.QuestionRequest
	Reply   chan *tool.QuestionResponse
}

// QuestionMsg carries an agent question request to the TUI.
type QuestionMsg struct {
	Index   int
	Request *tool.QuestionRequest
	Reply   chan *tool.QuestionResponse
}

// CheckTickMsg triggers a check for new progress updates.
type CheckTickMsg struct{}

// Hub is an instance-scoped progress transport.
type Hub struct {
	ch  chan update
	qch chan questionUpdate
}

// NewHub creates a new progress hub with the given buffer size.
func NewHub(buffer int) *Hub {
	if buffer <= 0 {
		buffer = 100
	}
	return &Hub{
		ch:  make(chan update, buffer),
		qch: make(chan questionUpdate, buffer),
	}
}

// SendForAgent enqueues a progress message for a specific agent index.
func (h *Hub) SendForAgent(index int, msg string) {
	select {
	case h.ch <- update{Index: index, Message: msg}:
	default:
	}
}

// Ask enqueues an interactive question and waits for the user's response.
func (h *Hub) Ask(ctx context.Context, index int, req *tool.QuestionRequest) (*tool.QuestionResponse, error) {
	if h == nil {
		return nil, fmt.Errorf("progress hub not initialized")
	}

	reply := make(chan *tool.QuestionResponse, 1)
	select {
	case h.qch <- questionUpdate{Index: index, Request: req, Reply: reply}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case resp := <-reply:
		if resp == nil {
			return nil, fmt.Errorf("question prompt closed without a response")
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Check returns a tea.Cmd that polls this hub for the next update.
func (h *Hub) Check() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		select {
		case q := <-h.qch:
			return QuestionMsg{Index: q.Index, Request: q.Request, Reply: q.Reply}
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
