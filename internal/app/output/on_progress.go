package output

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yanmxa/gencode/internal/tool"
)

type progressUpdate struct {
	Index   int
	Message string
}

// ProgressUpdateMsg carries a task progress update from an agent.
type ProgressUpdateMsg struct {
	Index   int
	Message string
}

type progressQuestionUpdate struct {
	Index   int
	Request *tool.QuestionRequest
	Reply   chan *tool.QuestionResponse
}

// ProgressQuestionMsg carries an agent question request to the TUI.
type ProgressQuestionMsg struct {
	Index   int
	Request *tool.QuestionRequest
	Reply   chan *tool.QuestionResponse
}

// ProgressCheckTickMsg triggers a check for new progress updates.
type ProgressCheckTickMsg struct{}

// ProgressHub is an instance-scoped progress transport.
type ProgressHub struct {
	ch  chan progressUpdate
	qch chan progressQuestionUpdate
}

// NewProgressHub creates a new progress hub with the given buffer size.
func NewProgressHub(buffer int) *ProgressHub {
	if buffer <= 0 {
		buffer = 100
	}
	return &ProgressHub{
		ch:  make(chan progressUpdate, buffer),
		qch: make(chan progressQuestionUpdate, buffer),
	}
}

// SendForAgent enqueues a progress message for a specific agent index.
func (h *ProgressHub) SendForAgent(index int, msg string) {
	select {
	case h.ch <- progressUpdate{Index: index, Message: msg}:
	default:
	}
}

// Ask enqueues an interactive question and waits for the user's response.
func (h *ProgressHub) Ask(ctx context.Context, index int, req *tool.QuestionRequest) (*tool.QuestionResponse, error) {
	if h == nil {
		return nil, fmt.Errorf("progress hub not initialized")
	}

	reply := make(chan *tool.QuestionResponse, 1)
	select {
	case h.qch <- progressQuestionUpdate{Index: index, Request: req, Reply: reply}:
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
func (h *ProgressHub) Check() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		select {
		case q := <-h.qch:
			return ProgressQuestionMsg{Index: q.Index, Request: q.Request, Reply: q.Reply}
		case u := <-h.ch:
			return ProgressUpdateMsg(u)
		default:
			return ProgressCheckTickMsg{}
		}
	})
}

// Drain pulls all pending updates into taskProgress.
func (h *ProgressHub) Drain(taskProgress map[int][]string) map[int][]string {
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
