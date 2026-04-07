package mode

import "testing"

func TestNextWithBypass_Disabled(t *testing.T) {
	cycle := []OperationMode{Normal, AutoAccept, Plan, Normal}
	for i := 0; i < len(cycle)-1; i++ {
		got := cycle[i].NextWithBypass(false)
		if got != cycle[i+1] {
			t.Errorf("NextWithBypass(false): from %d, got %d, want %d", cycle[i], got, cycle[i+1])
		}
	}
}

func TestNextWithBypass_Enabled(t *testing.T) {
	cycle := []OperationMode{Normal, AutoAccept, Plan, BypassPermissions, Normal}
	for i := 0; i < len(cycle)-1; i++ {
		got := cycle[i].NextWithBypass(true)
		if got != cycle[i+1] {
			t.Errorf("NextWithBypass(true): from %d, got %d, want %d", cycle[i], got, cycle[i+1])
		}
	}
}

func TestNextWithBypass_UnknownMode(t *testing.T) {
	unknown := OperationMode(99)
	if got := unknown.NextWithBypass(false); got != Normal {
		t.Errorf("NextWithBypass(false) from unknown: got %d, want %d", got, Normal)
	}
	if got := unknown.NextWithBypass(true); got != Normal {
		t.Errorf("NextWithBypass(true) from unknown: got %d, want %d", got, Normal)
	}
}

func TestNext_StillWorks(t *testing.T) {
	cycle := []OperationMode{Normal, AutoAccept, Plan, Normal}
	for i := 0; i < len(cycle)-1; i++ {
		got := cycle[i].Next()
		if got != cycle[i+1] {
			t.Errorf("Next(): from %d, got %d, want %d", cycle[i], got, cycle[i+1])
		}
	}
}

func TestNext_BypassReturnsNormal(t *testing.T) {
	got := BypassPermissions.Next()
	if got != Normal {
		t.Errorf("Next() from BypassPermissions: got %d, want %d", got, Normal)
	}
}
