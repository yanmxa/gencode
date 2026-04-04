package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yanmxa/gencode/internal/tool/ui"
)

// QuestionOption represents a single option for a question
type QuestionOption struct {
	Label       string `json:"label"`       // Display text for the option
	Description string `json:"description"` // Explanation of what this option means
}

// Question represents a question to ask the user
type Question struct {
	Question    string           `json:"question"`    // The complete question text
	Header      string           `json:"header"`      // Short label (max 12 chars)
	Options     []QuestionOption `json:"options"`     // 2-4 options
	MultiSelect bool             `json:"multiSelect"` // Allow multiple selections
}

// QuestionRequest is sent to the TUI to display questions
type QuestionRequest struct {
	ID        string     // Unique identifier for this request
	Questions []Question // Questions to display
}

// QuestionResponse contains the user's answers
type QuestionResponse struct {
	RequestID string           // ID of the original request
	Answers   map[int][]string // Question index -> selected option labels
	Cancelled bool             // True if user cancelled
}

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

	var questions []Question
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
	return &QuestionRequest{
		ID:        fmt.Sprintf("ask-%d", t.requestCounter),
		Questions: questions,
	}, nil
}

// ExecuteWithResponse formats the user's response for the LLM
func (t *AskUserQuestionTool) ExecuteWithResponse(ctx context.Context, params map[string]any, response any, cwd string) ui.ToolResult {
	resp, ok := response.(*QuestionResponse)
	if !ok {
		return ui.NewErrorResult("AskUserQuestion", "invalid response type")
	}

	if resp.Cancelled {
		return ui.ToolResult{
			Success: true,
			Output:  "User cancelled the question prompt without answering.",
			Metadata: ui.ResultMetadata{
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
	questionsRaw, _ := params["questions"]
	questionsJSON, _ := json.Marshal(questionsRaw)
	var questions []Question
	json.Unmarshal(questionsJSON, &questions)

	for i, q := range questions {
		answers := resp.Answers[i]
		if len(answers) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("\n%s: ", q.Header))
		sb.WriteString(strings.Join(answers, ", "))
	}

	return ui.ToolResult{
		Success: true,
		Output:  sb.String(),
		Metadata: ui.ResultMetadata{
			Title:    "AskUserQuestion",
			Icon:     "❓",
			Subtitle: fmt.Sprintf("%d answers", len(resp.Answers)),
		},
	}
}

// Execute should not be called directly for interactive tools
func (t *AskUserQuestionTool) Execute(ctx context.Context, params map[string]any, cwd string) ui.ToolResult {
	return ui.NewErrorResult("AskUserQuestion", "this tool requires user interaction - use PrepareInteraction and ExecuteWithResponse")
}

func init() {
	Register(NewAskUserQuestionTool())
}
