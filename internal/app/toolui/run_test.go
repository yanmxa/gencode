package toolui

import (
	"context"
	"testing"

	appmode "github.com/yanmxa/gencode/internal/app/mode"
	"github.com/yanmxa/gencode/internal/config"
	"github.com/yanmxa/gencode/internal/message"
	coretool "github.com/yanmxa/gencode/internal/tool"
	_ "github.com/yanmxa/gencode/internal/tool/registry"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

type cancellableTestTool struct{}

func (t *cancellableTestTool) Name() string        { return "AppToolCancellationTestTool" }
func (t *cancellableTestTool) Description() string { return "test tool" }
func (t *cancellableTestTool) Icon() string        { return "t" }
func (t *cancellableTestTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	select {
	case <-ctx.Done():
		return toolresult.NewErrorResult(t.Name(), "cancelled")
	default:
		return toolresult.ToolResult{Success: true, Output: "not cancelled"}
	}
}

func TestExecuteParallelPropagatesContextCancellation(t *testing.T) {
	coretool.Register(&cancellableTestTool{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := ExecuteParallel(ctx, nil, []message.ToolCall{{
		ID:    "tc1",
		Name:  "AppToolCancellationTestTool",
		Input: `{}`,
	}}, "", nil, nil, false, nil, nil)

	msg := cmd()
	result, ok := msg.(ExecResultMsg)
	if !ok {
		t.Fatalf("expected ExecResultMsg, got %T", msg)
	}
	if !result.Result.IsError {
		t.Fatal("expected cancelled execution to produce an error result")
	}
	if result.Result.Content != "Error: cancelled" {
		t.Fatalf("expected cancellation content, got %q", result.Result.Content)
	}
}

func TestRequiresUserInteraction_AskUserQuestionEvenWhenSafeToolAllowed(t *testing.T) {
	settings := config.Default()
	tc := message.ToolCall{
		ID:    "tc-question",
		Name:  "AskUserQuestion",
		Input: `{"questions":[{"question":"Choose","header":"Pick","options":[{"label":"A"},{"label":"B"}]}]}`,
	}

	if !RequiresUserInteraction(tc, settings, nil, false, nil, nil) {
		t.Fatal("expected AskUserQuestion to require interaction even when it is a safe tool")
	}
}

func TestProcessNext_RoutesAskUserQuestionToQuestionRequest(t *testing.T) {
	settings := config.Default()
	tc := message.ToolCall{
		ID:    "tc-question",
		Name:  "AskUserQuestion",
		Input: `{"questions":[{"question":"Choose","header":"Pick","options":[{"label":"A"},{"label":"B"}]}]}`,
	}

	msg := ProcessNext(context.Background(), nil, []message.ToolCall{tc}, 0, "", settings, nil)()
	reqMsg, ok := msg.(appmode.QuestionRequestMsg)
	if !ok {
		t.Fatalf("expected QuestionRequestMsg, got %T", msg)
	}
	if reqMsg.Request == nil || len(reqMsg.Request.Questions) != 1 {
		t.Fatalf("unexpected question request: %#v", reqMsg.Request)
	}
}
