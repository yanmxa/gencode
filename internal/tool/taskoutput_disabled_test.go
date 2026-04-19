package tool

import "testing"

func TestTaskOutputIsNotDeferredOrSearchable(t *testing.T) {
	if _, ok := deferredToolNames[ToolTaskOutput]; ok {
		t.Fatalf("did not expect %s to remain deferred once disabled", ToolTaskOutput)
	}

	ResetFetched()
	t.Cleanup(ResetFetched)
	MarkFetched(ToolTaskOutput)

	set := &Set{}
	for _, schema := range set.Tools() {
		if schema.Name == ToolTaskOutput {
			t.Fatalf("did not expect disabled tool %s in tool set", ToolTaskOutput)
		}
	}

	for _, schema := range SearchDeferredTools("TaskOutput", 5) {
		if schema.Name == ToolTaskOutput {
			t.Fatalf("did not expect disabled tool %s in deferred search results", ToolTaskOutput)
		}
	}
}

