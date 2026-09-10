package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/genai-io/san/internal/core"
)

type sessionTestAgent struct {
	core.Agent
	inbox  chan core.Message
	outbox chan core.Event
	run    func(context.Context) error

	mu       sync.Mutex
	messages []core.Message
}

func newSessionTestAgent(run func(context.Context) error) *sessionTestAgent {
	return &sessionTestAgent{
		inbox:  make(chan core.Message, 4),
		outbox: make(chan core.Event, 4),
		run:    run,
	}
}

func (a *sessionTestAgent) Inbox() chan<- core.Message { return a.inbox }
func (a *sessionTestAgent) Outbox() <-chan core.Event  { return a.outbox }
func (a *sessionTestAgent) Run(ctx context.Context) error {
	if a.run == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	return a.run(ctx)
}
func (a *sessionTestAgent) SetMessages(messages []core.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = append([]core.Message(nil), messages...)
}

func TestSessionStopWaitsForRunBeforeRestart(t *testing.T) {
	exited := make(chan struct{})
	first := newSessionTestAgent(func(ctx context.Context) error {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		close(exited)
		return ctx.Err()
	})
	second := newSessionTestAgent(nil)
	agents := []*sessionTestAgent{first, second}

	s := &Session{build: func(BuildParams) (core.Agent, *PermissionBridge, error) {
		ag := agents[0]
		agents = agents[1:]
		return ag, nil, nil
	}}
	if err := s.Start(BuildParams{}, nil); err != nil {
		t.Fatalf("Start first: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.StopContext(ctx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	select {
	case <-exited:
	default:
		t.Fatal("StopContext returned before Run exited")
	}
	if s.Active() {
		t.Fatal("session remained active after Run exited")
	}

	if err := s.Start(BuildParams{}, nil); err != nil {
		t.Fatalf("Start second: %v", err)
	}
	s.Stop()
}

func TestSessionDoesNotOverlapRunStillStopping(t *testing.T) {
	release := make(chan struct{})
	ag := newSessionTestAgent(func(ctx context.Context) error {
		<-ctx.Done()
		<-release
		return ctx.Err()
	})
	s := &Session{build: func(BuildParams) (core.Agent, *PermissionBridge, error) {
		return ag, nil, nil
	}}
	if err := s.Start(BuildParams{}, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err := s.StopContext(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StopContext error = %v, want deadline exceeded", err)
	}
	if err := s.Start(BuildParams{}, nil); err == nil {
		t.Fatal("Start succeeded while previous Run was still stopping")
	}

	close(release)
	waitForSessionState(t, time.Second, func() bool { return !s.Active() })
}

func TestSessionSendPreservesMessageIdentity(t *testing.T) {
	received := make(chan core.Message, 1)
	ag := newSessionTestAgent(nil)
	ag.run = func(ctx context.Context) error {
		select {
		case msg := <-ag.inbox:
			received <- msg
			<-ctx.Done()
			return ctx.Err()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s := &Session{build: func(BuildParams) (core.Agent, *PermissionBridge, error) {
		return ag, nil, nil
	}}
	if err := s.Start(BuildParams{}, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	want := core.Message{ID: "ui-message-1", Role: core.RoleUser, Content: "hello"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Send(ctx, want); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case got := <-received:
		if got.ID != want.ID || got.Content != want.Content {
			t.Fatalf("received = %+v, want ID/content from %+v", got, want)
		}
	case <-ctx.Done():
		t.Fatal("agent did not receive message")
	}
	if err := s.StopContext(ctx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if err := s.Send(ctx, want); !errors.Is(err, ErrSessionInactive) {
		t.Fatalf("Send after stop error = %v, want ErrSessionInactive", err)
	}
}

func TestSessionUnexpectedRunExitClearsActiveAndKeepsError(t *testing.T) {
	wantErr := errors.New("runner failed")
	ag := newSessionTestAgent(func(context.Context) error { return wantErr })
	s := &Session{build: func(BuildParams) (core.Agent, *PermissionBridge, error) {
		return ag, nil, nil
	}}
	if err := s.Start(BuildParams{}, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitForSessionState(t, time.Second, func() bool { return !s.Active() })
	if !errors.Is(s.LastRunError(), wantErr) {
		t.Fatalf("LastRunError = %v, want %v", s.LastRunError(), wantErr)
	}
}

func waitForSessionState(t *testing.T, timeout time.Duration, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("session did not reach expected state")
}
