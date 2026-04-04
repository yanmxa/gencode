package permission

import (
	"testing"

	"github.com/yanmxa/gencode/internal/config"
)

func TestIsReadOnlyToolMatchesConfig(t *testing.T) {
	tools := []string{
		"Read",
		"Glob",
		"Grep",
		"WebFetch",
		"WebSearch",
		"LSP",
		"Bash",
		"Write",
	}

	for _, name := range tools {
		if got, want := IsReadOnlyTool(name), config.IsReadOnlyTool(name); got != want {
			t.Fatalf("IsReadOnlyTool(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsSafeToolMatchesConfig(t *testing.T) {
	tools := []string{
		"TaskCreate",
		"TaskGet",
		"TaskList",
		"TaskUpdate",
		"AskUserQuestion",
		"EnterPlanMode",
		"ExitPlanMode",
		"ToolSearch",
		"LSP",
		"Edit",
	}

	for _, name := range tools {
		if got, want := IsSafeTool(name), config.IsSafeTool(name); got != want {
			t.Fatalf("IsSafeTool(%q) = %v, want %v", name, got, want)
		}
	}
}
