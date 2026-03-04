package progress

import (
	"sync"
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

var (
	ch   chan update
	once sync.Once
)

func getChan() chan update {
	once.Do(func() {
		ch = make(chan update, 100)
	})
	return ch
}

func Send(msg string) {
	SendForAgent(0, msg)
}

func SendForAgent(index int, msg string) {
	c := getChan()
	select {
	case c <- update{Index: index, Message: msg}:
	default:
	}
}

func Check() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		c := getChan()
		select {
		case u := <-c:
			return UpdateMsg{Index: u.Index, Message: u.Message}
		default:
			return CheckTickMsg{}
		}
	})
}

func Drain(taskProgress map[int][]string) map[int][]string {
	c := getChan()
	for {
		select {
		case u := <-c:
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
