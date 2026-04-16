package cron

import (
	"strings"
	"testing"
	"time"
)

func TestParseLoopCommand_RoundsNonCleanIntervals(t *testing.T) {
	now := time.Date(2026, 4, 6, 14, 30, 0, 0, time.Local)

	parsed, err := ParseLoopCommand("90m check the deploy", now)
	if err != nil {
		t.Fatalf("ParseLoopCommand failed: %v", err)
	}
	if parsed.Cron != "37 */2 * * *" {
		t.Fatalf("unexpected cron expression %q", parsed.Cron)
	}
	if !strings.Contains(parsed.Note, "Rounded `90m` to `every 2 hour(s)`") {
		t.Fatalf("expected rounding note, got %q", parsed.Note)
	}
}

func TestParseLoopCommand_AvoidsTopOfHourScheduling(t *testing.T) {
	now := time.Date(2026, 4, 6, 10, 0, 0, 0, time.Local)

	parsed, err := ParseLoopCommand("1h check deploy", now)
	if err != nil {
		t.Fatalf("ParseLoopCommand failed: %v", err)
	}
	if parsed.Cron != "7 * * * *" {
		t.Fatalf("unexpected cron expression %q", parsed.Cron)
	}
}

func TestParseLoopOnceCommand_SupportsTrailingInClause(t *testing.T) {
	now := time.Date(2026, 4, 6, 10, 5, 0, 0, time.Local)

	parsed, err := ParseLoopOnceCommand("check deploy in 20m", now)
	if err != nil {
		t.Fatalf("ParseLoopOnceCommand failed: %v", err)
	}
	if parsed.Prompt != "check deploy" {
		t.Fatalf("unexpected prompt %q", parsed.Prompt)
	}
	if parsed.Cron != "25 10 6 4 *" {
		t.Fatalf("unexpected cron expression %q", parsed.Cron)
	}
	if !strings.Contains(parsed.Human, "once at 2026-04-06 10:25") {
		t.Fatalf("unexpected human schedule %q", parsed.Human)
	}
}
