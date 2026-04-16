package mode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/tool"
	"github.com/yanmxa/gencode/internal/tool/toolresult"
)

// AskUserQuestionTool prompts the user for input
type AskUserQuestionTool struct {
	requestCounter int
}

// NewAskUserQuestionTool creates a new AskUserQuestionTool
func NewAskUserQuestionTool() *AskUserQuestionTool {
	return &AskUserQuestionTool{}
}

func (t *AskUserQuestionTool) Name() string {
	return "AskUserQuestion"
}

func (t *AskUserQuestionTool) Description() string {
	return "Ask the user questions to gather preferences, clarify requirements, or get decisions on implementation choices."
}

func (t *AskUserQuestionTool) Icon() string {
	return "❓"
}

func (t *AskUserQuestionTool) RequiresInteraction() bool {
	return true
}

// PrepareInteraction parses parameters and returns a QuestionRequest
func (t *AskUserQuestionTool) PrepareInteraction(ctx context.Context, params map[string]any, cwd string) (any, error) {
	questionsRaw, ok := params["questions"]
	if !ok {
		return nil, fmt.Errorf("missing required parameter: questions")
	}

	// Convert to JSON and back to properly parse the structure
	questionsJSON, err := json.Marshal(questionsRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid questions format: %w", err)
	}

	var questions []tool.Question
	if err := json.Unmarshal(questionsJSON, &questions); err != nil {
		return nil, fmt.Errorf("failed to parse questions: %w", err)
	}

	// Validate questions
	if len(questions) == 0 || len(questions) > 4 {
		return nil, fmt.Errorf("questions must have 1-4 items, got %d", len(questions))
	}

	for i, q := range questions {
		if q.Question == "" {
			return nil, fmt.Errorf("question[%d]: question text is required", i)
		}
		if len(q.Header) > 12 {
			return nil, fmt.Errorf("question[%d]: header must be at most 12 characters", i)
		}
		if len(q.Options) < 2 || len(q.Options) > 4 {
			return nil, fmt.Errorf("question[%d]: must have 2-4 options, got %d", i, len(q.Options))
		}
		for j, opt := range q.Options {
			if opt.Label == "" {
				return nil, fmt.Errorf("question[%d].options[%d]: label is required", i, j)
			}
		}
	}

	t.requestCounter++
	return &tool.QuestionRequest{
		ID:        fmt.Sprintf("ask-%d", t.requestCounter),
		Questions: questions,
	}, nil
}

// ExecuteWithResponse formats the user's response for the LLM
func (t *AskUserQuestionTool) ExecuteWithResponse(ctx context.Context, params map[string]any, response any, cwd string) toolresult.ToolResult {
	resp, ok := response.(*tool.QuestionResponse)
	if !ok {
		return toolresult.NewErrorResult("AskUserQuestion", "invalid response type")
	}

	if resp.Cancelled {
		return toolresult.ToolResult{
			Success: true,
			Output:  "User cancelled the question prompt without answering.",
			Metadata: toolresult.ResultMetadata{
				Title:    "AskUserQuestion",
				Icon:     "❓",
				Subtitle: "Cancelled",
			},
		}
	}

	// Format answers for the LLM
	var sb strings.Builder
	sb.WriteString("User responses:\n")

	// Get original questions for context
	questionsJSON, err := json.Marshal(params["questions"])
	if err != nil {
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("failed to marshal questions: %v", err))
	}
	var questions []tool.Question
	if err := json.Unmarshal(questionsJSON, &questions); err != nil {
		return toolresult.NewErrorResult(t.Name(), fmt.Sprintf("failed to unmarshal questions: %v", err))
	}

	for i, q := range questions {
		answers := resp.Answers[i]
		if len(answers) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n%s: ", q.Header))
		sb.WriteString(strings.Join(answers, ", "))
	}

	return toolresult.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: toolresult.ResultMetadata{
			Title:    "AskUserQuestion",
			Icon:     "❓",
			Subtitle: fmt.Sprintf("%d answers", len(resp.Answers)),
		},
	}
}

// Execute should not be called directly for interactive tools
func (t *AskUserQuestionTool) Execute(ctx context.Context, params map[string]any, cwd string) toolresult.ToolResult {
	return toolresult.NewErrorResult("AskUserQuestion", "this tool requires user interaction - use PrepareInteraction and ExecuteWithResponse")
}

func init() {
	tool.Register(NewAskUserQuestionTool())
}
