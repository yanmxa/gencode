package output

import (
	"testing"

	"github.com/yanmxa/gencode/internal/app/ui/progress"
)

func TestHandleProgressWithoutHubDoesNotPanic(t *testing.T) {
	m := New(80, nil)

	cmd := m.HandleProgress(progress.UpdateMsg{
		Index:   1,
		Message: "step",
	})
	if cmd == nil {
		t.Fatal("expected spinner cmd even without progress hub")
	}
	if len(m.TaskProgress[1]) != 1 || m.TaskProgress[1][0] != "step" {
		t.Fatalf("unexpected progress state: %#v", m.TaskProgress)
	}
}

func Test_drainProgressWithoutHubIsNoop(t *testing.T) {
	m := New(80, nil)
	m.TaskProgress = map[int][]string{2: {"existing"}}

	m.drainProgress()

	if len(m.TaskProgress[2]) != 1 || m.TaskProgress[2][0] != "existing" {
		t.Fatalf("unexpected progress state after drain: %#v", m.TaskProgress)
	}
}
