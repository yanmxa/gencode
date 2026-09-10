package core

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackoffDelayFloorWins(t *testing.T) {
	if got := backoffDelay(1, 5*time.Second, 0); got != 5*time.Second {
		t.Fatalf("backoffDelay floor = %v, want 5s", got)
	}
}

func TestBackoffDelayGrowsAndCaps(t *testing.T) {
	a1 := backoffDelay(1, 0, 1)
	a2 := backoffDelay(2, 0, 1)
	if a1 != retryBaseDelay {
		t.Fatalf("attempt1 = %v, want %v", a1, retryBaseDelay)
	}
	if a2 != 2*retryBaseDelay {
		t.Fatalf("attempt2 = %v, want %v", a2, 2*retryBaseDelay)
	}
	if capped := backoffDelay(20, 0, 1); capped != retryMaxDelay {
		t.Fatalf("attempt20 = %v, want cap %v", capped, retryMaxDelay)
	}
}

func TestBackoffSleepHonorsCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := BackoffSleep(ctx, 1, 10*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("BackoffSleep on canceled ctx = %v, want context.Canceled", err)
	}
}
