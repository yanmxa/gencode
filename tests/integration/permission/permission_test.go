package permission_test

import (
	"context"
	"testing"

	"github.com/yanmxa/gencode/internal/core"
	"github.com/yanmxa/gencode/internal/message"
	"github.com/yanmxa/gencode/internal/permission"
	"github.com/yanmxa/gencode/tests/integration/testutil"
)

func TestPermission_PermitAll_AllowsWrite(t *testing.T) {
	testutil.RegisterFakeTool(t, "Write", "written successfully")

	loop, _ := testutil.NewTestLoopWithPermission(t, permission.PermitAll(),
		testutil.ToolCallResponse("Write", "tc1", `{"file_path": "/tmp/test"}`),
		testutil.EndTurnResponse("done"),
	)
	loop.AddUser("write a file", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify tool executed successfully (no error results)
	for _, m := range result.Messages {
		if m.ToolResult != nil && m.ToolResult.IsError {
			t.Errorf("unexpected error result: %s", m.ToolResult.Content)
		}
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected 'end_turn', got %q", result.StopReason)
	}
}

func TestPermission_ReadOnly_BlocksWrite(t *testing.T) {
	testutil.RegisterFakeTool(t, "Write", "should not execute")

	loop, _ := testutil.NewTestLoopWithPermission(t, permission.ReadOnly(),
		testutil.ToolCallResponse("Write", "tc1", `{"file_path": "/tmp/test"}`),
		testutil.EndTurnResponse("ok"),
	)
	loop.AddUser("write", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify tool was rejected
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

	loop, _ := testutil.NewTestLoopWithPermission(t, permission.ReadOnly(),
		testutil.ToolCallResponse("Read", "tc1", `{"file_path": "/tmp/test"}`),
		testutil.EndTurnResponse("done"),
	)
	loop.AddUser("read", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify tool executed without error
	for _, m := range result.Messages {
		if m.ToolResult != nil && m.ToolResult.IsError {
			t.Errorf("unexpected error for Read tool: %s", m.ToolResult.Content)
		}
	}
}

func TestPermission_DenyAll_BlocksEverything(t *testing.T) {
	testutil.RegisterFakeTool(t, "Read", "should not execute")

	loop, _ := testutil.NewTestLoopWithPermission(t, permission.DenyAll(),
		testutil.ToolCallResponse("Read", "tc1", `{}`),
		testutil.EndTurnResponse("done"),
	)
	loop.AddUser("read", nil)

	result, err := loop.Run(context.Background(), core.RunOptions{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
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

func TestPermission_ExecTool_Directly(t *testing.T) {
	testutil.RegisterFakeTool(t, "Bash", "executed")
	tc := message.ToolCall{ID: "tc1", Name: "Bash", Input: `{"command": "echo hello"}`}

	tests := []struct {
		name      string
		checker   permission.Checker
		wantError bool
	}{
		{"PermitAll allows Bash", permission.PermitAll(), false},
		{"DenyAll rejects Bash", permission.DenyAll(), true},
		{"ReadOnly rejects Bash", permission.ReadOnly(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, _ := testutil.NewTestLoopWithPermission(t, tt.checker)
			result := loop.ExecTool(context.Background(), tc)
			if result.IsError != tt.wantError {
				t.Errorf("IsError = %v, want %v (content: %s)", result.IsError, tt.wantError, result.Content)
			}
		})
	}
}
