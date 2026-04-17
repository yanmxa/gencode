package permission_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/tests/integration/testutil"
)

func adaptChecker(checker permission.Checker) core.PermissionFunc {
	return func(_ context.Context, tc core.ToolCall) (bool, string) {
		params, _ := core.ParseToolInput(tc.Input)
		decision := checker.Check(tc.Name, params)
		if decision == permission.Reject {
			return false, fmt.Sprintf("tool %s is not permitted in this mode", tc.Name)
		}
		return true, ""
	}
}

func TestPermission_PermitAll_AllowsWrite(t *testing.T) {
	testutil.RegisterFakeTool(t, "Write", "written successfully")

	ag, _ := testutil.NewTestAgentWithPermission(t, adaptChecker(permission.PermitAll()),
		testutil.ToolCallResponse("Write", "tc1", `{"file_path": "/tmp/test"}`),
		testutil.EndTurnResponse("done"),
	)

	result, err := testutil.RunAgent(context.Background(), ag, "write a file")
	if err != nil {
		t.Fatalf("RunAgent() error: %v", err)
	}

	for _, m := range result.Messages {
		if m.ToolResult != nil && m.ToolResult.IsError {
			t.Errorf("unexpected error result: %s", m.ToolResult.Content)
		}
	}
	if result.StopReason != core.StopEndTurn {
		t.Errorf("expected 'end_turn', got %q", result.StopReason)
	}
}

func TestPermission_ReadOnly_BlocksWrite(t *testing.T) {
	testutil.RegisterFakeTool(t, "Write", "should not execute")

	ag, _ := testutil.NewTestAgentWithPermission(t, adaptChecker(permission.ReadOnly()),
		testutil.ToolCallResponse("Write", "tc1", `{"file_path": "/tmp/test"}`),
		testutil.EndTurnResponse("ok"),
	)

	result, err := testutil.RunAgent(context.Background(), ag, "write")
	if err != nil {
		t.Fatalf("RunAgent() error: %v", err)
	}

	hasError := false
	for _, m := range result.Messages {
		if m.ToolResult != nil && m.ToolResult.IsError {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("expected error result for Write tool in ReadOnly mode")
	}
}

func TestPermission_ReadOnly_AllowsRead(t *testing.T) {
	testutil.RegisterFakeTool(t, "Read", "file contents")

	ag, _ := testutil.NewTestAgentWithPermission(t, adaptChecker(permission.ReadOnly()),
		testutil.ToolCallResponse("Read", "tc1", `{"file_path": "/tmp/test"}`),
		testutil.EndTurnResponse("done"),
	)

	result, err := testutil.RunAgent(context.Background(), ag, "read")
	if err != nil {
		t.Fatalf("RunAgent() error: %v", err)
	}

	for _, m := range result.Messages {
		if m.ToolResult != nil && m.ToolResult.IsError {
			t.Errorf("unexpected error for Read tool: %s", m.ToolResult.Content)
		}
	}
}

func TestPermission_DenyAll_BlocksEverything(t *testing.T) {
	testutil.RegisterFakeTool(t, "Read", "should not execute")

	ag, _ := testutil.NewTestAgentWithPermission(t, adaptChecker(permission.DenyAll()),
		testutil.ToolCallResponse("Read", "tc1", `{}`),
		testutil.EndTurnResponse("done"),
	)

	result, err := testutil.RunAgent(context.Background(), ag, "read")
	if err != nil {
		t.Fatalf("RunAgent() error: %v", err)
	}

	hasError := false
	for _, m := range result.Messages {
		if m.ToolResult != nil && m.ToolResult.IsError {
			hasError = true
			break
		}
	}
	if !hasError {
		t.Error("expected error result for Read tool in DenyAll mode")
	}
}
