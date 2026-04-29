package tool

import "testing"

func TestBuiltInToolSchemasAreOpenAICompatibleObjects(t *testing.T) {
	for _, schema := range GetToolSchemas() {
		params, ok := schema.Parameters.(map[string]any)
		if !ok {
			t.Fatalf("%s parameters must be a JSON schema object map", schema.Name)
		}
		if typ, _ := params["type"].(string); typ != "object" {
			t.Fatalf("%s parameters must declare top-level type object, got %v", schema.Name, params["type"])
		}
		for _, keyword := range []string{"oneOf", "anyOf", "allOf", "enum", "not"} {
			if _, exists := params[keyword]; exists {
				t.Fatalf("%s parameters must not use top-level %q", schema.Name, keyword)
			}
		}
	}
}

func TestAskUserQuestionSchemaRejectsEmptyQuestionsShape(t *testing.T) {
	params := askUserQuestionToolSchema.Parameters.(map[string]any)
	if got, ok := params["minProperties"].(int); !ok || got != 1 {
		t.Fatalf("AskUserQuestion must require at least one input property, got %#v", params["minProperties"])
	}

	properties := params["properties"].(map[string]any)
	questions := properties["questions"].(map[string]any)
	if got, ok := questions["minItems"].(int); !ok || got != 1 {
		t.Fatalf("AskUserQuestion questions must require at least one item, got %#v", questions["minItems"])
	}
	if got, ok := questions["maxItems"].(int); !ok || got != 8 {
		t.Fatalf("AskUserQuestion questions must allow at most eight items, got %#v", questions["maxItems"])
	}

	item := questions["items"].(map[string]any)
	itemProps := item["properties"].(map[string]any)
	options := itemProps["options"].(map[string]any)
	if got, ok := options["minItems"].(int); !ok || got != 2 {
		t.Fatalf("AskUserQuestion nested options must require at least two items, got %#v", options["minItems"])
	}
	if got, ok := options["maxItems"].(int); !ok || got != 8 {
		t.Fatalf("AskUserQuestion nested options must allow at most eight items, got %#v", options["maxItems"])
	}
}
