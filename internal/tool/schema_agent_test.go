package tool

import (
	"strings"
	"testing"
)

func TestEnterPlanModeSchemaDescriptionIsRestrictive(t *testing.T) {
	desc := enterPlanModeSchema.Description

	for _, want := range []string{
		"architecture is still unknown after direct reading/search",
		"Direct implementation would be risky",
		"Tasks that are large only because they touch several files",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("enterPlanModeSchema.Description missing %q", want)
		}
	}
}
