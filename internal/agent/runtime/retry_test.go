package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/genai-io/san/internal/core"
)

func TestStreamSentinelsAreRetryable(t *testing.T) {
	for _, e := range []error{errStreamStalled, errStreamTruncated} {
		var re RetryableError
		if !errors.As(e, &re) {
			t.Fatalf("%v should satisfy RetryableError", e)
		}
		if re.RetryAfter() != 0 {
			t.Fatalf("%v RetryAfter = %v, want 0", e, re.RetryAfter())
		}
	}
}

// scriptedLLM fails the first `failures` Infer calls with failErr, then
// completes with a text-only end_turn response.
type scriptedLLM struct {
	failErr  error
	failures int
	calls    int
}

func (s *scriptedLLM) InputLimit() int { return 0 }

func (s *scriptedLLM) Infer(_ context.Context, _ InferRequest) (<-chan Chunk, error) {
	s.calls++
	fail := s.calls <= s.failures
	ch := make(chan Chunk, 1)
	go func() {
		defer close(ch)
		if fail {
			ch <- Chunk{Err: s.failErr}
			return
		}
		ch <- Chunk{Done: true, Response: &InferResponse{Content: "ok", StopReason: StopEndTurn}}
	}()
	return ch, nil
}

// hangLLM never produces a chunk; it unblocks only when the per-inference ctx
// is canceled (by the idle-timeout watchdog).
type hangLLM struct{ calls int }

func (h *hangLLM) InputLimit() int { return 0 }

func (h *hangLLM) Infer(ctx context.Context, _ InferRequest) (<-chan Chunk, error) {
	h.calls++
	ch := make(chan Chunk)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func newRetryAgent(t *testing.T, llm LLM, maxRetries int, timeout time.Duration) *agent {
	t.Helper()
	ag := New(Config{
		ID:                      "test",
		LLM:                     llm,
		System:                  NewSystem(),
		Tools:                   NewTools(),
		MaxTurnRetries:          maxRetries,
		StreamFirstChunkTimeout: timeout,
		StreamIdleTimeout:       timeout,
	})
	go func() {
		for range ag.Outbox() {
		}
	}()
	a := ag.(*agent)
	a.SetMessages([]Message{UserMessage("hi", nil)})
	return a
}

func TestThinkActRetriesTransientStreamError(t *testing.T) {
	llm := &scriptedLLM{failErr: errStreamStalled, failures: 2}
	a := newRetryAgent(t, llm, 2, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := a.ThinkAct(ctx)
	if err != nil {
		t.Fatalf("ThinkAct returned error after retries: %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("content = %q, want ok", result.Content)
	}
	if llm.calls != 3 {
		t.Fatalf("Infer calls = %d, want 3 (2 failures + 1 success)", llm.calls)
	}
}

func TestThinkActSurfacesErrorAfterMaxRetries(t *testing.T) {
	llm := &scriptedLLM{failErr: errStreamStalled, failures: 99}
	a := newRetryAgent(t, llm, 2, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := a.ThinkAct(ctx); err == nil {
		t.Fatal("ThinkAct should surface the error after exhausting retries")
	}
	if llm.calls != 3 { // 1 initial + 2 retries
		t.Fatalf("Infer calls = %d, want 3", llm.calls)
	}
}

func TestThinkActDoesNotRetryFatalError(t *testing.T) {
	llm := &scriptedLLM{failErr: errors.New("400 bad request"), failures: 99}
	a := newRetryAgent(t, llm, 3, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := a.ThinkAct(ctx); err == nil {
		t.Fatal("ThinkAct should surface a fatal error")
	}
	if llm.calls != 1 {
		t.Fatalf("Infer calls = %d, want 1 (no retry on fatal)", llm.calls)
	}
}

func TestStreamInferIdleTimeoutRetries(t *testing.T) {
	llm := &hangLLM{}
	a := newRetryAgent(t, llm, 1, 40*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := a.ThinkAct(ctx); err == nil {
		t.Fatal("ThinkAct should fail once a stalled stream exhausts retries")
	}
	if llm.calls != 2 { // 1 initial + 1 retry, each stalls
		t.Fatalf("Infer calls = %d, want 2 (idle-timeout retry)", llm.calls)
	}
}
