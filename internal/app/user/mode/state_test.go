package mode

import (
	"testing"

	"github.com/yanmxa/gencode/internal/config"
)

func TestNextWithBypass_Disabled(t *testing.T) {
	cycle := []config.OperationMode{config.ModeNormal, config.ModeAutoAccept, config.ModePlan, config.ModeNormal}
	for i := 0; i < len(cycle)-1; i++ {
		got := cycle[i].NextWithBypass(false)
		if got != cycle[i+1] {
			t.Errorf("NextWithBypass(false): from %d, got %d, want %d", cycle[i], got, cycle[i+1])
		}
	}
}

func TestNextWithBypass_Enabled(t *testing.T) {
	cycle := []config.OperationMode{config.ModeNormal, config.ModeAutoAccept, config.ModePlan, config.ModeBypassPermissions, config.ModeNormal}
	for i := 0; i < len(cycle)-1; i++ {
		got := cycle[i].NextWithBypass(true)
		if got != cycle[i+1] {
			t.Errorf("NextWithBypass(true): from %d, got %d, want %d", cycle[i], got, cycle[i+1])
		}
	}
}

func TestNextWithBypass_UnknownMode(t *testing.T) {
	unknown := config.OperationMode(99)
	if got := unknown.NextWithBypass(false); got != config.ModeNormal {
		t.Errorf("NextWithBypass(false) from unknown: got %d, want %d", got, config.ModeNormal)
	}
	if got := unknown.NextWithBypass(true); got != config.ModeNormal {
		t.Errorf("NextWithBypass(true) from unknown: got %d, want %d", got, config.ModeNormal)
	}
}

func TestNext_StillWorks(t *testing.T) {
	cycle := []config.OperationMode{config.ModeNormal, config.ModeAutoAccept, config.ModePlan, config.ModeNormal}
	for i := 0; i < len(cycle)-1; i++ {
		got := cycle[i].Next()
		if got != cycle[i+1] {
			t.Errorf("Next(): from %d, got %d, want %d", cycle[i], got, cycle[i+1])
		}
	}
}

func TestNext_BypassReturnsNormal(t *testing.T) {
	got := config.ModeBypassPermissions.Next()
	if got != config.ModeNormal {
		t.Errorf("Next() from ModeBypassPermissions: got %d, want %d", got, config.ModeNormal)
	}
}
