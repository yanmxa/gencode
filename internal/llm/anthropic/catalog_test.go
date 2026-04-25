package anthropic

import (
	"testing"

	"github.com/yanmxa/gencode/internal/llm"
)

func TestCatalogModelThinkingSupport(t *testing.T) {
	tests := []struct {
		model          string
		wantReasoning  bool
		wantInputLimit int
	}{
		{
			model:          "claude-opus-4-1-20250805",
			wantReasoning:  true,
			wantInputLimit: 200000,
		},
		{
			model:          "claude-sonnet-4@20250514",
			wantReasoning:  true,
			wantInputLimit: 200000,
		},
		{
			model:          "claude-3-7-sonnet-latest",
			wantReasoning:  true,
			wantInputLimit: 200000,
		},
		{
			model:          "claude-3-5-haiku-20241022",
			wantReasoning:  false,
			wantInputLimit: 200000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			info, ok := CatalogModel(tt.model)
			if !ok {
				t.Fatalf("CatalogModel(%q) not found", tt.model)
			}
			if info.InputTokenLimit != tt.wantInputLimit {
				t.Fatalf("InputTokenLimit = %d, want %d", info.InputTokenLimit, tt.wantInputLimit)
			}
			if got := supportsThinkingModel(tt.model); got != tt.wantReasoning {
				t.Fatalf("supportsThinkingModel(%q) = %v, want %v", tt.model, got, tt.wantReasoning)
			}
		})
	}
}

func TestAnthropicThinkingBudget(t *testing.T) {
	tests := []struct {
		model string
		level llm.ThinkingLevel
		want  int
	}{
		{model: "claude-opus-4-1-20250805", level: llm.ThinkingNormal, want: 5000},
		{model: "claude-sonnet-4@20250514", level: llm.ThinkingUltra, want: 128000},
		{model: "claude-3-5-haiku-20241022", level: llm.ThinkingUltra, want: 0},
		{model: "unknown-model", level: llm.ThinkingHigh, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := anthropicThinkingBudget(tt.model, tt.level); got != tt.want {
				t.Fatalf("anthropicThinkingBudget(%q, %v) = %d, want %d", tt.model, tt.level, got, tt.want)
			}
		})
	}
}

func TestStaticModelsUsesOfficialCatalog(t *testing.T) {
	models := StaticModels()
	if len(models) == 0 {
		t.Fatal("expected static models")
	}

	seen := map[string]bool{}
	for _, model := range models {
		seen[model.ID] = true
	}

	for _, required := range []string{
		"claude-opus-4-1-20250805",
		"claude-opus-4-20250514",
		"claude-sonnet-4-20250514",
		"claude-3-7-sonnet-20250219",
		"claude-3-5-haiku-20241022",
	} {
		if !seen[required] {
			t.Fatalf("expected %q in static model list", required)
		}
	}
}
